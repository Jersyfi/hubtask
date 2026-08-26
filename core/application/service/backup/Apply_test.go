// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The restore itself (E-06, backup-restore.md §8.3): what it writes, what it deliberately does not
// write, and what a dry run costs. Every case here is a round trip - the archive is written by the
// performer of E-05 and read back by the applier - because an archive nobody wrote is an archive
// whose format the test agrees with rather than the writer.

const restoreID = shared.ID("0192f000-0000-7000-8000-0000000000d1")

// restoreStore is the restore log.
type restoreStore struct {
	stored   map[shared.ID]domain.Restore
	claims   int
	refuse   bool
	outcomes []domain.RestoreOutcome
	safety   map[shared.ID]shared.ID
	running  bool
}

func newRestores() *restoreStore {
	return &restoreStore{
		stored: map[shared.ID]domain.Restore{}, safety: map[shared.ID]shared.ID{},
	}
}

func (r *restoreStore) Insert(_ context.Context, restore domain.Restore) error {
	r.stored[restore.ID] = restore
	return nil
}

func (r *restoreStore) Find(_ context.Context, id shared.ID) (domain.Restore, error) {
	restore, found := r.stored[id]
	if !found {
		return domain.Restore{}, shared.ErrNotFound.WithDetail(domain.CodeRestoreNotFound)
	}
	return restore, nil
}

func (r *restoreStore) Claim(_ context.Context, id shared.ID, at time.Time) (bool, error) {
	r.claims++
	if r.refuse {
		return false, nil
	}
	restore := r.stored[id]
	restore.Status, restore.StartedAt = domain.RestoreRunning, at
	r.stored[id] = restore
	return true, nil
}

func (r *restoreStore) Finish(_ context.Context, outcome domain.RestoreOutcome) error {
	r.outcomes = append(r.outcomes, outcome)
	restore := r.stored[outcome.ID]
	restore.Status = outcome.Status
	// A report that says nothing does not erase one that says something, which is what the
	// statement behind this does with COALESCE.
	if outcome.Report.New+outcome.Report.Conflicts+outcome.Report.Media > 0 ||
		len(outcome.Report.Withheld) > 0 || len(outcome.Report.Entities) > 0 {
		restore.Report = outcome.Report
	}
	r.stored[outcome.ID] = restore
	return nil
}

func (r *restoreStore) RecordProgress(
	_ context.Context, id shared.ID, report domain.Report, progress map[string]int,
) error {
	restore := r.stored[id]
	restore.Report, restore.Progress = report, maps.Clone(progress)
	r.stored[id] = restore
	return nil
}

func (r *restoreStore) RecordSafetyCopy(_ context.Context, id, backupRunID shared.ID) error {
	r.safety[id] = backupRunID
	restore := r.stored[id]
	restore.SafetyRunID = backupRunID
	r.stored[id] = restore
	return nil
}

func (r *restoreStore) InProgress(context.Context) (bool, error) { return r.running, nil }

var _ repository.Restores = (*restoreStore)(nil)

// importStore is the tenant a restore writes into, as a map. It keys rows the way the schema does -
// by the entity's declared key - so that a composite key behaves here as it does in the database.
type importStore struct {
	tables  map[string]map[string]map[string]any
	writes  int
	cleared []string
	// failAfter makes the store give up part way, which is how a worker dying mid-restore looks
	// from in here. Zero means never.
	failAfter int
}

func newImports() *importStore {
	return &importStore{tables: map[string]map[string]map[string]any{}}
}

