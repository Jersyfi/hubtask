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
	// progress keeps every recording, oldest first, for a test that asks what a resumed
	// attempt would have trusted at each step.
	progress []map[string]int
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
	restore.Status = domain.RestoreRunning
	// Kept rather than moved, which is what ClaimRestoreRun's COALESCE does: a resumed attempt
	// continues its own run, and everything derived from it - the identity of a duplicate, and
	// since #790 its name - has to come out the same on the second attempt as on the first.
	if restore.StartedAt.IsZero() {
		restore.StartedAt = at
	}
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
	r.progress = append(r.progress, maps.Clone(progress))
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
	// The one foreign key the database would enforce immediately and this map otherwise would
	// not: a work item's parent must be there (#693).
	if table == "work_item" {
		if parent, named := data["parent_id"].(string); named && parent != "" {
			if _, held := i.tables[table][parent]; !held {
				return false, errors.New("work_item_parent_id_fkey: the parent is not there")
			}
		}
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
	epochs   *epochDouble
	prefix   string
}

// epochDouble counts how often the workspace's synchronisation epoch was advanced (N-11).
type epochDouble struct{ advanced int }

func (e *epochDouble) Advance(context.Context) (int64, error) {
	e.advanced++
	return int64(e.advanced), nil
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
		epochs:         &epochDouble{},
		prefix:         run.ArchivePath,
	}
}

func (a *applyHarness) applier() Applier {
	return Applier{
		Restores: a.restores, Targets: a.targets, Import: a.imports, Journal: a.journal,
		Opener: a.opener, Encryptor: a.encryptor, Keys: a.keys, Cipher: a.cipher,
		Objects: a.objects, Safety: a.safety, UnitOfWork: a.uow, Epochs: a.epochs,
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

// The name the copy is called, and the calendar address it does not take over (#790).
//
// `mint` gave the copy an identity and changed nothing else, so it arrived under the living
// collection's name and met `container_name_uq`; and it kept the calendar UID the client that made
// the original keys its todo by.
func namedRows(export *rows) {
	export.byTable["container"] = []repository.Row{
		{ID: "c1", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
			"id": "c1", "name": "Errands", "parent_id": nil,
		}},
	}
	export.byTable["work_item"] = []repository.Row{
		{ID: "w1", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
			"id": "w1", "collection_id": "c1", "state": "OPEN", "calendar_uid": "todo-1@thunderbird",
		}},
	}
}

func TestADuplicateStandsBesideTheLivingObjectUnderANameOfItsOwn(t *testing.T) {
	h := newApplyHarness(t, namedRows)
	h.imports.tables["container"] = map[string]map[string]any{
		"c1": {"id": "c1", "name": "Errands"},
	}
	h.imports.tables["work_item"] = map[string]map[string]any{
		"w1": {"id": "w1", "collection_id": "c1", "state": "LIVE", "calendar_uid": "todo-1@thunderbird"},
	}
	in := h.accept(t, func(r *domain.Restore) { r.ConflictRule = domain.ConflictDuplicate })

	if _, err := h.applier().Apply(context.Background(), in); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	copied := h.imports.tables["container"][domain.DuplicateID(restoreID, "containers", "c1").String()]
	if copied == nil {
		t.Fatalf("the colliding container was not duplicated")
	}
	want := domain.DuplicatedName(restoreID, now, "Errands")
	if copied["name"] != want {
		t.Errorf("the copy is called %v, want %q - under the living name it meets container_name_uq",
			copied["name"], want)
	}
	if living := h.imports.tables["container"]["c1"]["name"]; living != "Errands" {
		t.Errorf("the living collection was renamed to %v", living)
	}

	item := h.imports.tables["work_item"][domain.DuplicateID(restoreID, "work_items", "w1").String()]
	if item == nil {
		t.Fatalf("the colliding item was not duplicated")
	}
	// A calendar client minted that UID and keys its todo by it. Two rows claiming one address is
	// what `wi_calendar_uid_uq` refuses, and the copy is not the entry the client made.
	if uid := item["calendar_uid"]; uid != nil && uid != "" {
		t.Errorf("the copy took over the calendar address %v", uid)
	}
	if uid := h.imports.tables["work_item"]["w1"]["calendar_uid"]; uid != "todo-1@thunderbird" {
		t.Errorf("the living item's calendar address became %v", uid)
	}
}

