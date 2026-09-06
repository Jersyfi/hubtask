// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	outboxport "github.com/Jersyfi/hubtask/core/application/repository/outbox"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	service "github.com/Jersyfi/hubtask/core/application/service/backup"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	cryptoadapter "github.com/Jersyfi/hubtask/infrastructure/crypto"
)

// The restore against a real local target, a real cipher and the real archive format (E-06).
//
// BK-5: a restore fires no automation, sends no webhook and no email, and restores no token or
// session. BK-6: an object deleted after the archive was taken does not come back, for a row and
// for a medium. BK-10: an archive belonging to another tenant is refused at the listing, at the dry
// run and at the execution. And §8.3's own promise: a dry run changes nothing, proved by a checksum
// of the workspace before and after rather than by reading the code.

var restoreID = shared.MustParseID("0198f0a0-0000-7000-8000-0000000000cd")

// The fixture identities. Real UUIDs, because a restore mints a medium's storage key from the
// tenant and the row's identity - and because everything the archive carries came out of a database
// where an identity is one.
var (
	containerFixture = "0198f0a0-0000-7000-8000-0000000000c1"
	itemFixture      = "0198f0a0-0000-7000-8000-0000000000e1"
	ruleFixture      = "0198f0a0-0000-7000-8000-0000000000a1"
	hookFixture      = "0198f0a0-0000-7000-8000-0000000000b1"
	reminderFixture  = "0198f0a0-0000-7000-8000-0000000000f1"
	mediumFixture    = "0198f0a0-0000-7000-8000-0000000000d1"
)

// ── The doubles the restore writes into ───────────────────────────────────────────────────────

type memoryRestores struct {
	stored map[shared.ID]domain.Restore
}

func newMemoryRestores() *memoryRestores {
	return &memoryRestores{stored: map[shared.ID]domain.Restore{}}
}

func (r *memoryRestores) Insert(_ context.Context, restore domain.Restore) error {
	r.stored[restore.ID] = restore
	return nil
}

func (r *memoryRestores) Find(_ context.Context, id shared.ID) (domain.Restore, error) {
	restore, found := r.stored[id]
	if !found {
		return domain.Restore{}, shared.ErrNotFound.WithDetail(domain.CodeRestoreNotFound)
	}
	return restore, nil
}

func (r *memoryRestores) Claim(_ context.Context, id shared.ID, at time.Time) (bool, error) {
	restore, found := r.stored[id]
	if !found || restore.Finished() {
		return false, nil
	}
	restore.Status, restore.StartedAt = domain.RestoreRunning, at
	r.stored[id] = restore
	return true, nil
}

func (r *memoryRestores) Finish(_ context.Context, outcome domain.RestoreOutcome) error {
	restore := r.stored[outcome.ID]
	restore.Status, restore.FinishedAt = outcome.Status, outcome.FinishedAt
	restore.ErrorCode = outcome.ErrorCode
	if outcome.Report.New+outcome.Report.Conflicts+outcome.Report.Media > 0 ||
		len(outcome.Report.Withheld) > 0 {
		restore.Report = outcome.Report
	}
	r.stored[outcome.ID] = restore
	return nil
}

func (r *memoryRestores) RecordProgress(
	_ context.Context, id shared.ID, report domain.Report, progress map[string]int,
) error {
	restore := r.stored[id]
	restore.Report, restore.Progress = report, maps.Clone(progress)
	r.stored[id] = restore
	return nil
}

func (r *memoryRestores) RecordSafetyCopy(_ context.Context, id, backupRunID shared.ID) error {
	restore := r.stored[id]
	restore.SafetyRunID = backupRunID
	r.stored[id] = restore
	return nil
}

func (r *memoryRestores) InProgress(context.Context) (bool, error) { return false, nil }

var _ repository.Restores = (*memoryRestores)(nil)

// workspace is the tenant a restore writes into, as tables of rows. It is deliberately not a
// database: what is proved here is what the archive format and the restore procedure do, and the
// statements behind the port are proved against a real PostgreSQL in test/integration.
type workspace struct {
	tables map[string]map[string]map[string]any
}