func keyOf(table string, data map[string]any) string {
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

func (i *importStore) Holds(_ context.Context, table string, data map[string]any) (bool, error) {
	_, held := i.tables[table][keyOf(table, data)]
	return held, nil
}

func (i *importStore) Write(
	_ context.Context, table string, data map[string]any, overwrite bool,
) (bool, error) {
	if i.tables[table] == nil {
		i.tables[table] = map[string]map[string]any{}
	}
	if i.failAfter > 0 && i.writes >= i.failAfter {
		return false, errors.New("the worker died")
	}
	key := keyOf(table, data)
	if _, held := i.tables[table][key]; held && !overwrite {
		return false, nil
	}
	i.tables[table][key] = maps.Clone(data)
	i.writes++
	return true, nil
}

func (i *importStore) Clear(_ context.Context, table string) (int, error) {
	removed := len(i.tables[table])
	i.cleared = append(i.cleared, table)
	delete(i.tables, table)
	return removed, nil
}

var _ repository.Import = (*importStore)(nil)

// journalDouble is the deletion journal, read.
type journalDouble struct{ entries []repository.Deletion }

func (j *journalDouble) DeletedSince(
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

var _ repository.Journal = (*journalDouble)(nil)

// safetyDouble is the backup taken before a destructive mode.
type safetyDouble struct {
	taken []PerformInput
	err   error
}

func (s *safetyDouble) Perform(_ context.Context, in PerformInput) (domain.Run, error) {
	if s.err != nil {
		return domain.Run{}, s.err
	}
	s.taken = append(s.taken, in)
	return domain.Run{ID: in.RunID, TargetID: in.TargetID, TenantID: in.TenantID}, nil
}

type applyHarness struct {
	*performHarness
	restores *restoreStore
	imports  *importStore
	journal  *journalDouble
	safety   *safetyDouble
	prefix   string
}

// newApplyHarness writes one archive with the performer and hands back everything needed to read it
// back in.
func newApplyHarness(t *testing.T, seed func(*rows)) *applyHarness {
	t.Helper()
	performing := newPerformHarness(t)
	seed(performing.export)

	run, err := performing.performer().Perform(context.Background(), performInput())
	if err != nil {
		t.Fatalf("writing the archive to restore from: %v", err)
	}

	return &applyHarness{
		performHarness: performing,
		restores:       newRestores(),
		imports:        newImports(),
		journal:        &journalDouble{},
		safety:         &safetyDouble{},
		prefix:         run.ArchivePath,
	}
}

func (a *applyHarness) applier() Applier {
	return Applier{
		Restores: a.restores, Targets: a.targets, Import: a.imports, Journal: a.journal,
		Opener: a.opener, Encryptor: a.encryptor, Keys: a.keys, Cipher: a.cipher,
		Objects: a.objects, Safety: a.safety, UnitOfWork: a.uow,
		Clock: clock.Fixed(now), IDs: ids{next: runID}, SchemaVersion: "0032", Batch: 2,
	}
}

// accept writes the restore the way the use case would, and hands back what the job takes.
func (a *applyHarness) accept(t *testing.T, change func(*domain.Restore)) ApplyInput {
	t.Helper()
	restore := domain.Restore{
		ID: restoreID, TargetID: targetID, TenantID: tenantID, SourceArchive: a.prefix,
		Mode: domain.RestoreMerge, ConflictRule: domain.ConflictSkip, DryRun: false,
		Status: domain.RestorePending, RequestedBy: actorID,
	}
	change(&restore)
	if err := a.restores.Insert(context.Background(), restore); err != nil {
		t.Fatalf("accepting the restore: %v", err)
	}
	return ApplyInput{RestoreID: restoreID, TenantID: tenantID}
}

func containerRows(export *rows) {
	export.byTable["container"] = []repository.Row{
		{ID: "c1", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
			"id": "c1", "name_length": 4, "parent_id": nil,
		}},
	}
	export.byTable["work_item"] = []repository.Row{
		{ID: "w1", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
			"id": "w1", "collection_id": "c1", "state": "OPEN",
		}},
		{ID: "w2", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
			"id": "w2", "collection_id": "c1", "state": "DONE",
		}},
	}
}

