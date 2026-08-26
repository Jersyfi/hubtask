// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	service "github.com/Jersyfi/hubtask/core/application/service/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portstorage "github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	recurrenceport "github.com/Jersyfi/hubtask/core/port/recurrence"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	cryptoadapter "github.com/Jersyfi/hubtask/infrastructure/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/recurrence"
)

// BK-7 and BK-8 against a real local target, a real cipher and the real archive format: a run that
// dies half way is resumed without a duplicate archive or a duplicated medium, and the generation
// plan deletes exactly what it should and nothing that is not ours.
//
// The database's half of both is in test/integration, against a real PostgreSQL: the claim that
// makes a resumption possible is a statement rather than a procedure, and it is tested where the
// statement runs.

var (
	runTenant = shared.MustParseID("0198f0a0-0000-7000-8000-0000000000ab")
	runTarget = shared.MustParseID("0198f0a0-0000-7000-8000-0000000000bc")
	runClock  = time.Date(2026, 8, 26, 3, 0, 0, 0, time.UTC)
)

// ── The doubles for the database half ─────────────────────────────────────────────────────────

type memoryRuns struct {
	stored map[shared.ID]domain.Run
	// holder is the run that has the target, which is the whole of what the statement enforces.
	holder shared.ID
}

func newMemoryRuns() *memoryRuns { return &memoryRuns{stored: map[shared.ID]domain.Run{}} }

func (r *memoryRuns) Start(_ context.Context, run domain.Run) (bool, error) {
	if !r.holder.IsZero() && r.holder != run.ID {
		return false, nil
	}
	r.holder = run.ID
	if _, already := r.stored[run.ID]; !already {
		r.stored[run.ID] = run
	}
	return true, nil
}

func (r *memoryRuns) Find(_ context.Context, id shared.ID) (domain.Run, error) {
	run, found := r.stored[id]
	if !found {
		return domain.Run{}, shared.ErrNotFound.WithDetail(domain.CodeRunNotFound)
	}
	return run, nil
}

func (r *memoryRuns) Finish(_ context.Context, outcome domain.Outcome) error {
	run, found := r.stored[outcome.ID]
	if !found || run.Status != domain.RunRunning {
		return shared.ErrConflict.WithDetail(domain.CodeRunNotRunning)
	}
	run.Status, run.ArchivePath, run.FinishedAt = outcome.Status, outcome.ArchivePath, outcome.FinishedAt
	run.SizeBytes, run.ItemCount, run.MediaCount = outcome.SizeBytes, outcome.ItemCount, outcome.MediaCount
	run.SnapshotAt, run.ErrorCode = outcome.SnapshotAt, outcome.ErrorCode
	r.stored[outcome.ID] = run
	r.holder = ""
	return nil
}

func (r *memoryRuns) LatestSuccessful(_ context.Context, target shared.ID) (domain.Run, error) {
	var newest domain.Run
	for _, run := range r.stored {
		if run.TargetID != target || !run.Succeeded() || run.ArchivePath == "" {
			continue
		}
		if newest.ID == "" || run.SnapshotAt.After(newest.SnapshotAt) {
			newest = run
		}
	}
	if newest.ID == "" {
		return domain.Run{}, shared.ErrNotFound.WithDetail(domain.CodeNoParentArchive)
	}
	return newest, nil
}

func (r *memoryRuns) RecordVerification(_ context.Context, id shared.ID, at time.Time, ok bool) error {
	run := r.stored[id]
	run.VerifiedAt, run.VerifyOK = at, &ok
	r.stored[id] = run
	return nil
}

func (r *memoryRuns) SetExpiry(_ context.Context, id shared.ID, at time.Time) error {
	run := r.stored[id]
	run.ExpiresAt = at
	r.stored[id] = run
	return nil
}

func (r *memoryRuns) MarkExpired(_ context.Context, id shared.ID) error {
	run := r.stored[id]
	run.Status = domain.RunExpired
	r.stored[id] = run
	return nil
}