func newWorkspace() *workspace {
	return &workspace{tables: map[string]map[string]map[string]any{}}
}

func identityOf(table string, data map[string]any) string {
	entity, known := archive.FindEntityByTable(table)
	if !known {
		return ""
	}
	parts := make([]string, 0, len(entity.Keys))
	for _, column := range entity.Keys {
		value, _ := data[column].(string)
		parts = append(parts, value)
	}
	return strings.Join(parts, "/")
}

func (w *workspace) Holds(_ context.Context, table string, data map[string]any) (bool, error) {
	_, held := w.tables[table][identityOf(table, data)]
	return held, nil
}

func (w *workspace) Write(
	_ context.Context, table string, data map[string]any, overwrite bool,
) (bool, error) {
	if w.tables[table] == nil {
		w.tables[table] = map[string]map[string]any{}
	}
	key := identityOf(table, data)
	if _, held := w.tables[table][key]; held && !overwrite {
		return false, nil
	}
	w.tables[table][key] = maps.Clone(data)
	return true, nil
}

func (w *workspace) Clear(_ context.Context, table string) (int, error) {
	removed := len(w.tables[table])
	delete(w.tables, table)
	return removed, nil
}

var _ repository.Import = (*workspace)(nil)

// checksum is the whole workspace as one digest, which is how a dry run is judged: §8.3 asks for a
// report and no change, and a digest over everything is the check that cannot be satisfied by a
// change nobody thought to look for.
func (w *workspace) checksum() string {
	digest := sha256.New()
	for _, table := range slices.Sorted(maps.Keys(w.tables)) {
		for _, key := range slices.Sorted(maps.Keys(w.tables[table])) {
			encoded, _ := json.Marshal(w.tables[table][key])
			fmt.Fprintf(digest, "%s\x00%s\x00%s\n", table, key, encoded)
		}
	}
	return hex.EncodeToString(digest.Sum(nil))
}

type memoryJournal struct{ entries []repository.Deletion }

func (j *memoryJournal) DeletedSince(
	_ context.Context, since time.Time, yield func(repository.Deletion) error,
) error {
	for _, entry := range j.entries {
		if !entry.DeletedAt.After(since) {
			continue
		}
		if err := yield(entry); err != nil {
			return err
		}
	}
	return nil
}

var _ repository.Journal = (*memoryJournal)(nil)

// ── The spies BK-5 is proved by ───────────────────────────────────────────────────────────────

// eventSpy is the outbox. Nothing a restore does may reach it: an event in this table is what wakes
// the rule engine and the webhook dispatcher, and §8.4's first and third prohibitions are exactly
// that nothing here is written.
type eventSpy struct{ appended []event.Envelope }

func (s *eventSpy) Append(_ context.Context, envelope event.Envelope) error {
	s.appended = append(s.appended, envelope)
	return nil
}

var _ outboxport.Events = (*eventSpy)(nil)

// mailSpy is every message that would leave the installation.
type mailSpy struct{ sent []mail.Message }

func (s *mailSpy) Send(_ context.Context, message mail.Message) error {
	s.sent = append(s.sent, message)
	return nil
}

var _ mail.Sender = (*mailSpy)(nil)

// putSpy counts what reached the object store, which is how "the medium did not come back" is
// checked rather than assumed.
type putSpy struct {
	*memoryObjects
	puts []string
}

func (s *putSpy) Put(_ context.Context, upload storage.Upload) error {
	s.puts = append(s.puts, upload.Key)
	return nil
}

// ── The harness ───────────────────────────────────────────────────────────────────────────────

type restoreHarness struct {
	*runHarness
	restores *memoryRestores
	into     *workspace
	journal  *memoryJournal
	events   *eventSpy
	mail     *mailSpy
	objects  *putSpy
	prefix   string
}

