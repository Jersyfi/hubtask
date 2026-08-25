// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// UpdateWorkItem (B-05). The item is a work package, so that both fields under test are live: its
// profile carries NOTES, and the activity beside it does not - which is the gate the task is named
// after.

var updateNow = time.Date(2026, 8, 17, 11, 0, 0, 0, time.UTC)

type updateHarness struct {
	handler    UpdateWorkItem
	items      *items
	buckets    *buckets
	containers *containers
	events     *events
	changes    *changes
	audit      *sink
	history    *journal
	authorizer *authorizer
	uow        *unitOfWork
}

func newUpdateHarness() *updateHarness {
	store := &items{
		stored:   map[shared.ID]domain.WorkItem{},
		children: map[shared.ID]domain.ChildCompletion{},
	}
	containerStore := &containers{stored: map[shared.ID]domain.Container{}}

	board := &buckets{stored: map[shared.ID]domain.Bucket{}}
	h := &updateHarness{
		items: store, buckets: board, containers: containerStore,
		events: &events{}, changes: &changes{}, audit: &sink{}, history: &journal{},
		authorizer: &authorizer{}, uow: &unitOfWork{},
	}
	profileRows := &profiles{rows: dueDateProfiles()}
	h.handler = UpdateWorkItem{
		Items: store, Buckets: board, Containers: containerStore,
		Profiles:   profileRows,
		Authorizer: h.authorizer, Events: h.events, Changes: h.changes, Audit: h.audit,
		Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
		UnitOfWork: h.uow, Clock: clock.Fixed(updateNow), IDs: &ids{}, HLC: &hlcSource{},
		// The D-01 machinery, over the same fakes: the patch is its second caller.
		DueDates: DueDateWriter{
			Items: store, Containers: containerStore, Profiles: profileRows,
			Authorizer: h.authorizer, Events: h.events, Changes: h.changes, Audit: h.audit,
			Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
			UnitOfWork: h.uow, Clock: clock.Fixed(updateNow), IDs: &ids{}, HLC: &hlcSource{},
		},
	}

	containerStore.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	containerStore.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}

	store.stored[packageID] = updatableItem(packageID, domain.ItemWorkPackage, taskID)
	store.stored[activityID] = updatableItem(activityID, domain.ItemActivity, packageID)
	// A task as well as the two below it: only a task carries a board, so the bucket tests need
	// one and the capability tests need the others (domain-model.md §2).
	task := updatableItem(taskID, domain.ItemTask, "")
	task.Path = domain.RootPath(taskID)
	store.stored[taskID] = task
	return h
}

// withBucketOn puts a column on some collection's board and hands it back.
func (h *updateHarness) withBucketOn(collection shared.ID) domain.Bucket {
	bucket := domain.Bucket{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000009b1"), TenantID: tenantID,
		CollectionID: collection, Name: "Doing", OrderKey: "a0", Version: 1,
	}
	if collection != collectionID {
		bucket.ID = shared.MustParseID("0192f000-0000-7000-8000-0000000009b2")
	}
	h.buckets.stored[bucket.ID] = bucket
	return bucket
}

func updatableItem(id shared.ID, itemType domain.ItemType, parent shared.ID) domain.WorkItem {
	return domain.WorkItem{
		ID: id, TenantID: tenantID, CollectionID: collectionID, Type: itemType, ParentID: parent,
		Path: domain.RootPath(id), Depth: 1, Title: "Order the cable", Notes: "Three metres, not two.",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 4,
	}
}

func said(value string) *string { return &value }

// written returns the single call SetAttributes took, and fails when there was not exactly one -
// which is the assertion most of these tests actually want to make.
func (h *updateHarness) written(t *testing.T) attributeWrite {
	t.Helper()

	if len(h.items.attributes) != 1 {
		t.Fatalf("%d writes rather than one: %+v", len(h.items.attributes), h.items.attributes)
	}
	return h.items.attributes[0]
}

// The ordinary case, end to end: what was sent lands, and the version the caller matched on is the
// one the write goes against.
func TestUpdateWritesTheFieldsThatWereSent(t *testing.T) {
	h := newUpdateHarness()

	item, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID:          packageID,
		Attributes:      domain.ItemAttributes{Title: said("Order the longer cable")},
		ExpectedVersion: 4,
	})
	if err != nil {
		t.Fatalf("the update was refused: %v", err)
	}

	write := h.written(t)
	if write.expectedVersion != 4 {
		t.Errorf("the write went against version %d rather than 4", write.expectedVersion)
	}
	if write.item.Title != "Order the longer cable" {
		t.Errorf("the stored title is %q", write.item.Title)
	}
	if item.Version != 5 {
		t.Errorf("the answer carries version %d rather than 5", item.Version)
	}
	if !item.UpdatedAt.Equal(updateNow) {
		t.Errorf("updated_at is %s rather than the clock's %s", item.UpdatedAt, updateNow)
	}
}