// The rename reaches the top of the duplicated tree and stops there.
//
// A collection's `parent_id` is a reference, so the copy lands under the copy of the hub - where
// its name is free, and suffixing it would disfigure a copy for a collision that cannot happen.
// The hub is what has no parent to follow, and `container_name_uq` reads a null parent as a scope
// of its own.
func TestOnlyTheTopOfADuplicatedTreeIsRenamed(t *testing.T) {
	h := newApplyHarness(t, func(export *rows) {
		export.byTable["container"] = []repository.Row{
			{ID: "c1", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
				"id": "c1", "name": "Home", "parent_id": nil,
			}},
			{ID: "c2", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
				"id": "c2", "name": "Errands", "parent_id": "c1",
			}},
		}
	})
	h.imports.tables["container"] = map[string]map[string]any{
		"c1": {"id": "c1", "name": "Home", "parent_id": nil},
		"c2": {"id": "c2", "name": "Errands", "parent_id": "c1"},
	}
	in := h.accept(t, func(r *domain.Restore) { r.ConflictRule = domain.ConflictDuplicate })

	if _, err := h.applier().Apply(context.Background(), in); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	hub := h.imports.tables["container"][domain.DuplicateID(restoreID, "containers", "c1").String()]
	inside := h.imports.tables["container"][domain.DuplicateID(restoreID, "containers", "c2").String()]
	if hub == nil || inside == nil {
		t.Fatalf("the tree was not duplicated whole")
	}
	if want := domain.DuplicatedName(restoreID, now, "Home"); hub["name"] != want {
		t.Errorf("the duplicated hub is called %v, want %q", hub["name"], want)
	}
	if inside["name"] != "Errands" {
		t.Errorf("the collection inside the copy is called %v; its name was already free there",
			inside["name"])
	}
	if inside["parent_id"] != domain.DuplicateID(restoreID, "containers", "c1").String() {
		t.Errorf("the collection landed under %v rather than under the duplicated hub",
			inside["parent_id"])
	}
}

// A custom field definition is copied inside a copied collection and left alone when it belongs to
// the whole workspace.
//
// Its key is unique per collection, and `collection_id` is null for a tenant-wide one - which the
// remap has nothing to move. Renaming the key is not available: `work_item.custom_fields` is a
// document keyed by it rather than by the definition's identity, so the copy would be a field
// none of the copied values are stored under. The row falls back to SKIP, the way an account or a
// medium does, and the report counts it.
func TestATenantWideCustomFieldIsNotDuplicatedAndOneInACollectionIs(t *testing.T) {
	h := newApplyHarness(t, func(export *rows) {
		export.byTable["container"] = []repository.Row{
			{ID: "c1", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
				"id": "c1", "name": "Home", "parent_id": nil,
			}},
		}
		export.byTable["custom_field_definition"] = []repository.Row{
			{ID: "f1", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
				"id": "f1", "key": "priority", "collection_id": "c1",
			}},
			{ID: "f2", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
				"id": "f2", "key": "cost_centre", "collection_id": nil,
			}},
		}
	})
	h.imports.tables["container"] = map[string]map[string]any{
		"c1": {"id": "c1", "name": "Home", "parent_id": nil},
	}
	h.imports.tables["custom_field_definition"] = map[string]map[string]any{
		"f1": {"id": "f1", "key": "priority", "collection_id": "c1"},
		"f2": {"id": "f2", "key": "cost_centre", "collection_id": nil},
	}
	in := h.accept(t, func(r *domain.Restore) { r.ConflictRule = domain.ConflictDuplicate })

	if _, err := h.applier().Apply(context.Background(), in); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	fields := h.imports.tables["custom_field_definition"]
	inCollection := fields[domain.DuplicateID(restoreID, "custom_field_definitions", "f1").String()]
	if inCollection == nil {
		t.Fatalf("the field of the duplicated collection was not copied with it")
	}
	if inCollection["key"] != "priority" {
		t.Errorf("the copied field is keyed %v; its key was already free in the copy",
			inCollection["key"])
	}
	if inCollection["collection_id"] != domain.DuplicateID(restoreID, "containers", "c1").String() {
		t.Errorf("the copied field belongs to %v rather than to the duplicated collection",
			inCollection["collection_id"])
	}
	if copied := fields[domain.DuplicateID(restoreID, "custom_field_definitions", "f2").String()]; copied != nil {
		t.Errorf("the tenant-wide field was copied as %v, and it has no free key to be copied under",
			copied)
	}
}