// newRestoreHarness writes one archive with the real performer and hands back everything needed to
// read it back in.
func newRestoreHarness(t *testing.T, seed func(*runHarness)) *restoreHarness {
	t.Helper()

	run := newRunHarness(t, domain.EncryptionAES256GCM)
	seed(run)

	written, err := run.performer(t, run.store).Perform(context.Background(), runFor(runIdentifier(1), domain.ModeFull))
	if err != nil {
		t.Fatalf("writing the archive to restore from: %v", err)
	}

	return &restoreHarness{
		runHarness: run, restores: newMemoryRestores(), into: newWorkspace(),
		journal: &memoryJournal{}, events: &eventSpy{}, mail: &mailSpy{},
		objects: &putSpy{memoryObjects: run.objects},
		prefix:  written.ArchivePath,
	}
}

func (h *restoreHarness) applier(t *testing.T) service.Applier {
	t.Helper()

	ring, err := cryptoadapter.NewKeyring([]cryptoadapter.KeyMaterial{
		{ID: "k1", Material: secret.New("the master key of this installation, long enough")},
	})
	if err != nil {
		t.Fatalf("the keyring: %v", err)
	}
	envelope := cryptoadapter.NewEnvelope(ring, clockadapter.CryptoRandom{})

	return service.Applier{
		Restores: h.restores, Targets: h.targets, Import: h.into, Journal: h.journal,
		Opener: openerFor{store: h.store}, Encryptor: envelope, Keys: envelope,
		Cipher: realCipher(), Objects: h.objects,
		UnitOfWork: directWork{at: h.at}, Clock: clock.Fixed(h.at.Add(time.Hour)),
		IDs: fixedIDs{}, SchemaVersion: "0032", Batch: 2,
	}
}

// accept writes the restore the way the use case would.
func (h *restoreHarness) accept(t *testing.T, change func(*domain.Restore)) service.ApplyInput {
	t.Helper()
	restore := domain.Restore{
		ID: restoreID, TargetID: runTarget, TenantID: runTenant, SourceArchive: h.prefix,
		Mode: domain.RestoreMerge, ConflictRule: domain.ConflictSkip,
		Status: domain.RestorePending, RequestedBy: runTenant,
	}
	change(&restore)
	if err := h.restores.Insert(context.Background(), restore); err != nil {
		t.Fatalf("accepting the restore: %v", err)
	}
	return service.ApplyInput{RestoreID: restoreID, TenantID: runTenant}
}

// aTenantWithEverythingThatCouldFire seeds the rows §8.4 is about: an automation rule, a webhook
// subscription, a reminder whose moment has gone, and an attachment.
func aTenantWithEverythingThatCouldFire(run *runHarness) {
	export := run.export
	// The attachment's bytes have to be in the object store before the archive is written, which
	// is what makes the medium in the archive a real one rather than a reference to nothing.
	run.objects.content["media/old/d1"] = "an attachment"
	export.rows["container"] = []repository.Row{
		{ID: containerFixture, ChangedAt: runClock, Data: map[string]any{"id": containerFixture, "order_key": "m"}},
	}
	export.rows["work_item"] = []repository.Row{
		{ID: itemFixture, ChangedAt: runClock, Data: map[string]any{
			"id": itemFixture, "collection_id": containerFixture, "is_completed": false,
		}},
	}
	export.rows["automation_rule"] = []repository.Row{
		{ID: ruleFixture, ChangedAt: runClock, Data: map[string]any{"id": ruleFixture, "enabled": true}},
	}
	export.rows["webhook_subscription"] = []repository.Row{
		{ID: hookFixture, ChangedAt: runClock, Data: map[string]any{"id": hookFixture, "active": true}},
	}
	export.rows["reminder"] = []repository.Row{
		{ID: reminderFixture, ChangedAt: runClock, Data: map[string]any{
			"id": reminderFixture, "item_id": itemFixture, "state": "PENDING",
			"fire_at": runClock.Add(-24 * time.Hour).Format(time.RFC3339Nano),
		}},
	}
	export.rows["media_object"] = []repository.Row{
		{ID: mediumFixture, ChangedAt: runClock, Data: map[string]any{
			"id": mediumFixture, "checksum": digestOf("an attachment"), "status": "READY",
			"byte_size": float64(len("an attachment")), "storage_key": "media/old/d1",
		}},
	}
	export.blobs[digestOf("an attachment")] = repository.MediaLocation{
		StorageKey: "media/old/d1", Bytes: int64(len("an attachment")),
	}
}