// A field the caller did not send is not touched. It is the whole reason the command carries
// pointers, and the failure it prevents is silent: a client renaming an item would erase its notes.
func TestUpdateLeavesAFieldThatWasNotSentAlone(t *testing.T) {
	h := newUpdateHarness()

	if _, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID, Attributes: domain.ItemAttributes{Title: said("Order the longer cable")},
	}); err != nil {
		t.Fatalf("the update was refused: %v", err)
	}

	if notes := h.written(t).item.Notes; notes != "Three metres, not two." {
		t.Errorf("the notes became %q", notes)
	}
}

// And the other half of that contract: an empty value is an instruction, not an omission.
func TestUpdateClearsTheNotesWhenAskedTo(t *testing.T) {
	h := newUpdateHarness()

	if _, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID, Attributes: domain.ItemAttributes{Notes: said("")},
	}); err != nil {
		t.Fatalf("clearing the notes was refused: %v", err)
	}

	write := h.written(t)
	if write.item.Notes != "" {
		t.Errorf("the notes are still %q", write.item.Notes)
	}
	if write.item.Title != "Order the cable" {
		t.Errorf("clearing the notes changed the title to %q", write.item.Title)
	}
}

// The capability profile as the gate, which is what B-05 is named after. Nothing is written, and
// the refusal names the type and the capability rather than the field alone.
func TestNotesOnAnActivityAreRefusedAndNothingIsWritten(t *testing.T) {
	h := newUpdateHarness()

	_, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: activityID, Attributes: domain.ItemAttributes{Notes: said("Whole, not semi.")},
	})
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("notes on an activity answered %v", err)
	}

	if len(h.items.attributes) != 0 {
		t.Errorf("%d writes happened anyway", len(h.items.attributes))
	}
	if len(h.events.appended) != 0 || len(h.changes.recorded) != 0 {
		t.Error("a refused update announced itself")
	}
	if !h.uow.rolledBack {
		t.Error("the transaction was not rolled back")
	}
}

// An update that asks for what is already stored writes nothing, spends no version and announces
// nothing. Idempotence that is real rather than merely tolerated: a client echoing the whole object
// back does not produce a change every device has to merge.
func TestAnUpdateThatChangesNothingWritesNothing(t *testing.T) {
	h := newUpdateHarness()

	item, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID,
		Attributes: domain.ItemAttributes{
			Title: said("Order the cable"), Notes: said("Three metres, not two."),
		},
	})
	if err != nil {
		t.Fatalf("the update was refused: %v", err)
	}

	if len(h.items.attributes) != 0 {
		t.Errorf("%d writes happened for a no-op", len(h.items.attributes))
	}
	if len(h.events.appended) != 0 {
		t.Error("a no-op was announced")
	}
	if len(h.changes.recorded) != 0 {
		t.Error("a no-op reached the change log")
	}
	if len(h.audit.entries) != 0 {
		t.Error("a no-op was audited")
	}
	if item.Version != 4 {
		t.Errorf("the version moved to %d", item.Version)
	}
}

// The If-Match is honoured even when the change would have been a no-op: the state the caller was
// reasoning about is not the state that is there, and it should learn that before it decides the
// update was unnecessary.
func TestANoOpStillRefusesAStaleVersion(t *testing.T) {
	h := newUpdateHarness()

	_, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID:          packageID,
		Attributes:      domain.ItemAttributes{Title: said("Order the cable")},
		ExpectedVersion: 2,
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version on a no-op answered %v", err)
	}
}

// The concurrent update of the acceptance criterion: the loser is told, and nothing is written.
func TestAConcurrentUpdateLosesCleanly(t *testing.T) {
	h := newUpdateHarness()
	h.items.conflictOn = packageID

	_, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID:          packageID,
		Attributes:      domain.ItemAttributes{Title: said("Order the longer cable")},
		ExpectedVersion: 4,
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("the losing write answered %v, want a version conflict", err)
	}

	if len(h.events.appended) != 0 || len(h.changes.recorded) != 0 || len(h.audit.entries) != 0 {
		t.Error("the losing write announced itself anyway")
	}
	if !h.uow.rolledBack {
		t.Error("the transaction was not rolled back")
	}
}