func (r *memoryRuns) LastSuccessPerTarget(context.Context) (map[shared.ID]time.Time, error) {
	return nil, nil
}

var _ repository.Runs = (*memoryRuns)(nil)

type memoryTargets struct{ target domain.Target }

func (t *memoryTargets) Insert(context.Context, domain.Target, crypto.Sealed) error { return nil }

func (t *memoryTargets) List(context.Context) ([]domain.Target, error) {
	return []domain.Target{t.target}, nil
}

func (t *memoryTargets) Find(_ context.Context, id shared.ID) (domain.Target, error) {
	if id != t.target.ID {
		return domain.Target{}, shared.ErrNotFound.WithDetail("backup.target_not_found")
	}
	return t.target, nil
}

func (t *memoryTargets) Credential(context.Context, shared.ID) (crypto.Sealed, error) {
	return crypto.Sealed{}, nil
}

func (t *memoryTargets) RecordTest(context.Context, shared.ID, time.Time, bool, string) error {
	return nil
}

func (t *memoryTargets) Coverage(context.Context) (repository.Coverage, error) {
	return repository.Coverage{Configured: 1}, nil
}

var _ repository.Targets = (*memoryTargets)(nil)

// tableExport is a tenant's rows, written by hand, with a blob or two.
type tableExport struct {
	rows  map[string][]repository.Row
	blobs map[string]repository.MediaLocation
}

func newTableExport() *tableExport {
	return &tableExport{
		rows:  map[string][]repository.Row{},
		blobs: map[string]repository.MediaLocation{},
	}
}

func (e *tableExport) Rows(_ context.Context, table string, _ time.Time, yield func(repository.Row) error) error {
	for _, row := range e.rows[table] {
		if err := yield(row); err != nil {
			return err
		}
	}
	return nil
}

func (e *tableExport) Tombstones(context.Context, string, time.Time, func(repository.Tombstone) error) error {
	return nil
}

func (e *tableExport) MediaLocation(_ context.Context, checksum string) (repository.MediaLocation, error) {
	location, found := e.blobs[checksum]
	if !found {
		return repository.MediaLocation{}, shared.ErrNotFound
	}
	return location, nil
}

var _ repository.Export = (*tableExport)(nil)

type memoryObjects struct{ content map[string]string }

func (o *memoryObjects) Put(context.Context, storage.Upload) error { return nil }
func (o *memoryObjects) Delete(context.Context, string) error      { return nil }

func (o *memoryObjects) Get(_ context.Context, key string) (storage.Object, error) {
	body, found := o.content[key]
	if !found {
		return storage.Object{}, shared.ErrNotFound
	}
	return storage.Object{Content: io.NopCloser(strings.NewReader(body)), Size: int64(len(body))}, nil
}

var _ storage.ObjectStore = (*memoryObjects)(nil)

// directWork runs the callback without a transaction, and takes the snapshot instant from a clock
// a test fixes. The consistency it stands in for is proved against a real database in
// test/integration; what is proved here is what the archive comes out looking like.
type directWork struct{ at time.Time }