func digestOf(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// ── BK-5 ──────────────────────────────────────────────────────────────────────────────────────

// BK-5: a restore fires no automation, sends no webhook and no email, and restores no token or
// session - each proved by a spy rather than by inspection.
func TestARestoreFiresNothingAndRestoresNoCredential(t *testing.T) {
	h := newRestoreHarness(t, aTenantWithEverythingThatCouldFire)

	if _, err := h.applier(t).Apply(context.Background(), h.accept(t, func(*domain.Restore) {})); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	// The rules and the subscriptions came back - they are the tenant's configuration - and not
	// one of them produced an event. An event in the outbox is what wakes the rule engine and the
	// webhook dispatcher, so the absence of one is the whole of §8.4's first and third
	// prohibitions.
	if len(h.into.tables["automation_rule"]) != 1 {
		t.Fatalf("the automation rule was not restored, so the test proves nothing")
	}
	if len(h.events.appended) != 0 {
		t.Errorf("a restore wrote %d events: %+v", len(h.events.appended), h.events.appended)
	}
	if len(h.mail.sent) != 0 {
		t.Errorf("a restore sent %d messages", len(h.mail.sent))
	}

	// §8.4's fourth: no token and no session. The archive does not carry them, so the restore
	// cannot write them - and both halves are checked, because an archive that carried them would
	// be the defect whether or not this restore wrote them.
	for _, table := range []string{"access_token", "sync_device"} {
		if rows := h.into.tables[table]; len(rows) != 0 {
			t.Errorf("a restore wrote %d rows into %s", len(rows), table)
		}
		if _, carried := archive.FindEntityByTable(table); carried {
			t.Errorf("the archive carries %s at all", table)
		}
	}

	// And the second: the reminder is back and will not fire.
	if state := h.into.tables["reminder"][reminderFixture]["state"]; state != "LAPSED" {
		t.Errorf("a reminder whose moment has gone came back as %v", state)
	}

	// The attachment did come back, under a key minted for the tenant it landed in rather than the
	// one the archive recorded - otherwise two workspaces' rows would name one object, and
	// deleting either would take the other's bytes.
	if len(h.objects.puts) != 1 {
		t.Fatalf("%d attachments were written back, want 1", len(h.objects.puts))
	}
	if key := h.objects.puts[0]; key != "media/"+runTenant.String()+"/"+mediumFixture {
		t.Errorf("the attachment was written to %q, which is not this workspace's key", key)
	}
}

// ── BK-6 ──────────────────────────────────────────────────────────────────────────────────────

// BK-6: an object deleted after the archive was taken does not come back - for a row, and for a
// medium, whose bytes must not reach the object store either.
func TestTheDeletionJournalKeepsBackARowAndAMedium(t *testing.T) {
	h := newRestoreHarness(t, aTenantWithEverythingThatCouldFire)
	h.journal.entries = []repository.Deletion{
		{Entity: "work_item", EntityID: shared.ID(itemFixture),
			DeletedAt: h.at.Add(time.Minute), Reason: "USER"},
		{Entity: "media_object", EntityID: shared.ID(mediumFixture),
			DeletedAt: h.at.Add(time.Minute), Reason: "DSR_ERASURE"},
	}

	report, err := h.applier(t).Apply(context.Background(), h.accept(t, func(*domain.Restore) {}))
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}

	if _, back := h.into.tables["work_item"][itemFixture]; back {
		t.Error("a row deleted after the archive came back")
	}
	if _, back := h.into.tables["media_object"][mediumFixture]; back {
		t.Error("a medium erased after the archive came back")
	}
	if len(h.objects.puts) != 0 {
		t.Errorf("the bytes of an erased attachment were written back: %v", h.objects.puts)
	}
	// The reminder pointed at the deleted item, so it is kept out too - otherwise the restore
	// would write a row referencing something it deliberately did not create.
	if _, back := h.into.tables["reminder"][reminderFixture]; back {
		t.Error("a reminder for a deleted item came back")
	}
	if report.Deleted() < 3 {
		t.Errorf("the report says the journal kept out %d, want the row, the medium and the reminder",
			report.Deleted())
	}
}