func TestARestoreWritesWhatTheArchiveHoldsAndSaysWhatItDid(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	in := h.accept(t, func(*domain.Restore) {})

	report, err := h.applier().Apply(context.Background(), in)
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}

	if len(h.imports.tables["work_item"]) != 2 {
		t.Fatalf("%d work items landed", len(h.imports.tables["work_item"]))
	}
	if len(h.imports.tables["container"]) != 1 {
		t.Fatalf("%d containers landed", len(h.imports.tables["container"]))
	}
	if report.New != 3 {
		t.Errorf("the report says %d new, want 3", report.New)
	}
	if report.Conflicts != 0 {
		t.Errorf("the report says %d conflicts in an empty tenant", report.Conflicts)
	}
	if report.Entities["work_items"] != 2 {
		t.Errorf("the report attributes %d records to work_items", report.Entities["work_items"])
	}

	// The outcome is recorded, which is what makes "what happened to the restore I started" a
	// question the row answers.
	if len(h.restores.outcomes) != 1 || h.restores.outcomes[0].Status != domain.RestoreSucceeded {
		t.Fatalf("the run was closed as %+v", h.restores.outcomes)
	}
}

// §8.3 step 2, and the property the whole procedure rests on: the report a caller approves is
// produced by a run that changed nothing.
func TestADryRunReportsTheSameAndWritesNothing(t *testing.T) {
	dry := newApplyHarness(t, containerRows)
	wet := newApplyHarness(t, containerRows)

	dryReport, err := dry.applier().Apply(context.Background(),
		dry.accept(t, func(r *domain.Restore) { r.DryRun = true }))
	if err != nil {
		t.Fatalf("the dry run failed: %v", err)
	}
	wetReport, err := wet.applier().Apply(context.Background(), wet.accept(t, func(*domain.Restore) {}))
	if err != nil {
		t.Fatalf("the restore failed: %v", err)
	}

	if dry.imports.writes != 0 {
		t.Errorf("a dry run wrote %d rows", dry.imports.writes)
	}
	if len(dry.imports.tables) != 0 {
		t.Errorf("a dry run left %d tables behind", len(dry.imports.tables))
	}
	if dryReport.New != wetReport.New || dryReport.Conflicts != wetReport.Conflicts {
		t.Fatalf("the dry run reported %+v and the execution did %+v", dryReport, wetReport)
	}
	// The transaction the dry run opened was a reading one, which is what makes "changes nothing"
	// the database's answer rather than this code's promise.
	if dry.uow.writes != wet.uow.writes-1 && dry.uow.writes >= wet.uow.writes {
		t.Errorf("a dry run opened %d writing transactions", dry.uow.writes)
	}
}

// INSPECT is the mode that cannot write, whatever `dry_run` says.
func TestInspectNeverWritesEvenWhenTheRequestSaysItIsNotADryRun(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	in := h.accept(t, func(r *domain.Restore) {
		r.Mode, r.DryRun = domain.RestoreInspect, false
	})

	report, err := h.applier().Apply(context.Background(), in)
	if err != nil {
		t.Fatalf("inspecting: %v", err)
	}
	if h.imports.writes != 0 {
		t.Fatalf("INSPECT wrote %d rows", h.imports.writes)
	}
	if report.New == 0 {
		t.Error("INSPECT reported no difference at all against an empty tenant")
	}
}

// BK-6. An object deleted after the archive was taken does not come back, and neither does anything
// that would point at it.
func TestTheDeletionJournalKeepsAnErasedObjectOut(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	h.journal.entries = []repository.Deletion{{
		Entity: "work_item", EntityID: shared.ID("w1"),
		DeletedAt: now.Add(time.Hour), Reason: "USER",
	}}
	in := h.accept(t, func(*domain.Restore) {})

	report, err := h.applier().Apply(context.Background(), in)
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}

	if _, back := h.imports.tables["work_item"]["w1"]; back {
		t.Fatal("an object deleted after the archive came back through the restore")
	}
	if _, present := h.imports.tables["work_item"]["w2"]; !present {
		t.Error("the object that was not deleted did not come back")
	}
	if report.Deleted() != 1 {
		t.Errorf("the report says the journal kept out %d, want 1", report.Deleted())
	}
}