// A caller that read no version accepts whatever is there - but the version in hand is still what
// the write matches on, so a concurrent write between the read and here is caught either way.
func TestWithoutAnIfMatchTheWriteStillMatchesOnTheVersionItRead(t *testing.T) {
	h := newUpdateHarness()

	if _, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID, Attributes: domain.ItemAttributes{Title: said("Order the longer cable")},
	}); err != nil {
		t.Fatalf("the update was refused: %v", err)
	}

	if version := h.written(t).expectedVersion; version != 4 {
		t.Errorf("the write went against version %d rather than the one it read", version)
	}
}

// The acceptance criterion: each changed field lands individually in the change log, so that the
// per-field last-writer-wins merge can decide them apart (offline-sync.md §4.2). One entry covering
// both would let one HLC decide the pair, silently discarding whichever field another device wrote.
func TestEachChangedFieldLandsIndividuallyInTheChangeLog(t *testing.T) {
	h := newUpdateHarness()

	if _, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID,
		Attributes: domain.ItemAttributes{
			Title: said("Order the longer cable"), Notes: said("Four metres."),
		},
	}); err != nil {
		t.Fatalf("the update was refused: %v", err)
	}

	if len(h.changes.recorded) != 2 {
		t.Fatalf("%d change log entries rather than one per field", len(h.changes.recorded))
	}

	seen := map[string]any{}
	for _, recorded := range h.changes.recorded {
		if len(recorded.Payload) != 1 {
			t.Errorf("an entry carries %d fields rather than the one that moved: %v", len(recorded.Payload), recorded.Payload)
		}
		for field, value := range recorded.Payload {
			seen[field] = value
		}
		if recorded.EntityID != packageID || recorded.ContainerID != collectionID {
			t.Errorf("an entry points at %s in %s", recorded.EntityID, recorded.ContainerID)
		}
	}
	if seen[domain.FieldTitle] != "Order the longer cable" || seen[domain.FieldNotes] != "Four metres." {
		t.Errorf("the entries carry %v", seen)
	}

	// Each entry takes its own reading, which is what makes them mergeable apart.
	if h.changes.recorded[0].HLC == h.changes.recorded[1].HLC {
		t.Error("both entries share one HLC, so a merge cannot tell them apart")
	}
}

// A field that did not move produces no entry: an offline device must not be handed a change to
// merge for something nobody touched.
func TestOnlyTheFieldThatMovedReachesTheChangeLog(t *testing.T) {
	h := newUpdateHarness()

	if _, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID,
		Attributes: domain.ItemAttributes{
			Title: said("Order the longer cable"), Notes: said("Three metres, not two."),
		},
	}); err != nil {
		t.Fatalf("the update was refused: %v", err)
	}

	if len(h.changes.recorded) != 1 {
		t.Fatalf("%d entries rather than one", len(h.changes.recorded))
	}
	if _, present := h.changes.recorded[0].Payload[domain.FieldNotes]; present {
		t.Error("the unchanged notes reached the change log")
	}
}

// The other half of the acceptance criterion: the event names the changed fields and carries the
// content of no other one.
func TestTheEventCarriesTheChangedFieldsAndNoOthers(t *testing.T) {
	h := newUpdateHarness()

	if _, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID, Attributes: domain.ItemAttributes{Title: said("Order the longer cable")},
	}); err != nil {
		t.Fatalf("the update was refused: %v", err)
	}

	if len(h.events.appended) != 1 {
		t.Fatalf("%d events rather than one", len(h.events.appended))
	}
	announced := h.events.appended[0]
	if announced.Type != event.ItemUpdated {
		t.Errorf("the event is %s", announced.Type)
	}

	changeSet, present := announced.Payload["change_set"].(map[string]any)
	if !present {
		t.Fatalf("there is no change set: %v", announced.Payload)
	}
	if len(changeSet) != 1 {
		t.Errorf("the change set names %d fields: %v", len(changeSet), changeSet)
	}
	title, _ := changeSet[domain.FieldTitle].(map[string]any)
	if title["from"] != "Order the cable" || title["to"] != "Order the longer cable" {
		t.Errorf("the change set says %v", changeSet[domain.FieldTitle])
	}

	// The notes did not change, so neither the field nor its content is in the event at all.
	if _, present := changeSet[domain.FieldNotes]; present {
		t.Error("the change set names the notes, which did not move")
	}
	for field := range announced.Payload {
		if field == domain.FieldNotes || field == domain.FieldTitle {
			t.Errorf("the event carries %q outside the change set", field)
		}
	}
	// The version the change produced, not the one it was written against: a consumer comparing the
	// event with what it fetched over REST reads it to tell which is newer.
	if announced.Payload["version"] != 5 {
		t.Errorf("the event carries version %v", announced.Payload["version"])
	}
}