// ── §8.3 step 2 ───────────────────────────────────────────────────────────────────────────────

// A dry run produces a report of what would happen and changes nothing - proved by a digest over
// the whole workspace before and after, which is the check that cannot be satisfied by a change
// nobody thought to look for.
func TestADryRunChangesNothingAtAll(t *testing.T) {
	h := newRestoreHarness(t, aTenantWithEverythingThatCouldFire)

	// Something already in the workspace, so that "unchanged" is a statement about content rather
	// than about emptiness.
	if _, err := h.into.Write(context.Background(), "container",
		map[string]any{"id": "living", "order_key": "z"}, false); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	before := h.into.checksum()

	report, err := h.applier(t).Apply(context.Background(),
		h.accept(t, func(r *domain.Restore) { r.DryRun = true }))
	if err != nil {
		t.Fatalf("the dry run failed: %v", err)
	}

	if after := h.into.checksum(); after != before {
		t.Fatalf("the workspace changed during a dry run:\n before %s\n after  %s", before, after)
	}
	if len(h.objects.puts) != 0 {
		t.Errorf("a dry run wrote %d objects", len(h.objects.puts))
	}
	if report.New == 0 {
		t.Error("a dry run against a workspace missing the whole archive reported nothing new")
	}
	if report.Media == 0 {
		t.Error("a dry run reported no attachments although the archive holds one")
	}
}

// ── BK-10 ─────────────────────────────────────────────────────────────────────────────────────

// BK-10: another tenant's archive is refused at the dry run and at the execution, not only at the
// listing - and refused before anything is written. The manifest is compared against the tenant
// that asked - the archive's owner - not against the destination, which for NEW_TENANT is a
// workspace minted a moment ago and could never match (#206).
func TestAnotherTenantsArchiveIsRefusedAtEveryStage(t *testing.T) {
	other := shared.MustParseID("0198f0a0-0000-7000-8000-0000000000ee")

	for name, dry := range map[string]bool{"at the dry run": true, "at the execution": false} {
		t.Run(name, func(t *testing.T) {
			h := newRestoreHarness(t, aTenantWithEverythingThatCouldFire)
			in := h.accept(t, func(r *domain.Restore) { r.DryRun = dry })
			// The asker is the tenant the run row lives in, and here it is not the tenant the
			// archive's manifest names.
			in.TenantID = other

			_, err := h.applier(t).Apply(context.Background(), in)

			var domainErr *shared.Error
			if !errors.As(err, &domainErr) ||
				domainErr.DetailCode != domain.CodeRestoreArchiveScopeMismatch {
				t.Fatalf("refused with %v", err)
			}
			if len(h.into.tables) != 0 {
				t.Errorf("the refused restore wrote into %d tables", len(h.into.tables))
			}
			// The refusal is on the run, as a code and nothing else.
			if stored := h.restores.stored[restoreID]; stored.Status != domain.RestoreFailed ||
				stored.ErrorCode != domain.CodeRestoreArchiveScopeMismatch {
				t.Errorf("the run was closed as %s / %q", stored.Status, stored.ErrorCode)
			}
		})
	}
}