// A deletion recorded *before* the archive was taken is not in the archive, so it is not a reason
// to withhold anything - and reading the whole journal would be a pass over a table that outlives
// every archive.
func TestADeletionFromBeforeTheArchiveWithholdsNothing(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	h.journal.entries = []repository.Deletion{{
		Entity: "work_item", EntityID: shared.ID("w1"),
		DeletedAt: now.Add(-2 * time.Hour), Reason: "USER",
	}}

	report, err := h.applier().Apply(context.Background(), h.accept(t, func(*domain.Restore) {}))
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}
	if report.Deleted() != 0 {
		t.Errorf("a deletion older than the archive withheld %d objects", report.Deleted())
	}
	if len(h.imports.tables["work_item"]) != 2 {
		t.Errorf("%d work items landed", len(h.imports.tables["work_item"]))
	}
}

func TestACollisionIsSettledByTheRuleTheRestoreWasGiven(t *testing.T) {
	for name, test := range map[string]struct {
		rule        domain.ConflictRule
		state       string
		skipped     int
		overwritten int
	}{
		"skip leaves the living object alone": {domain.ConflictSkip, "LIVE", 1, 0},
		"overwrite replaces it":               {domain.ConflictOverwrite, "OPEN", 0, 1},
	} {
		t.Run(name, func(t *testing.T) {
			h := newApplyHarness(t, containerRows)
			h.imports.tables["work_item"] = map[string]map[string]any{
				"w1": {"id": "w1", "collection_id": "c1", "state": "LIVE"},
			}
			in := h.accept(t, func(r *domain.Restore) { r.ConflictRule = test.rule })

			report, err := h.applier().Apply(context.Background(), in)
			if err != nil {
				t.Fatalf("restoring: %v", err)
			}

			if state := h.imports.tables["work_item"]["w1"]["state"]; state != test.state {
				t.Errorf("the living object is %v, want %v", state, test.state)
			}
			if report.Conflicts != 1 {
				t.Errorf("%d conflicts, want 1", report.Conflicts)
			}
			if report.Skipped != test.skipped || report.Overwritten != test.overwritten {
				t.Errorf("skipped %d, overwritten %d", report.Skipped, report.Overwritten)
			}
		})
	}
}

// DUPLICATE gives the copy new identities and keeps the copies pointing at each other rather than
// at the originals - otherwise the duplicated items would land inside the living collection.
func TestDuplicateMintsNewIdentitiesAndRemapsTheReferencesBetweenThem(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	h.imports.tables["container"] = map[string]map[string]any{
		"c1": {"id": "c1", "name_length": 4},
	}
	h.imports.tables["work_item"] = map[string]map[string]any{
		"w1": {"id": "w1", "collection_id": "c1", "state": "LIVE"},
	}
	in := h.accept(t, func(r *domain.Restore) { r.ConflictRule = domain.ConflictDuplicate })

	report, err := h.applier().Apply(context.Background(), in)
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}

	duplicateContainer := domain.DuplicateID(restoreID, "containers", "c1").String()
	duplicateItem := domain.DuplicateID(restoreID, "work_items", "w1").String()

	if _, minted := h.imports.tables["container"][duplicateContainer]; !minted {
		t.Fatalf("the colliding container was not duplicated; the table holds %v",
			slicesOfKeys(h.imports.tables["container"]))
	}
	copied, minted := h.imports.tables["work_item"][duplicateItem]
	if !minted {
		t.Fatalf("the colliding item was not duplicated")
	}
	if copied["collection_id"] != duplicateContainer {
		t.Errorf("the duplicated item is in %v, want the duplicated container",
			copied["collection_id"])
	}
	// The living objects are untouched.
	if h.imports.tables["work_item"]["w1"]["state"] != "LIVE" {
		t.Error("a DUPLICATE restore changed the living object")
	}
	// w2 did not collide, and it is new - but it still belongs in the copy of the collection.
	if h.imports.tables["work_item"]["w2"]["collection_id"] != duplicateContainer {
		t.Errorf("a new item beside a duplicated collection stayed in the living one")
	}
	if report.Duplicated != 2 {
		t.Errorf("%d duplicated, want 2", report.Duplicated)
	}
}