// Rule 10: the trail records that the fields changed, never what they now say. The entry outlives
// the item, so a title kept here in clear text would be a copy no deletion of that item reaches.
func TestTheAuditEntryRecordsThatTheFieldsChangedAndNotTheirContent(t *testing.T) {
	h := newUpdateHarness()

	if _, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID, Attributes: domain.ItemAttributes{Title: said("Order the longer cable")},
	}); err != nil {
		t.Fatalf("the update was refused: %v", err)
	}

	if len(h.audit.entries) != 1 {
		t.Fatalf("%d audit entries rather than one", len(h.audit.entries))
	}
	entry := h.audit.entries[0]
	if entry.Action != ItemUpdatedAction || entry.Outcome != audit.OutcomeSuccess {
		t.Errorf("the entry is %s / %s", entry.Action, entry.Outcome)
	}
	if entry.TargetID != packageID || entry.TargetType != itemTarget {
		t.Errorf("the entry points at %s %s", entry.TargetType, entry.TargetID)
	}

	recorded, present := entry.Changes[domain.FieldTitle].(map[string]any)
	if !present {
		t.Fatalf("the title is not in the entry: %v", entry.Changes)
	}
	if recorded["changed"] != true {
		t.Errorf("the title is recorded as %v", recorded)
	}
	for _, side := range []string{"to", "from"} {
		if value, present := recorded[side]; present {
			t.Errorf("the entry keeps the %s value in clear text: %v", side, value)
		}
	}
	if recorded["to_hash"] == nil || recorded["from_hash"] == nil {
		t.Errorf("the entry has no hash to compare two versions by: %v", recorded)
	}
}

// The permission question is asked on the collection's path and before the transaction opens, so
// that the audit entry a refusal writes is not rolled back with it (audit.md §7).
func TestThePermissionIsAskedOnThePathBeforeTheWrite(t *testing.T) {
	h := newUpdateHarness()
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.denied")

	_, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID, Attributes: domain.ItemAttributes{Title: said("Order the longer cable")},
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a refused actor answered %v", err)
	}

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d permission questions", len(h.authorizer.requests))
	}
	request := h.authorizer.requests[0]
	if request.Action != ItemUpdatedAction || request.TargetID != packageID {
		t.Errorf("the question was %+v", request)
	}
	if len(request.Path) == 0 {
		t.Error("the question named no path, so a membership at the hub could not apply")
	}
	if h.uow.writes != 0 {
		t.Error("a write transaction was opened for a refused actor")
	}
}

// I-C3: an archived collection is read-only, and its items inherit that.
func TestAnArchivedCollectionRefusesTheUpdate(t *testing.T) {
	h := newUpdateHarness()
	collection := h.containers.stored[collectionID]
	archivedAt := now
	collection.ArchivedAt = &archivedAt
	h.containers.stored[collectionID] = collection

	_, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID, Attributes: domain.ItemAttributes{Title: said("Order the longer cable")},
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("an archived collection answered %v", err)
	}
	if len(h.items.attributes) != 0 {
		t.Error("the write happened anyway")
	}
}

// I-W4, one level down: the item's own state.
func TestAnArchivedItemRefusesTheUpdate(t *testing.T) {
	h := newUpdateHarness()
	item := h.items.stored[packageID]
	archivedAt := now
	item.ArchivedAt = &archivedAt
	h.items.stored[packageID] = item

	_, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID, Attributes: domain.ItemAttributes{Title: said("Order the longer cable")},
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("an archived item answered %v", err)
	}
	if len(h.items.attributes) != 0 {
		t.Error("the write happened anyway")
	}
}

func TestAnItemThatDoesNotExistIsSaidSo(t *testing.T) {
	h := newUpdateHarness()
	absent := shared.MustParseID("0192f000-0000-7000-8000-0000000009ff")

	_, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: absent, Attributes: domain.ItemAttributes{Title: said("Order the longer cable")},
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing item answered %v", err)
	}
}