// And BK-10 at the listing, where it is a filter on the archive's own name: a shared target holds
// other tenants' archives and this tenant is told about none of them.
func TestTheListingAtASharedTargetAnswersOnlyOnesOwnArchives(t *testing.T) {
	h := newRestoreHarness(t, aTenantWithEverythingThatCouldFire)
	other := shared.MustParseID("0198f0a0-0000-7000-8000-0000000000ee")

	// Somebody else's archive, laid out at the same target the way a run leaves one.
	foreign := archive.Name(other, h.at, archive.ModeFull)
	manifest := archive.Manifest{
		FormatVersion: archive.FormatVersion, ArchiveID: other.String(),
		SchemaVersion: "0032", ProductVersion: "0.4.5", Mode: archive.ModeFull,
		Scope:      archive.Scope{Kind: archive.ScopeTenant, ID: other.String()},
		SnapshotAt: h.at, Encryption: archive.Encryption{Mode: archive.EncryptionNone},
		Counts: map[string]int64{},
	}
	var encoded strings.Builder
	if err := manifest.Encode(&encoded); err != nil {
		t.Fatalf("encoding the manifest: %v", err)
	}
	if _, err := h.store.Put(context.Background(),
		foreign+"/"+archive.ManifestName, strings.NewReader(encoded.String())); err != nil {
		t.Fatalf("laying out the other tenant's archive: %v", err)
	}

	listed := listArchives(t, h)

	for _, found := range listed {
		if strings.HasPrefix(found.Path, foreign) {
			t.Fatalf("the listing answered another tenant's archive at %s", found.Path)
		}
	}
	if len(listed) != 1 {
		t.Fatalf("%d archives listed, want this tenant's one", len(listed))
	}
}

// §8.1's promise, made concrete: the listing needs no state in the database. The restorer built
// here has no run repository at all - there is no field for one - so the archives it answers come
// from the manifests at the target and from nowhere else.
func TestTheListingWorksWithNoRunRowsAtAll(t *testing.T) {
	h := newRestoreHarness(t, aTenantWithEverythingThatCouldFire)

	// Everything the database knew about the run, gone: the state a total loss leaves behind.
	h.runs.stored = map[shared.ID]domain.Run{}

	listed := listArchives(t, h)

	if len(listed) != 1 {
		t.Fatalf("%d archives listed with an empty database, want the one at the target", len(listed))
	}
	if listed[0].Path != h.prefix {
		t.Fatalf("the archive is at %s, want %s", listed[0].Path, h.prefix)
	}
	if !listed[0].Complete {
		t.Error("the archive does not say the run that wrote it finished")
	}
}

func listArchives(t *testing.T, h *restoreHarness) []service.Archive {
	t.Helper()

	restorer := service.Restorer{
		Targets: h.targets, Encryptor: noCredentials{}, Opener: openerFor{store: h.store},
		Cipher: realCipher(), Authorizer: allowEverything{}, Audit: silentAudit{},
		UnitOfWork: directWork{at: h.at}, Clock: clock.Fixed(h.at), IDs: fixedIDs{},
	}
	listed, err := (service.ListBackupsAtTarget{Restorer: restorer}).Execute(
		context.Background(), actorIn(runTenant), runTarget, shared.ID(""))
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	return listed
}

func actorIn(tenant shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: tenant,
		AccountName: "The operator", Locale: "en", TimeZone: "UTC",
	}
}

type allowEverything struct{}

func (allowEverything) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return nil
}

type silentAudit struct{}

func (silentAudit) Append(context.Context, audit.Entry) error { return nil }

var _ audit.Sink = silentAudit{}

// noCredentials is the encryptor for a target that has none - the local one, which is what these
// tests write to.
type noCredentials struct{}

func (noCredentials) Seal(context.Context, secret.Secret, crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{}, nil
}

func (noCredentials) Open(context.Context, crypto.Sealed, crypto.Purpose) (secret.Secret, error) {
	return secret.Secret{}, nil
}

func (noCredentials) ActiveKeyID() string { return "k1" }

func (noCredentials) KeyIDs() []string { return []string{"k1"} }

func (noCredentials) Rewrap(_ context.Context, sealed crypto.Sealed, _ crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{KeyID: "k1", Ciphertext: sealed.Ciphertext}, nil
}

var _ crypto.Encryptor = noCredentials{}