func slicesOfKeys(rows map[string]map[string]any) []string {
	var keys []string
	for key := range rows {
		keys = append(keys, key)
	}
	return keys
}

// BK-7's restore half at this level: the same restore applied twice writes the same rows, not two
// copies of them. That is what makes a worker that died safe to replace.
func TestARestoreAppliedTwiceProducesNoDuplicates(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	in := h.accept(t, func(r *domain.Restore) { r.ConflictRule = domain.ConflictDuplicate })

	if _, err := h.applier().Apply(context.Background(), in); err != nil {
		t.Fatalf("the first attempt failed: %v", err)
	}
	after := len(h.imports.tables["work_item"])

	// The row is put back to RUNNING the way a resumed job finds it - with the progress the first
	// attempt recorded, which is what a worker that died leaves behind.
	restore := h.restores.stored[restoreID]
	restore.Status = domain.RestoreRunning
	h.restores.stored[restoreID] = restore

	if _, err := h.applier().Apply(context.Background(), in); err != nil {
		t.Fatalf("the second attempt failed: %v", err)
	}
	if again := len(h.imports.tables["work_item"]); again != after {
		t.Fatalf("a repeated restore left %d work items where the first left %d", again, after)
	}
}

// REPLACE_TENANT resets the tenant to the archive, so what the archive does not name goes - and the
// emptying happens before anything is written rather than as a side effect of it.
func TestReplaceTenantEmptiesTheTenantFirst(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	h.imports.tables["work_item"] = map[string]map[string]any{
		"gone": {"id": "gone", "state": "LIVE"},
	}
	in := h.accept(t, func(r *domain.Restore) { r.Mode = domain.RestoreReplaceTenant })

	if _, err := h.applier().Apply(context.Background(), in); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	if _, survived := h.imports.tables["work_item"]["gone"]; survived {
		t.Error("an object the archive does not name survived a REPLACE_TENANT")
	}
	if len(h.imports.tables["work_item"]) != 2 {
		t.Errorf("%d work items after the replace", len(h.imports.tables["work_item"]))
	}
	// The tenant's own row is never cleared: it is the row the transaction is standing inside.
	for _, table := range h.imports.cleared {
		if table == "tenant" {
			t.Fatal("the tenant row itself was cleared")
		}
	}
}

// §8.3 step 4: the copy comes before the destruction, and its identifier is on the run before the
// mode that needs it runs.
func TestADestructiveModeTakesASafetyCopyFirst(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	in := h.accept(t, func(r *domain.Restore) {
		r.Mode, r.CreateSafetyBackup = domain.RestoreReplaceTenant, true
	})

	if _, err := h.applier().Apply(context.Background(), in); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	if len(h.safety.taken) != 1 {
		t.Fatalf("%d safety copies taken", len(h.safety.taken))
	}
	if h.safety.taken[0].Trigger != domain.TriggerPreRestore {
		t.Errorf("the safety copy was recorded as %s", h.safety.taken[0].Trigger)
	}
	if h.restores.safety[restoreID].IsZero() {
		t.Error("the safety copy is not named on the restore run")
	}
}