func (d directWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (d directWork) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (d directWork) WithinSnapshot(
	ctx context.Context, _ persistence.Scope, fn func(context.Context, time.Time) error,
) error {
	return fn(ctx, d.at)
}

var (
	_ persistence.UnitOfWork = directWork{}
	_ persistence.Snapshot   = directWork{}
)

// openerFor answers the local adapter for one directory, which is a real target rather than a map.
type openerFor struct{ store portstorage.Store }

func (o openerFor) Open(context.Context, portstorage.Spec) (portstorage.Store, error) {
	return o.store, nil
}

func (o openerFor) Kinds() []domain.TargetKind { return []domain.TargetKind{domain.KindLocal} }

// failAfter wraps a store and refuses everything after the nth write - which is what a process
// dying half way through a run leaves behind.
type failAfter struct {
	portstorage.Store
	limit   int
	written int
}

func (f *failAfter) Put(ctx context.Context, key string, content io.Reader) (int64, error) {
	if f.written >= f.limit {
		return 0, shared.ErrUnavailable.WithDetail(portstorage.CodeTargetUnreachable)
	}
	f.written++
	return f.Store.Put(ctx, key, content)
}

// ── The harness ───────────────────────────────────────────────────────────────────────────────

type runHarness struct {
	root    string
	store   portstorage.Store
	runs    *memoryRuns
	targets *memoryTargets
	export  *tableExport
	objects *memoryObjects
	at      time.Time
}

func newRunHarness(t *testing.T, mode domain.EncryptionMode) *runHarness {
	t.Helper()

	root := t.TempDir()
	target, err := domain.NewTarget(domain.NewTargetInput{
		ID: runTarget, TenantID: runTenant, Name: "The volume", Kind: domain.KindLocal,
		Config: domain.TargetConfig{"path": "archives"}, EncryptionMode: mode,
		InsecureAcknowledged: mode == domain.EncryptionNone,
		CreatedBy:            runTenant, Now: runClock,
	})
	if err != nil {
		t.Fatalf("building the target: %v", err)
	}

	// The adapter is rooted where the target says: the domain requires a path, and the store is
	// opened at the same one, so what the test looks at on disk is what a real target holds.
	inside := filepath.Join(root, "archives")
	if err := os.MkdirAll(inside, 0o750); err != nil {
		t.Fatalf("the target directory: %v", err)
	}

	return &runHarness{
		root: inside, store: archiveStore(t, inside), runs: newMemoryRuns(),
		targets: &memoryTargets{target: target}, export: newTableExport(),
		objects: &memoryObjects{content: map[string]string{}}, at: runClock,
	}
}

func (h *runHarness) performer(t *testing.T, store portstorage.Store) service.Performer {
	t.Helper()

	ring, err := cryptoadapter.NewKeyring([]cryptoadapter.KeyMaterial{
		{ID: "k1", Material: secret.New("the master key of this installation, long enough")},
	})
	if err != nil {
		t.Fatalf("the keyring: %v", err)
	}
	envelope := cryptoadapter.NewEnvelope(ring, clockadapter.CryptoRandom{})

	return service.Performer{
		Runs: h.runs, Targets: h.targets, Export: h.export,
		Opener: openerFor{store: store}, Encryptor: envelope, Keys: envelope,
		Cipher: realCipher(), Objects: h.objects,
		Snapshot: directWork{at: h.at}, UnitOfWork: directWork{at: h.at},
		Clock: clock.Fixed(h.at), IDs: fixedIDs{},
		SchemaVersion: "0032", ProductVersion: "0.4.5",
	}
}

type fixedIDs struct{}

func (fixedIDs) NewID() shared.ID { return runTarget }

func runFor(id shared.ID, mode domain.Mode) service.PerformInput {
	return service.PerformInput{
		RunID: id, TargetID: runTarget, TenantID: runTenant,
		Mode: mode, Trigger: domain.TriggerSchedule, IncludeMedia: true,
	}
}

func runIdentifier(n int) shared.ID {
	return shared.MustParseID(fmt.Sprintf("0198f0a0-0000-7000-8000-%012d", n))
}

// ── BK-7 ──────────────────────────────────────────────────────────────────────────────────────

// BK-7: process death during a backup resumes without a duplicate archive and without a duplicated
// media object.
func TestARunThatDiedResumesWithoutDuplicating(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t, domain.EncryptionAES256GCM)

	content := "the bytes of an attachment"
	digest := contentDigest(content)
	h.objects.content["media/one"] = content
	h.export.blobs[digest] = repository.MediaLocation{StorageKey: "media/one", Bytes: int64(len(content))}
	h.export.rows["media_object"] = []repository.Row{{
		ID: "m1", ChangedAt: runClock, Data: map[string]any{
			"checksum": digest, "status": "READY", "byte_size": float64(len(content)),
		},
	}}
	h.export.rows["work_item"] = []repository.Row{
		{ID: "w1", ChangedAt: runClock, Data: map[string]any{"state": "OPEN"}},
	}

	id := runIdentifier(1)

	// The first attempt dies after five members - somewhere in the middle of the data files.
	dying := &failAfter{Store: h.store, limit: 5}
	if _, err := h.performer(t, dying).Perform(ctx, runFor(id, domain.ModeFull)); err == nil {
		t.Fatal("a target that stopped answering produced an archive")
	}
	if h.runs.stored[id].Status != domain.RunFailed {
		t.Fatalf("the run that died is %s", h.runs.stored[id].Status)
	}
	// It left members behind and no manifest: an archive nothing reads.
	before := filesUnder(t, h.root)
	if len(before) == 0 {
		t.Fatal("the first attempt wrote nothing at all - the test proves nothing")
	}
	for _, path := range before {
		if strings.HasSuffix(path, archive.ManifestName) || strings.HasSuffix(path, archive.ChecksumsName) {
			t.Fatalf("the attempt that died wrote %s", filepath.Base(path))
		}
	}

	// The attempt that takes over is the same run. It has to be able to claim the target again.
	h.runs.stored[id] = domain.Run{
		ID: id, TargetID: runTarget, TenantID: runTenant, Trigger: domain.TriggerSchedule,
		Mode: domain.ModeFull, Status: domain.RunRunning, StartedAt: runClock,
	}
	h.runs.holder = id

	run, err := h.performer(t, h.store).Perform(ctx, runFor(id, domain.ModeFull))
	if err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if run.Status != domain.RunSucceeded {
		t.Fatalf("the resumed run is %s", run.Status)
	}

	// One archive, not two: the run kept its own identity, so the prefix is the same one.
	archives := map[string]bool{}
	for _, path := range filesUnder(t, h.root) {
		relative, err := filepath.Rel(h.root, path)
		if err != nil {
			t.Fatalf("relative: %v", err)
		}
		archives[strings.SplitN(relative, string(filepath.Separator), 2)[0]] = true
	}
	if len(archives) != 1 {
		t.Fatalf("%d archives at the target after a resumption: %v", len(archives), archives)
	}

	// And one copy of the medium, at its content address, rather than two.
	media := filepath.Join(h.root, run.ArchivePath, filepath.FromSlash(archive.MediaName(digest)))
	if _, err := os.Stat(media); err != nil {
		t.Fatalf("the medium is not at its content address: %v", err)
	}
	if run.MediaCount != 1 {
		t.Fatalf("media count %d after a resumption", run.MediaCount)
	}

	// The archive is sound, which is the point of all of it.
	if err := archive.NewReader(h.store, realCipher()).Verify(ctx, run.ArchivePath); err != nil {
		t.Fatalf("the resumed archive does not verify: %v", err)
	}
}

