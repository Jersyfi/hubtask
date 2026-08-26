// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// rows is the export, as a table a test writes by hand.
type rows struct {
	byTable map[string][]repository.Row
	markers map[string][]repository.Tombstone
	blobs   map[string]repository.MediaLocation
	failure error
}

func newRows() *rows {
	return &rows{
		byTable: map[string][]repository.Row{},
		markers: map[string][]repository.Tombstone{},
		blobs:   map[string]repository.MediaLocation{},
	}
}

func (r *rows) Rows(_ context.Context, table string, _ time.Time, yield func(repository.Row) error) error {
	if r.failure != nil {
		return r.failure
	}
	for _, row := range r.byTable[table] {
		if err := yield(row); err != nil {
			return err
		}
	}
	return nil
}

func (r *rows) Tombstones(_ context.Context, table string, since time.Time, yield func(repository.Tombstone) error) error {
	if since.IsZero() {
		return nil
	}
	for _, marker := range r.markers[table] {
		if err := yield(marker); err != nil {
			return err
		}
	}
	return nil
}

func (r *rows) MediaLocation(_ context.Context, checksum string) (repository.MediaLocation, error) {
	location, found := r.blobs[checksum]
	if !found {
		return repository.MediaLocation{}, shared.ErrNotFound
	}
	return location, nil
}

var _ repository.Export = (*rows)(nil)

// snapshotDouble is the consistency the export reads under, without a database.
type snapshotDouble struct {
	at    time.Time
	taken int
}

func (s *snapshotDouble) WithinSnapshot(
	ctx context.Context, _ persistence.Scope, fn func(context.Context, time.Time) error,
) error {
	s.taken++
	return fn(ctx, s.at)
}

var _ persistence.Snapshot = (*snapshotDouble)(nil)

// keys is the key materialiser: a fixed key per purpose, so that a test can assert that a member
// was encrypted without owning a cipher.
type keys struct{ purposes []crypto.Purpose }

func (k *keys) DeriveFromMaster(_ context.Context, purpose crypto.Purpose, length int) (crypto.MasterDerived, error) {
	k.purposes = append(k.purposes, purpose)
	return crypto.MasterDerived{KeyID: "k1", Key: secret.NewBytes(bytes.Repeat([]byte{0x11}, length))}, nil
}

func (k *keys) ReproduceFromMaster(_ context.Context, _ string, _ crypto.Purpose, length int) (secret.Bytes, error) {
	return secret.NewBytes(bytes.Repeat([]byte{0x11}, length)), nil
}

var _ crypto.KeyMaterialiser = (*keys)(nil)

// maskingCipher is obviously not encryption, which is the point: a plaintext leak is then visible
// in an assertion rather than in a hexdump. The real cipher is exercised in test/backup.
type maskingCipher struct{ sealed []crypto.Purpose }

const mask = 0x5A

func (c *maskingCipher) KeyBytes() int { return 32 }

func (c *maskingCipher) Seal(w io.Writer, _ secret.Bytes, purpose crypto.Purpose) (io.WriteCloser, error) {
	c.sealed = append(c.sealed, purpose)
	if _, err := io.WriteString(w, "sealed\n"); err != nil {
		return nil, err
	}
	return maskWriter{to: w}, nil
}

func (c *maskingCipher) Open(r io.Reader, _ secret.Bytes, _ crypto.Purpose) (io.Reader, error) {
	buffered := bufio.NewReader(r)
	if _, err := buffered.ReadString('\n'); err != nil {
		return nil, crypto.NotAuthentic()
	}
	return maskReader{from: buffered}, nil
}

type maskWriter struct{ to io.Writer }