// A destructive mode with nowhere to write the copy is refused rather than carried out. A
// destructive restore with no way back is the situation the step exists to prevent.
func TestADestructiveModeWithNoSafetyCopyIsRefused(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	applier := h.applier()
	applier.Safety = nil
	in := h.accept(t, func(r *domain.Restore) {
		r.Mode, r.CreateSafetyBackup = domain.RestoreReplaceTenant, true
	})

	_, err := applier.Apply(context.Background(), in)

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeRestoreSafetyCopyUnavailable {
		t.Fatalf("refused with %v", err)
	}
	if h.imports.writes != 0 {
		t.Error("the destructive mode wrote something before it was refused")
	}
	if len(h.restores.outcomes) != 1 || h.restores.outcomes[0].Status != domain.RestoreFailed {
		t.Errorf("the run was left as %+v", h.restores.outcomes)
	}
}

// BK-10 at the dry run and at the execution, not only at the listing.
func TestAnArchiveOfAnotherTenantIsRefusedAtTheRestore(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000ff")
	in := h.accept(t, func(r *domain.Restore) { r.TenantID = other })

	_, err := h.applier().Apply(context.Background(), in)

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeRestoreArchiveScopeMismatch {
		t.Fatalf("refused with %v", err)
	}
	if h.imports.writes != 0 {
		t.Error("the restore wrote something before the scope was checked")
	}
}

// An archive from a newer schema is refused before anything is read. A restore migrates upwards and
// cannot go the other way, and guessing which columns a later migration added is how a restore
// writes a row that is silently wrong.
func TestAnArchiveFromANewerSchemaIsRefused(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	applier := h.applier()
	applier.SchemaVersion = "0001"

	_, err := applier.Apply(context.Background(), h.accept(t, func(*domain.Restore) {}))

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeRestoreSchemaAhead {
		t.Fatalf("refused with %v", err)
	}
}

// A second restore in one tenant is not a failure to retry into: the work is either happening or
// finished, and a run that never got the lock has no row of its own to close.
func TestARestoreThatCannotClaimTheTenantClosesNothing(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	h.restores.refuse = true

	_, err := h.applier().Apply(context.Background(), h.accept(t, func(*domain.Restore) {}))

	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("refused with %v", err)
	}
	if len(h.restores.outcomes) != 0 {
		t.Errorf("a restore that never started closed %d runs", len(h.restores.outcomes))
	}
}

// BK-7's restore half, as it actually happens: a worker dies part way through and another picks the
// run up. The rows the first attempt wrote are not written again, and - the case this is really
// about - a DUPLICATE restore does not meet its own work and duplicate it a second time.
func TestARestoreResumesWhereTheWorkerDied(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	in := h.accept(t, func(r *domain.Restore) { r.ConflictRule = domain.ConflictDuplicate })

	// The first attempt gets two rows in and stops.
	h.imports.failAfter = 2
	if _, err := h.applier().Apply(context.Background(), in); err == nil {
		t.Fatal("the first attempt did not fail")
	}
	after := h.imports.writes
	if after == 0 {
		t.Fatal("the first attempt wrote nothing, so there is nothing to resume around")
	}

	// A second worker picks the run up: RUNNING, with the progress the first one recorded.
	restore := h.restores.stored[restoreID]
	restore.Status = domain.RestoreRunning
	h.restores.stored[restoreID] = restore
	if len(restore.Progress) == 0 {
		t.Fatal("the first attempt recorded no progress, so a resume cannot skip anything")
	}

	h.imports.failAfter = 0
	report, err := h.applier().Apply(context.Background(), in)
	if err != nil {
		t.Fatalf("the second attempt failed: %v", err)
	}

	if items := len(h.imports.tables["work_item"]); items != 2 {
		t.Fatalf("%d work items after a resumed restore, want the archive's 2", items)
	}
	if containers := len(h.imports.tables["container"]); containers != 1 {
		t.Fatalf("%d containers after a resumed restore, want the archive's 1", containers)
	}
	// The report continues rather than starting again: a restore that did three objects over two
	// attempts says three.
	if report.New != 3 {
		t.Errorf("the resumed report says %d new, want the 3 the archive holds", report.New)
	}
	if report.Duplicated != 0 {
		t.Errorf("a resumed restore duplicated %d of its own rows", report.Duplicated)
	}
}