// A run that dies leaves no commit point, so nothing reads what it left: that is what makes a
// resumption safe rather than a merge of two attempts.
func TestAnArchiveFromADeadRunIsNotListed(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t, domain.EncryptionAES256GCM)
	h.export.rows["work_item"] = []repository.Row{
		{ID: "w1", ChangedAt: runClock, Data: map[string]any{"state": "OPEN"}},
	}

	dying := &failAfter{Store: h.store, limit: 4}
	if _, err := h.performer(t, dying).Perform(ctx, runFor(runIdentifier(2), domain.ModeFull)); err == nil {
		t.Fatal("a dying target produced an archive")
	}

	listed, err := archive.NewReader(h.store, realCipher()).List(ctx, archive.Prefix(runTenant))
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("an archive from a dead run is listed: %+v", listed)
	}
}

// ── BK-8 ──────────────────────────────────────────────────────────────────────────────────────

// BK-8: the generation plan deletes exactly what it should, min_keep is never undercut, a failed
// run deletes nothing, and a foreign file at the target stays untouched.
func TestTheGenerationPlanDeletesExactlyWhatItShould(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t, domain.EncryptionAES256GCM)
	h.export.rows["work_item"] = []repository.Row{
		{ID: "w1", ChangedAt: runClock, Data: map[string]any{"state": "OPEN"}},
	}

	// A file at the target that is not ours, put there before anything else.
	foreign := filepath.Join(h.root, "somebody-elses-notes.txt")
	if err := os.WriteFile(foreign, []byte("not ours"), 0o600); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	// Five daily archives.
	var written []domain.Run
	for day := range 5 {
		h.at = runClock.AddDate(0, 0, day)
		id := runIdentifier(10 + day)
		run, err := h.performer(t, h.store).Perform(ctx, runFor(id, domain.ModeFull))
		if err != nil {
			t.Fatalf("run %d: %v", day, err)
		}
		written = append(written, run)
	}

	expiry, err := h.performer(t, h.store).Expire(ctx, service.ExpireInput{
		TargetID: runTarget, TenantID: runTenant,
		Plan:     domain.Retention{KeepLast: 2, MinKeep: 1},
		TimeZone: "Europe/Berlin",
	})
	if err != nil {
		t.Fatalf("expiring: %v", err)
	}

	if len(expiry.Keep) != 2 || len(expiry.Expire) != 3 {
		t.Fatalf("kept %d and expired %d, want 2 and 3", len(expiry.Keep), len(expiry.Expire))
	}
	for _, gone := range written[:3] {
		if _, err := os.Stat(filepath.Join(h.root, gone.ArchivePath)); !os.IsNotExist(err) {
			t.Errorf("%s is still at the target", gone.ArchivePath)
		}
	}
	for _, kept := range written[3:] {
		if err := archive.NewReader(h.store, realCipher()).Verify(ctx, kept.ArchivePath); err != nil {
			t.Errorf("%s was kept and no longer verifies: %v", kept.ArchivePath, err)
		}
	}
	// The file that is not ours was never listed, so it was never a candidate.
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("a file at the target that is not a Hubtask archive was deleted: %v", err)
	}
}