func TestTheItemHasToBeNamed(t *testing.T) {
	h := newUpdateHarness()

	_, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		Attributes: domain.ItemAttributes{Title: said("Order the longer cable")},
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a nameless item answered %v", err)
	}
}

// The catalogue's untyped door, which is the one MCP and automation come through. The two fields
// have to survive it as the two instructions they are: sent-and-empty, and not sent at all.
func TestTheUntypedInputTellsAnEmptyNoteFromAnAbsentOne(t *testing.T) {
	h := newUpdateHarness()

	out, err := h.handler.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{
		"item_id": packageID.String(), "notes": "",
	})
	if err != nil {
		t.Fatalf("clearing the notes through the catalogue was refused: %v", err)
	}
	if h.written(t).item.Notes != "" {
		t.Error("an empty note was read as an absent one")
	}
	if out.String("title") != "Order the cable" {
		t.Errorf("the answer renamed the item to %q", out.String("title"))
	}
}

// An update that names no field at all is a request with no instruction in it. Refused rather than
// answered with an unchanged item, because the two are different: one is a caller that meant
// something and spelled it wrongly, the other is a caller that asked for what is already there.
func TestAnUpdateWithNoFieldsIsRefused(t *testing.T) {
	h := newUpdateHarness()

	_, err := h.handler.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{
		"item_id": packageID.String(),
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an empty update answered %v", err)
	}
}

// The If-Match arrives through the catalogue as a number, whichever channel read it.
func TestTheExpectedVersionArrivesThroughTheCatalogue(t *testing.T) {
	h := newUpdateHarness()

	_, err := h.handler.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{
		"item_id": packageID.String(), "title": "Order the longer cable", "expected_version": 2,
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version through the catalogue answered %v", err)
	}
}

// The declaration the audit gate reads (AU-1), and the scope a token needs.
func TestTheDescriptorDeclaresWhatTheGatesRead(t *testing.T) {
	descriptor := UpdateWorkItem{}.Descriptor()

	if descriptor.Name != UpdateWorkItemName {
		t.Errorf("the use case is called %q", descriptor.Name)
	}
	if descriptor.TokenScope != itemsWrite {
		t.Errorf("the token scope is %q", descriptor.TokenScope)
	}
	if descriptor.Audit.Action != ItemUpdatedAction || !descriptor.Audit.Required {
		t.Errorf("the audit declaration is %+v", descriptor.Audit)
	}

	declared := map[string]bool{}
	for _, field := range descriptor.Input {
		declared[field.Name] = true
	}
	for _, field := range []string{"item_id", "title", "notes", "expected_version"} {
		if !declared[field] {
			t.Errorf("the input does not declare %q", field)
		}
	}
}

// The board an entry sits on, through the update (B-09). Empty is not "leave it alone": it takes
// the entry off the board, which is the same distinction the text fields keep.
func TestUpdatingTheColumnAnEntrySitsIn(t *testing.T) {
	h := newUpdateHarness()
	board := h.withBucketOn(collectionID)

	t.Run("into a column of this collection's board", func(t *testing.T) {
		out, err := h.handler.invoke(t.Context(), actorFixture(), usecase.Input{
			"item_id": taskID.String(), "bucket_id": board.ID.String(),
		})
		if err != nil {
			t.Fatalf("the update failed: %v", err)
		}
		if out["bucket_id"] != board.ID.String() {
			t.Errorf("bucket_id is %v", out["bucket_id"])
		}
	})

	t.Run("a column nobody named leaves it where it is", func(t *testing.T) {
		out, err := h.handler.invoke(t.Context(), actorFixture(), usecase.Input{
			"item_id": taskID.String(), "title": "Buy oat milk",
		})
		if err != nil {
			t.Fatalf("the update failed: %v", err)
		}
		if out["bucket_id"] != board.ID.String() {
			t.Errorf("the entry left its column: %v", out["bucket_id"])
		}
	})

	t.Run("off the board again", func(t *testing.T) {
		out, err := h.handler.invoke(t.Context(), actorFixture(), usecase.Input{
			"item_id": taskID.String(), "bucket_id": "",
		})
		if err != nil {
			t.Fatalf("the update failed: %v", err)
		}
		if out["bucket_id"] != nil {
			t.Errorf("bucket_id is %v, want null", out["bucket_id"])
		}
	})
}