func (m maskWriter) Write(p []byte) (int, error) {
	masked := make([]byte, len(p))
	for i := range p {
		masked[i] = p[i] ^ mask
	}
	if _, err := m.to.Write(masked); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (m maskWriter) Close() error { return nil }

type maskReader struct{ from io.Reader }

func (m maskReader) Read(p []byte) (int, error) {
	n, err := m.from.Read(p)
	for i := range p[:n] {
		p[i] ^= mask
	}
	return n, err
}

var _ crypto.StreamCipher = (*maskingCipher)(nil)

// objects is the media store.
type objects struct{ content map[string]string }

func (o *objects) Put(context.Context, storage.Upload) error { return nil }
func (o *objects) Delete(context.Context, string) error      { return nil }

func (o *objects) Get(_ context.Context, key string) (storage.Object, error) {
	body, found := o.content[key]
	if !found {
		return storage.Object{}, shared.ErrNotFound
	}
	return storage.Object{Content: io.NopCloser(strings.NewReader(body)), Size: int64(len(body))}, nil
}

var _ storage.ObjectStore = (*objects)(nil)

type performHarness struct {
	*harness
	runs     *runStore
	export   *rows
	snapshot *snapshotDouble
	keys     *keys
	cipher   *maskingCipher
	objects  *objects
}

func newPerformHarness(t *testing.T) *performHarness {
	t.Helper()
	h := newHarness()
	enabledTarget(t, h)
	return &performHarness{
		harness: h, runs: newRuns(), export: newRows(),
		snapshot: &snapshotDouble{at: now}, keys: &keys{},
		cipher: &maskingCipher{}, objects: &objects{content: map[string]string{}},
	}
}

func (p *performHarness) performer() Performer {
	return Performer{
		Runs: p.runs, Targets: p.targets, Export: p.export, Opener: p.opener,
		Encryptor: p.encryptor, Keys: p.keys, Cipher: p.cipher, Objects: p.objects,
		Snapshot: p.snapshot, UnitOfWork: p.uow, Clock: clock.Fixed(now), IDs: ids{next: runID},
		SchemaVersion: "0032", ProductVersion: "0.4.5",
	}
}

func performInput() PerformInput {
	return PerformInput{
		RunID: runID, TargetID: targetID, TenantID: tenantID,
		Mode: domain.ModeFull, Trigger: domain.TriggerManual, IncludeMedia: true,
	}
}

func TestARunWritesAnArchiveAndRecordsWhatItLeftBehind(t *testing.T) {
	h := newPerformHarness(t)
	h.export.byTable["work_item"] = []repository.Row{
		{ID: "w1", ChangedAt: now.Add(-time.Hour), Data: map[string]any{"state": "OPEN"}},
	}

	run, err := h.performer().Perform(context.Background(), performInput())
	if err != nil {
		t.Fatalf("performing: %v", err)
	}

	switch {
	case run.Status != domain.RunSucceeded:
		t.Fatalf("status %s", run.Status)
	case run.ArchivePath == "":
		t.Fatal("the run records no archive")
	case run.ItemCount != 1:
		t.Fatalf("item count %d", run.ItemCount)
	case h.snapshot.taken != 1:
		t.Fatalf("%d snapshots taken - the export reads one point in time", h.snapshot.taken)
	}

	// The archive is at the target, and its manifest is readable there without the key.
	stored := h.opener.store.objects
	if _, found := stored[run.ArchivePath+"/"+archive.ManifestName]; !found {
		t.Fatalf("no manifest at the target: %v", keysOf(stored))
	}
	if _, found := stored[run.ArchivePath+"/"+archive.ChecksumsName]; !found {
		t.Fatal("no checksums.txt - the run did not finish")
	}
	member := stored[run.ArchivePath+"/"+archive.DataName("work_items")]
	if bytes.Contains(member, []byte(`"w1"`)) {
		t.Fatalf("the member reached the target in the clear:\n%s", member)
	}
}

// The archive key is bound to the target, so two targets never share one.
func TestTheArchiveKeyIsBoundToItsTarget(t *testing.T) {
	h := newPerformHarness(t)

	if _, err := h.performer().Perform(context.Background(), performInput()); err != nil {
		t.Fatalf("performing: %v", err)
	}
	if len(h.keys.purposes) != 1 {
		t.Fatalf("%d keys derived", len(h.keys.purposes))
	}
	if h.keys.purposes[0] != archiveKeyPurpose(targetID) {
		t.Fatalf("the key is bound to %q", h.keys.purposes[0])
	}
}

// An unencrypted target derives no key at all, and the members are the records.
func TestAnUnencryptedTargetWritesNoKeyAndNoCipher(t *testing.T) {
	h := newPerformHarness(t)
	target := h.targets.stored[0]
	target.EncryptionMode = domain.EncryptionNone
	h.targets.stored[0] = target
	h.export.byTable["label"] = []repository.Row{
		{ID: "l1", ChangedAt: now, Data: map[string]any{"colour_token": "label.red"}},
	}

	run, err := h.performer().Perform(context.Background(), performInput())
	if err != nil {
		t.Fatalf("performing: %v", err)
	}
	if len(h.keys.purposes) != 0 || len(h.cipher.sealed) != 0 {
		t.Fatalf("an unencrypted target derived %d keys and sealed %d members",
			len(h.keys.purposes), len(h.cipher.sealed))
	}
	member := h.opener.store.objects[run.ArchivePath+"/"+archive.DataName("labels")]
	if !bytes.Contains(member, []byte(`"l1"`)) {
		t.Fatalf("the member is not the records:\n%s", member)
	}
}

// A second run at a target that is already being backed up is refused, and refused without writing
// anything: the lock is what stops two archives being written at one moment.
func TestASecondRunAtABusyTargetIsRefused(t *testing.T) {
	h := newPerformHarness(t)
	h.runs.claimed = false

	_, err := h.performer().Perform(context.Background(), performInput())
	if err == nil {
		t.Fatal("a second run at a busy target went ahead")
	}
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeTargetBusy {
		t.Fatalf("detail code: %v", err)
	}
	if len(h.opener.store.objects) != 0 {
		t.Fatal("a refused run wrote to the target")
	}
	// And it closed no run row, because it never claimed one.
	if len(h.runs.outcomes) != 0 {
		t.Fatalf("a run that never started was closed: %+v", h.runs.outcomes)
	}
}

// A run that failed is recorded as failed rather than left RUNNING: a row nobody closed would hold
// the target's lock until somebody noticed.
func TestAFailedRunIsClosedWithItsCode(t *testing.T) {
	h := newPerformHarness(t)
	h.export.failure = shared.ErrUnavailable.WithDetail("postgres.query_failed")

	_, err := h.performer().Perform(context.Background(), performInput())
	if err == nil {
		t.Fatal("a failing export produced an archive")
	}
	if len(h.runs.outcomes) != 1 {
		t.Fatalf("%d outcomes recorded", len(h.runs.outcomes))
	}
	outcome := h.runs.outcomes[0]
	if outcome.Status != domain.RunFailed {
		t.Fatalf("status %s", outcome.Status)
	}
	if outcome.ErrorCode != "postgres.query_failed" {
		t.Fatalf("code %q", outcome.ErrorCode)
	}
}

// Progress is reported as the entities go past, which is the only honest reading a backup has: how
// many rows a tenant holds is a question that costs a pass over the tenant to answer.
func TestProgressIsReportedAsTheEntitiesGoPast(t *testing.T) {
	h := newPerformHarness(t)
	var readings []float64
	in := performInput()
	in.Report = func(fraction float64) { readings = append(readings, fraction) }

	if _, err := h.performer().Perform(context.Background(), in); err != nil {
		t.Fatalf("performing: %v", err)
	}

	if len(readings) == 0 {
		t.Fatal("nothing was reported")
	}
	for i, reading := range readings {
		if reading <= 0 || reading > 1 {
			t.Fatalf("reading %d is %v", i, reading)
		}
		if i > 0 && reading <= readings[i-1] {
			t.Fatalf("progress went backwards: %v then %v", readings[i-1], reading)
		}
	}
}

// The archive can be read back at the target, which is the round trip the whole thing is for.
func TestWhatWasWrittenCanBeReadBack(t *testing.T) {
	h := newPerformHarness(t)
	h.export.byTable["comment"] = []repository.Row{
		{ID: "k1", ChangedAt: now, Data: map[string]any{"body_length": float64(12)}},
	}

	run, err := h.performer().Perform(context.Background(), performInput())
	if err != nil {
		t.Fatalf("performing: %v", err)
	}

	reader := archive.NewReader(h.opener.store, h.cipher)
	description, err := reader.Describe(context.Background(), run.ArchivePath)
	if err != nil {
		t.Fatalf("describing: %v", err)
	}
	if !description.Complete || description.Manifest.SchemaVersion != "0032" {
		t.Fatalf("manifest: %+v", description.Manifest)
	}

	comments, _ := archive.FindEntity("comments")
	var read int
	err = reader.Records(context.Background(), description, comments,
		secret.NewBytes(bytes.Repeat([]byte{0x11}, 32)), func(record archive.Record) error {
			read++
			if record.ID != "k1" {
				return errors.New("the wrong record came back")
			}
			return nil
		})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if read != 1 {
		t.Fatalf("%d records read", read)
	}
}

// Verifying reads the archive at the target and writes the answer onto the run, whichever way it
// came out.
func TestVerifyingRecordsWhatItFound(t *testing.T) {
	h := newPerformHarness(t)
	run, err := h.performer().Perform(context.Background(), performInput())
	if err != nil {
		t.Fatalf("performing: %v", err)
	}

	sound, err := h.performer().Verify(context.Background(), run.ID, tenantID)
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if !sound {
		t.Fatal("a sound archive did not verify")
	}
	if stored := h.runs.stored[run.ID]; stored.VerifyOK == nil || !*stored.VerifyOK {
		t.Fatalf("the answer was not recorded: %+v", stored)
	}

	// And a damaged one is recorded as damaged rather than as unchecked.
	member := run.ArchivePath + "/" + archive.DataName("containers")
	h.opener.store.objects[member] = []byte("not what was written")

	sound, err = h.performer().Verify(context.Background(), run.ID, tenantID)
	if err != nil {
		t.Fatalf("verifying: %v", err)
	}
	if sound {
		t.Fatal("a damaged archive verified")
	}
	if stored := h.runs.stored[run.ID]; stored.VerifyOK == nil || *stored.VerifyOK {
		t.Fatalf("the finding was not recorded: %+v", stored)
	}
}

func keysOf(objects map[string][]byte) []string {
	var names []string
	for key := range objects {
		names = append(names, key)
	}
	return names
}

// BK-8, at the level the application layer owns it: the plan deletes exactly what it should, only
// archives Hubtask wrote, and nothing else at the target.
func TestTheGenerationPlanDeletesArchivesAndNothingElse(t *testing.T) {
	h := newPerformHarness(t)
	ctx := context.Background()

	// Three archives, oldest first, and a file at the target that is not one of ours.
	var written []domain.Run
	for day := range 3 {
		in := performInput()
		in.RunID = shared.MustParseID("0192f000-0000-7000-8000-00000000010" + string(rune('0'+day)))
		performer := h.performer()
		performer.Clock = clock.Fixed(now.AddDate(0, 0, day))
		performer.IDs = ids{next: in.RunID}
		h.snapshot.at = now.AddDate(0, 0, day)

		run, err := performer.Perform(ctx, in)
		if err != nil {
			t.Fatalf("run %d: %v", day, err)
		}
		written = append(written, run)
		// A run holds the target until it finishes; the double keeps the row, so the next run has
		// to be allowed to claim it again.
		h.runs.claimed = true
	}
	h.opener.store.objects["somebody-elses-file.tar"] = []byte("not ours")

	expiry, err := h.performer().Expire(ctx, ExpireInput{
		TargetID: targetID, TenantID: tenantID,
		Plan:     domain.Retention{KeepLast: 1, MinKeep: 1},
		TimeZone: "Europe/Berlin",
	})
	if err != nil {
		t.Fatalf("expiring: %v", err)
	}

	if len(expiry.Keep) != 1 || expiry.Keep[0].ID != written[2].ID {
		t.Fatalf("kept %+v, want the newest", expiry.Keep)
	}
	if len(expiry.Expire) != 2 {
		t.Fatalf("expired %d, want the two older ones", len(expiry.Expire))
	}
	for _, gone := range written[:2] {
		if _, still := h.opener.store.objects[gone.ArchivePath+"/"+archive.ManifestName]; still {
			t.Errorf("%s is still at the target", gone.ArchivePath)
		}
	}
	if _, still := h.opener.store.objects[written[2].ArchivePath+"/"+archive.ManifestName]; !still {
		t.Error("the archive that was kept was deleted")
	}
	// The file that is not ours is untouched, and it is untouched because nothing ever listed it.
	if _, still := h.opener.store.objects["somebody-elses-file.tar"]; !still {
		t.Error("a file at the target that is not a Hubtask archive was deleted")
	}
	// And the runs whose archives went are recorded as expired rather than removed: the history of
	// what was backed up survives the archives themselves.
	for _, gone := range written[:2] {
		if h.runs.stored[gone.ID].Status != domain.RunExpired {
			t.Errorf("%s is %s", gone.ID, h.runs.stored[gone.ID].Status)
		}
	}
}

// The floor is never undercut, whatever the plan works out to.
func TestTheFloorHoldsAtTheTarget(t *testing.T) {
	h := newPerformHarness(t)
	ctx := context.Background()
	for day := range 2 {
		in := performInput()
		in.RunID = shared.MustParseID("0192f000-0000-7000-8000-00000000020" + string(rune('0'+day)))
		performer := h.performer()
		performer.Clock = clock.Fixed(now.AddDate(0, 0, day))
		performer.IDs = ids{next: in.RunID}
		h.snapshot.at = now.AddDate(0, 0, day)
		if _, err := performer.Perform(ctx, in); err != nil {
			t.Fatalf("run %d: %v", day, err)
		}
		h.runs.claimed = true
	}

	// A plan that keeps nothing at all - which is exactly what min_keep exists for.
	expiry, err := h.performer().Expire(ctx, ExpireInput{
		TargetID: targetID, TenantID: tenantID, Plan: domain.Retention{MinKeep: 2},
	})
	if err != nil {
		t.Fatalf("expiring: %v", err)
	}
	if len(expiry.Expire) != 0 || !expiry.FloorHeld {
		t.Fatalf("expiry: %+v", expiry)
	}
}

// A run that is still going is neither counted nor deleted: it has no checksums.txt, so it is not
// an archive yet.
func TestAnUnfinishedArchiveIsNotCountedByThePlan(t *testing.T) {
	h := newPerformHarness(t)
	ctx := context.Background()
	run, err := h.performer().Perform(ctx, performInput())
	if err != nil {
		t.Fatalf("performing: %v", err)
	}
	delete(h.opener.store.objects, run.ArchivePath+"/"+archive.ChecksumsName)

	expiry, err := h.performer().Expire(ctx, ExpireInput{
		TargetID: targetID, TenantID: tenantID, Plan: domain.Retention{KeepLast: 1, MinKeep: 1},
	})
	if err != nil {
		t.Fatalf("expiring: %v", err)
	}
	if len(expiry.Keep) != 0 || len(expiry.Expire) != 0 {
		t.Fatalf("an unfinished archive was counted: %+v", expiry)
	}
	if _, still := h.opener.store.objects[run.ArchivePath+"/"+archive.ManifestName]; !still {
		t.Fatal("an unfinished archive was deleted")
	}
}

// A zone this installation does not know is not worth failing a retention pass for: the
// generations shift by hours, and refusing to delete anything because tzdata is old is worse.
func TestAnUnknownZoneFallsBackToUTC(t *testing.T) {
	if zoneOr("Mars/Olympus").String() != time.UTC.String() {
		t.Fatalf("an unknown zone gave %s", zoneOr("Mars/Olympus"))
	}
	if zoneOr("").String() != time.UTC.String() {
		t.Fatalf("an empty zone gave %s", zoneOr(""))
	}
	if zoneOr("Europe/Berlin").String() != "Europe/Berlin" {
		t.Fatal("a known zone was not used")
	}
}

// The two enums have the same two values and are kept apart, so the translation has to be right in
// both directions.
func TestTheTwoModesTranslateBothWays(t *testing.T) {
	for _, mode := range []domain.Mode{domain.ModeFull, domain.ModeIncremental} {
		if back := modeBack(modeOf(mode)); back != mode {
			t.Fatalf("%s came back as %s", mode, back)
		}
	}
}

// A chain whose parent has been expired away cannot be continued: saying so now is better than
// writing an incremental nobody can restore.
func TestAChainWhoseParentIsGoneIsRefused(t *testing.T) {
	h := newPerformHarness(t)
	h.runs.stored[jobID] = domain.Run{
		ID: jobID, TargetID: targetID, Mode: domain.ModeIncremental,
		ParentRunID: scheduleID, Status: domain.RunSucceeded, ArchivePath: "somewhere",
	}
	in := performInput()
	in.Mode, in.ParentRunID = domain.ModeIncremental, jobID

	_, err := h.performer().Perform(context.Background(), in)
	if err == nil {
		t.Fatal("an incremental with a broken chain went ahead")
	}
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeChainIncomplete {
		t.Fatalf("detail code: %v", err)
	}
}