// The other mode that mints identities does not rename, and that is the point: every one of these
// indexes is per tenant, and a NEW_TENANT copy lands in a tenant that did not exist a moment ago.
// A migration that renamed every collection and dropped every calendar address would be answering
// a collision that cannot happen.
func TestANewTenantCopyKeepsItsNamesAndItsCalendarAddresses(t *testing.T) {
	h := newApplyHarness(t, namedRows)
	in := h.accept(t, func(r *domain.Restore) {
		r.Mode, r.TenantID = domain.RestoreNewTenant, mintedTenant
	})

	if _, err := h.applier().Apply(context.Background(), in); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	copied := h.imports.tables["container"][domain.DuplicateID(restoreID, "containers", "c1").String()]
	if copied == nil {
		t.Fatalf("the container did not land in the new tenant")
	}
	if copied["name"] != "Errands" {
		t.Errorf("a migrated collection is called %v, want the name it had", copied["name"])
	}
	item := h.imports.tables["work_item"][domain.DuplicateID(restoreID, "work_items", "w1").String()]
	if item == nil {
		t.Fatalf("the item did not land in the new tenant")
	}
	if item["calendar_uid"] != "todo-1@thunderbird" {
		t.Errorf("a migrated item's calendar address became %v", item["calendar_uid"])
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

// B-5 (backup-restore.md §12, N-11): a restore into an existing workspace advances its
// synchronisation epoch as it succeeds - REPLACE_TENANT and the selective kinds alike - so that
// every cursor minted before is refused and the devices resynchronise by themselves. A dry run
// advances nothing, and neither does a restore into a new workspace.
func TestARestoreIntoAnExistingWorkspaceAdvancesTheSynchronisationEpoch(t *testing.T) {
	cases := map[string]struct {
		change   func(*domain.Restore)
		advanced int
	}{
		"REPLACE_TENANT": {func(r *domain.Restore) { r.Mode = domain.RestoreReplaceTenant }, 1},
		"MERGE":          {func(r *domain.Restore) { r.Mode = domain.RestoreMerge }, 1},
		"a dry run":      {func(r *domain.Restore) { r.Mode, r.DryRun = domain.RestoreMerge, true }, 0},
		"INSPECT":        {func(r *domain.Restore) { r.Mode = domain.RestoreInspect }, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newApplyHarness(t, containerRows)
			in := h.accept(t, tc.change)
			if _, err := h.applier().Apply(context.Background(), in); err != nil {
				t.Fatalf("restoring: %v", err)
			}
			if h.epochs.advanced != tc.advanced {
				t.Errorf("the epoch was advanced %d times, want %d", h.epochs.advanced, tc.advanced)
			}
		})
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

// BK-10 at the dry run and at the execution, not only at the listing. The manifest is compared
// against the tenant that asked - the archive's owner - so a run row in tenant B pointing at A's
// archive path on a shared target is refused whatever mode it names, NEW_TENANT included (#206).
func TestAnArchiveOfAnotherTenantIsRefusedAtTheRestore(t *testing.T) {
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000ff")
	for name, change := range map[string]func(*domain.Restore){
		"MERGE":      func(r *domain.Restore) { r.TenantID = other },
		"NEW_TENANT": func(r *domain.Restore) { r.Mode, r.TenantID = domain.RestoreNewTenant, mintedTenant },
	} {
		t.Run(name, func(t *testing.T) {
			h := newApplyHarness(t, containerRows)
			in := h.accept(t, change)
			// The asker is the tenant the run row lives in, and here it is not the tenant the
			// archive's manifest names.
			in.TenantID = other

			_, err := h.applier().Apply(context.Background(), in)

			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeRestoreArchiveScopeMismatch {
				t.Fatalf("refused with %v", err)
			}
			if h.imports.writes != 0 {
				t.Error("the restore wrote something before the scope was checked")
			}
		})
	}
}

// An abandoned restore is closed rather than left holding the one-restore-per-tenant lock (#207):
// the queue gave up on the job, nobody else will ever close the row, and InProgress would refuse
// every later restore while it stood.
func TestAnAbandonedRestoreIsClosedUnderItsOwnCode(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	h.accept(t, func(r *domain.Restore) { r.Status = domain.RestoreRunning })

	if err := h.applier().Abandon(context.Background(), restoreID, tenantID); err != nil {
		t.Fatalf("abandoning: %v", err)
	}

	if len(h.restores.outcomes) != 1 {
		t.Fatalf("%d outcomes written, want the abandoned restore's", len(h.restores.outcomes))
	}
	outcome := h.restores.outcomes[0]
	if outcome.Status != domain.RestoreFailed || outcome.ErrorCode != domain.CodeRestoreAbandoned {
		t.Errorf("closed as %s under %q", outcome.Status, outcome.ErrorCode)
	}
}

// INSTANCE stays refused, and the refusal is explicit rather than an accident of the scope
// comparison: even the asker's own tenant archive is refused under it. Since H-10 it carries its
// own code, because "that archive belongs to another workspace" was the wrong sentence about an
// archive that belongs to nobody - the message now names the operator procedure (§8.5).
func TestAnInstanceRestoreIsRefused(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	in := h.accept(t, func(r *domain.Restore) {
		r.Mode, r.TenantID = domain.RestoreInstance, shared.ID("")
	})

	_, err := h.applier().Apply(context.Background(), in)

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeRestoreInstanceIsTheOperators {
		t.Fatalf("refused with %v", err)
	}
	if h.imports.writes != 0 {
		t.Error("the instance restore wrote something")
	}
}

// mintedTenant stands in for the identifier StartRestore mints for a NEW_TENANT restore.
var mintedTenant = shared.MustParseID("0192f000-0000-7000-8000-0000000000ee")

// The mode backup-restore.md §10 recommends for a trial restore: the archive is the asker's own,
// and the rows land in the workspace that was minted for them (#206). Before the fix the precheck
// compared the manifest against that minted workspace and could never match.
//
// The identity rule is the substance: the source rows still live in this installation and every
// identity in the schema is global, so the copy derives a new identity for every row, follows the
// references, rewrites the unique slug, and mints no second copy of a credential - the calendar
// feed's token hash is unique across the installation.
func TestANewTenantRestoreAcceptsItsOwnArchive(t *testing.T) {
	h := newApplyHarness(t, func(export *rows) {
		containerRows(export)
		export.byTable["tenant"] = []repository.Row{
			{ID: tenantID.String(), ChangedAt: now.Add(-time.Hour), Data: map[string]any{
				"id": tenantID.String(), "slug": "the-source", "display_name": "The source",
			}},
		}
		export.byTable["calendar_feed"] = []repository.Row{
			{ID: "f1", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
				"id": "f1", "token_hash": "abc123",
			}},
		}
	})
	in := h.accept(t, func(r *domain.Restore) {
		r.Mode, r.TenantID = domain.RestoreNewTenant, mintedTenant
	})

	report, err := h.applier().Apply(context.Background(), in)
	if err != nil {
		t.Fatalf("restoring into the minted workspace: %v", err)
	}

	if report.New != 4 {
		t.Errorf("%d records restored, wanted the tenant, the container and the two items", report.New)
	}
	if len(h.imports.tables["work_item"]) != 2 || len(h.imports.tables["container"]) != 1 {
		t.Errorf("the minted workspace holds %d items and %d containers",
			len(h.imports.tables["work_item"]), len(h.imports.tables["container"]))
	}
	if len(h.restores.outcomes) != 1 || h.restores.outcomes[0].Status != domain.RestoreSucceeded {
		t.Errorf("the run was left as %+v", h.restores.outcomes)
	}

	// The copy's identities are its own, and the references follow them.
	mintedContainer := domain.DuplicateID(restoreID, "containers", "c1").String()
	if _, held := h.imports.tables["container"][mintedContainer]; !held {
		t.Errorf("the container kept the source's identity: %v", h.imports.tables["container"])
	}
	mintedItem := domain.DuplicateID(restoreID, "work_items", "w1").String()
	item, held := h.imports.tables["work_item"][mintedItem]
	if !held {
		t.Fatalf("the item kept the source's identity: %v", h.imports.tables["work_item"])
	}
	if item["collection_id"] != mintedContainer {
		t.Errorf("the item points at %v rather than at the copied container", item["collection_id"])
	}

	// The tenant row is the minted workspace under a slug of its own.
	tenant, held := h.imports.tables["tenant"][mintedTenant.String()]
	if !held {
		t.Fatalf("the tenant row was not rewritten: %v", h.imports.tables["tenant"])
	}
	if tenant["slug"] != domain.RestoredSlug(mintedTenant) {
		t.Errorf("the copy kept the source's slug %v, which is unique across the installation", tenant["slug"])
	}

	// A credential is not copied: the feed's token hash is unique across the installation, and a
	// URL that read two workspaces would be §8.4's defect wearing a copy.
	if len(h.imports.tables["calendar_feed"]) != 0 {
		t.Errorf("a calendar feed was copied: %v", h.imports.tables["calendar_feed"])
	}
	if report.Withheld[domain.WithheldExcluded] != 1 {
		t.Errorf("the withheld feed is not on the report: %v", report.Withheld)
	}
	if h.epochs.advanced != 0 {
		t.Errorf("a restore into a new workspace advanced its epoch %d times", h.epochs.advanced)
	}
}

// A NEW_TENANT restore whose destination already exists is refused before a row is written. The
// mode's safety argument is that the destination was minted a moment ago; a run row naming a
// living tenant is one that argument no longer covers.
func TestANewTenantRestoreIntoALivingTenantIsRefused(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	h.imports.tables["tenant"] = map[string]map[string]any{
		mintedTenant.String(): {"id": mintedTenant.String()},
	}
	in := h.accept(t, func(r *domain.Restore) {
		r.Mode, r.TenantID = domain.RestoreNewTenant, mintedTenant
	})

	_, err := h.applier().Apply(context.Background(), in)

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeRestoreTenantNotNew {
		t.Fatalf("refused with %v", err)
	}
	if h.imports.writes != 0 {
		t.Error("the restore wrote something into the living tenant")
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

	// The first attempt gets the container in and dies on the work items' batch. The store
	// double is not transactional, so the death is placed where a batch begins: what the database
	// would roll back is here never written.
	h.imports.failAfter = 1
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

// §8.4's second prohibition: a reminder whose moment passed while the data was in an archive is
// marked lapsed rather than left pending, so the scheduler's next pass does not send every one of
// them at once. A reminder for next week still fires.
func TestARestoredReminderWhoseMomentHasGoneIsMarkedLapsed(t *testing.T) {
	h := newApplyHarness(t, func(export *rows) {
		export.byTable["reminder"] = []repository.Row{
			{ID: "past", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
				"id": "past", "state": "PENDING",
				"fire_at": now.Add(-24 * time.Hour).Format(time.RFC3339Nano),
			}},
			{ID: "future", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
				"id": "future", "state": "PENDING",
				"fire_at": now.Add(24 * time.Hour).Format(time.RFC3339Nano),
			}},
			{ID: "sent", ChangedAt: now.Add(-time.Hour), Data: map[string]any{
				"id": "sent", "state": "SENT",
				"fire_at": now.Add(-48 * time.Hour).Format(time.RFC3339Nano),
			}},
		}
	})

	if _, err := h.applier().Apply(context.Background(), h.accept(t, func(*domain.Restore) {})); err != nil {
		t.Fatalf("restoring: %v", err)
	}

	reminders := h.imports.tables["reminder"]
	if state := reminders["past"]["state"]; state != "LAPSED" {
		t.Errorf("a reminder whose moment has gone came back as %v", state)
	}
	if state := reminders["future"]["state"]; state != "PENDING" {
		t.Errorf("a reminder for next week came back as %v", state)
	}
	// A reminder that had already been sent says something about what happened, and a restore does
	// not rewrite that.
	if state := reminders["sent"]["state"]; state != "SENT" {
		t.Errorf("a reminder that had fired came back as %v", state)
	}
}

// A child exported before its parent lands after it (#693): the export orders rows by when they
// changed, a parent edited after its child comes second, and a NEW_TENANT copy - which writes
// every row - failed on the foreign key. The store above enforces that key; the applier defers.
func childBeforeParentRows(export *rows) {
	export.byTable["container"] = []repository.Row{
		{ID: "c1", ChangedAt: now.Add(-time.Hour), Data: map[string]any{"id": "c1", "name_length": 4, "parent_id": nil}},
	}
	export.byTable["work_item"] = []repository.Row{
		// The grandchild first, then the child, then the parent - the worst order.
		{ID: "w3", ChangedAt: now.Add(-3 * time.Hour), Data: map[string]any{"id": "w3", "collection_id": "c1", "parent_id": "w2", "state": "OPEN"}},
		{ID: "w2", ChangedAt: now.Add(-2 * time.Hour), Data: map[string]any{"id": "w2", "collection_id": "c1", "parent_id": "w1", "state": "OPEN"}},
		{ID: "w1", ChangedAt: now.Add(-time.Hour), Data: map[string]any{"id": "w1", "collection_id": "c1", "state": "OPEN"}},
		{ID: "w4", ChangedAt: now, Data: map[string]any{"id": "w4", "collection_id": "c1", "parent_id": "w9", "state": "OPEN"}},
	}
}

func TestAChildExportedBeforeItsParentLandsAfterIt(t *testing.T) {
	h := newApplyHarness(t, childBeforeParentRows)
	in := h.accept(t, func(r *domain.Restore) { r.Mode, r.TenantID = domain.RestoreNewTenant, mintedTenant })

	report, err := h.applier().Apply(context.Background(), in)
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}
	items := h.imports.tables["work_item"]
	if len(items) != 3 {
		t.Fatalf("%d work items landed, want the three whose parents exist: %v", len(items), keysOfTables(items))
	}
	for _, row := range items {
		if parent, named := row["parent_id"].(string); named && parent != "" {
			if _, held := items[parent]; !held {
				t.Errorf("a row landed pointing at a parent that did not: %v", row)
			}
		}
	}
	// The one whose parent is in neither the archive nor the target is withheld and said so.
	if report.Withheld[domain.WithheldOrphaned] != 1 {
		t.Errorf("the orphan was not withheld: %+v", report.Withheld)
	}
	if report.New != 4 {
		t.Errorf("the report counts %d new, want the container and three items", report.New)
	}
}

// The progress a resumed attempt trusts stops before a deferred child, so that a crash between
// the deferral and the settling re-reads it rather than skipping it.
func TestProgressStopsBeforeADeferredChild(t *testing.T) {
	h := newApplyHarness(t, childBeforeParentRows)
	in := h.accept(t, func(r *domain.Restore) { r.Mode, r.TenantID = domain.RestoreNewTenant, mintedTenant })

	if _, err := h.applier().Apply(context.Background(), in); err != nil {
		t.Fatalf("restoring: %v", err)
	}
	// Somewhere along the way the recorded progress for work_items must have been below the
	// count staged; at the end it is the whole entity.
	var lowest, last int
	lowest = 1 << 30
	for _, progress := range h.restores.progress {
		if p, held := progress["work_items"]; held {
			if p < lowest {
				lowest = p
			}
			last = p
		}
	}
	if lowest > 0 && lowest >= 3 {
		t.Errorf("the progress never stopped before the deferred children: lowest %d", lowest)
	}
	if last != 4 {
		t.Errorf("the final progress is %d, want every position", last)
	}
}

func keysOfTables(rows map[string]map[string]any) []string {
	out := make([]string, 0, len(rows))
	for key := range rows {
		out = append(out, key)
	}
	return out
}

// The trial's reader (B-4, P-14): an archive just written is read back whole and compared with
// the workspace, nothing is written, and a member damaged at the target is refused by name.
func TestTheTrialInspectsAnArchiveAndRefusesADamagedOne(t *testing.T) {
	h := newApplyHarness(t, containerRows)

	report, err := h.applier().Inspect(context.Background(), InspectInput{
		TenantID: tenantID, TargetID: targetID, Store: h.opener.store, Archive: h.prefix,
	})
	if err != nil {
		t.Fatalf("inspecting: %v", err)
	}
	if h.imports.writes != 0 || len(h.restores.outcomes) != 0 {
		t.Fatalf("the trial wrote %d rows and closed %d restores", h.imports.writes, len(h.restores.outcomes))
	}
	if report.New != 3 || report.Entities["work_items"] != 2 {
		t.Errorf("the trial's report is %+v", report)
	}

	// The attacker, or the disk: one member's bytes change after the write.
	member := h.prefix + "/" + archive.DataName("work_items")
	damaged := append([]byte{}, h.opener.store.objects[member]...)
	damaged[len(damaged)/2] ^= 0xff
	h.opener.store.objects[member] = damaged
	_, err = h.applier().Inspect(context.Background(), InspectInput{
		TenantID: tenantID, TargetID: targetID, Store: h.opener.store, Archive: h.prefix,
	})
	if shared.AsError(err).DetailCode != archive.CodeChecksumMismatch || memberOf(err) == "" {
		t.Errorf("a damaged member answered %v", err)
	}

	// And a member gone from the target.
	delete(h.opener.store.objects, member)
	if _, err := h.applier().Inspect(context.Background(), InspectInput{
		TenantID: tenantID, TargetID: targetID, Store: h.opener.store, Archive: h.prefix,
	}); err == nil {
		t.Error("an archive missing a member was read back as sound")
	}
}