// Invariant I-W6: a column belongs to the collection whose board it is on, and one from elsewhere
// would put the entry on a board it is not on.
func TestAColumnFromAnotherCollectionIsRefused(t *testing.T) {
	h := newUpdateHarness()
	elsewhere := h.withBucketOn(shared.MustParseID("0192f000-0000-7000-8000-0000000009cf"))

	_, err := h.handler.invoke(t.Context(), actorFixture(), usecase.Input{
		"item_id": taskID.String(), "bucket_id": elsewhere.ID.String(),
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a foreign column was accepted: %v", err)
	}
	if shared.AsError(err).DetailCode != "buckets.not_in_collection" {
		t.Errorf("detail code %s", shared.AsError(err).DetailCode)
	}
}

// A board belongs to a collection, so only the entries directly in it have a place on one. The
// capability matrix says the same thing about the type, and it is what refuses this.
func TestAWorkPackageCannotBePutOnABoard(t *testing.T) {
	h := newUpdateHarness()
	board := h.withBucketOn(collectionID)

	_, err := h.handler.invoke(t.Context(), actorFixture(), usecase.Input{
		"item_id": packageID.String(), "bucket_id": board.ID.String(),
	})
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("a work package was put on a board: %v", err)
	}
}

// A column that is no longer on the board is refused: an entry put into one would be nowhere a
// client draws.
func TestADeletedColumnCannotTakeAnEntry(t *testing.T) {
	h := newUpdateHarness()
	board := h.withBucketOn(collectionID)
	deleted, _, err := board.Deleted(updateNow)
	if err != nil {
		t.Fatalf("deleting the column: %v", err)
	}
	h.buckets.stored[board.ID] = deleted

	_, err = h.handler.invoke(t.Context(), actorFixture(), usecase.Input{
		"item_id": taskID.String(), "bucket_id": board.ID.String(),
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a deleted column took an entry: %v", err)
	}
}

// The history keeps what the audit trail deliberately does not. A rename means nothing without both
// titles, and the trail records them as hashes because it outlives the item; the history is deleted
// with the item and can therefore say what a person opening it wants to read (audit.md §1).
func TestTheHistoryKeepsBothTitlesAndOnlyThatTheNoteChanged(t *testing.T) {
	h := newUpdateHarness()

	_, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID,
		Attributes: domain.ItemAttributes{
			Title: said("Order the longer cable"),
			Notes: said("Five metres, and the right connector this time."),
		},
	})
	if err != nil {
		t.Fatalf("the update was refused: %v", err)
	}

	step := h.history.only(t)
	if step.Verb != activity.ItemUpdated || step.ItemID != packageID {
		t.Fatalf("the history recorded %s about %s", step.Verb, step.ItemID)
	}

	title, _ := step.ChangeSet[domain.FieldTitle].(map[string]any)
	if title["from"] != "Order the cable" || title["to"] != "Order the longer cable" {
		t.Errorf("the rename reads %v, want both titles", title)
	}

	// The other way round for the note: a note can be a page of text, and its history is that
	// somebody edited it.
	notes, _ := step.ChangeSet[domain.FieldNotes].(map[string]any)
	if notes["changed"] != true || len(notes) != 1 {
		t.Errorf("the note reads %v, want that it changed and nothing else", notes)
	}
}

// "Compact history for activities" (the capability matrix, domain-model.md §2): the verb, the actor
// and the time, without the fields that moved. An activity carries none of the fields a diff would
// be about, so its history says what happened and leaves the reading of it to the entry itself.
func TestAnActivitysHistoryIsCompact(t *testing.T) {
	h := newUpdateHarness()

	if _, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID:     activityID,
		Attributes: domain.ItemAttributes{Title: said("Semi-skimmed, two litres")},
	}); err != nil {
		t.Fatalf("the update was refused: %v", err)
	}

	step := h.history.only(t)
	if step.Verb != activity.ItemUpdated {
		t.Fatalf("the history recorded %s", step.Verb)
	}
	if len(step.ChangeSet) != 0 {
		t.Errorf("the compact history holds %v, want the verb alone", step.ChangeSet)
	}
}

// An update that asks for what is already stored writes nothing, and that includes the history: a
// client echoing the whole object back must not fill a history with steps in which nothing happened.
func TestAnUpdateThatChangesNothingLeavesNoStepInTheHistory(t *testing.T) {
	h := newUpdateHarness()

	stored := h.items.stored[packageID]
	if _, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
		ItemID: packageID, Attributes: domain.ItemAttributes{Title: said(stored.Title)},
	}); err != nil {
		t.Fatalf("the update was refused: %v", err)
	}

	if len(h.history.entries) != 0 {
		t.Errorf("the history holds %v, want nothing", h.history.verbs())
	}
}