// The floor is never undercut, whatever the plan works out to.
func TestTheFloorIsNeverUndercutAtARealTarget(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t, domain.EncryptionNone)
	h.export.rows["work_item"] = []repository.Row{
		{ID: "w1", ChangedAt: runClock, Data: map[string]any{"state": "OPEN"}},
	}
	for day := range 3 {
		h.at = runClock.AddDate(0, 0, day)
		if _, err := h.performer(t, h.store).Perform(ctx, runFor(runIdentifier(20+day), domain.ModeFull)); err != nil {
			t.Fatalf("run %d: %v", day, err)
		}
	}

	// A plan that keeps nothing at all - which is exactly what min_keep exists for.
	expiry, err := h.performer(t, h.store).Expire(ctx, service.ExpireInput{
		TargetID: runTarget, TenantID: runTenant, Plan: domain.Retention{MinKeep: 2},
	})
	if err != nil {
		t.Fatalf("expiring: %v", err)
	}
	if len(expiry.Keep) != 2 || !expiry.FloorHeld {
		t.Fatalf("expiry: %+v", expiry)
	}
	if len(filesUnder(t, h.root)) == 0 {
		t.Fatal("the floor held and the target is empty")
	}
}

// A chain is kept whole: deleting a parent does not free one archive, it destroys every archive
// after it.
func TestAChainSomethingNewerNeedsSurvivesTheRealPlan(t *testing.T) {
	ctx := context.Background()
	h := newRunHarness(t, domain.EncryptionNone)
	h.export.rows["work_item"] = []repository.Row{
		{ID: "w1", ChangedAt: runClock, Data: map[string]any{"state": "OPEN"}},
	}

	full, err := h.performer(t, h.store).Perform(ctx, runFor(runIdentifier(30), domain.ModeFull))
	if err != nil {
		t.Fatalf("the full run: %v", err)
	}
	h.at = runClock.AddDate(0, 0, 1)
	incremental := runFor(runIdentifier(31), domain.ModeIncremental)
	incremental.ParentRunID = full.ID
	later, err := h.performer(t, h.store).Perform(ctx, incremental)
	if err != nil {
		t.Fatalf("the incremental run: %v", err)
	}

	// A plan that would otherwise keep only the newest.
	expiry, err := h.performer(t, h.store).Expire(ctx, service.ExpireInput{
		TargetID: runTarget, TenantID: runTenant, Plan: domain.Retention{KeepLast: 1, MinKeep: 1},
	})
	if err != nil {
		t.Fatalf("expiring: %v", err)
	}
	if len(expiry.Expire) != 0 {
		t.Fatalf("%d archives of the chain were let go", len(expiry.Expire))
	}
	if len(expiry.ChainHeld) != 1 {
		t.Fatalf("the answer names %d archives held for the chain", len(expiry.ChainHeld))
	}
	// And the chain still walks.
	chain, err := archive.NewReader(h.store, realCipher()).Chain(ctx, later.ArchivePath)
	if err != nil {
		t.Fatalf("the chain: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("the chain is %d archives long", len(chain))
	}
}

// ── The schedule through both transitions ─────────────────────────────────────────────────────

// The acceptance criterion: a schedule at FREQ=DAILY;BYHOUR=3 in Europe/Berlin fires at 03:00 local
// through both daylight saving transitions - which is the whole reason the zone is stored on the
// schedule rather than the offset.
func TestADailyScheduleFiresAtThreeThroughBothTransitions(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("the zone: %v", err)
	}
	created := time.Date(2026, 3, 25, 14, 0, 0, 0, berlin)
	schedule, err := domain.NewSchedule(domain.NewScheduleInput{
		ID: runTarget, TargetID: runTarget, TenantID: runTenant, Scope: domain.ScopeTenant,
		RRULE: "FREQ=DAILY;BYHOUR=3;BYMINUTE=0", TimeZone: "Europe/Berlin",
		Mode: domain.ModeIncremental, Retention: domain.DefaultRetention(), Now: created,
	})
	if err != nil {
		t.Fatalf("building the schedule: %v", err)
	}

	expander := recurrence.New()
	// Across the spring transition (29 March 2026) and the autumn one (25 October 2026).
	for _, window := range []struct {
		name string
		from time.Time
		days int
	}{
		{"spring", time.Date(2026, 3, 26, 12, 0, 0, 0, berlin), 6},
		{"autumn", time.Date(2026, 10, 22, 12, 0, 0, 0, berlin), 6},
	} {
		t.Run(window.name, func(t *testing.T) {
			moments, err := expander.Occurrences(recurrenceRuleOf(schedule),
				window.from, window.from.AddDate(0, 0, window.days), window.days+2)
			if err != nil {
				t.Fatalf("expanding: %v", err)
			}
			if len(moments) < window.days-1 {
				t.Fatalf("%d occurrences in %d days", len(moments), window.days)
			}
			for _, moment := range moments {
				local := moment.In(berlin)
				if local.Hour() != 3 || local.Minute() != 0 {
					t.Errorf("%s fired at %s local - the clocks changed and the schedule moved",
						moment.Format(time.RFC3339), local.Format("15:04"))
				}
			}
			// And the instants themselves are an hour apart across the transition, which is the
			// proof that the zone is doing the work rather than a fixed offset.
			offsets := map[int]bool{}
			for _, moment := range moments {
				_, offset := moment.In(berlin).Zone()
				offsets[offset] = true
			}
			if len(offsets) != 2 {
				t.Fatalf("the window covers %d offsets, want both sides of the transition", len(offsets))
			}
		})
	}
}

func recurrenceRuleOf(schedule domain.Schedule) recurrenceport.Rule {
	return recurrenceport.Rule{
		RRULE: schedule.RRULE, TimeZone: schedule.TimeZone, Start: schedule.Anchor(),
	}
}

// contentDigest is the address the archive stores a medium under: the SHA-256 of its content.
func contentDigest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
