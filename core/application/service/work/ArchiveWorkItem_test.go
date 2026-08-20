// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// lifecycleHarness is one collection with one task in it: the smallest shape every lifecycle verb
// needs. The steps that follow reuse it for the trash, which is the point of it being here.
type lifecycleHarness struct {
	writer     LifecycleWriter
	items      *items
	containers *containers
	events     *events
	changes    *changes
	audit      *sink
	history    *journal
	authorizer *authorizer
	uow        *unitOfWork
	jobs       *jobs
}

func newLifecycleHarness() *lifecycleHarness {
	store := &items{
		stored:   map[shared.ID]domain.WorkItem{},
		children: map[shared.ID]domain.ChildCompletion{},
	}
	containerStore := &containers{stored: map[shared.ID]domain.Container{}}

	h := &lifecycleHarness{
		items: store, containers: containerStore,
		events: &events{}, changes: &changes{}, audit: &sink{}, history: &journal{},
		authorizer: &authorizer{}, uow: &unitOfWork{}, jobs: &jobs{},
	}
	h.writer = LifecycleWriter{
		Items: store, Containers: containerStore, Authorizer: h.authorizer,
		Events: h.events, Changes: h.changes, Audit: h.audit,
		Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
		Queue: h.jobs,
	}

	containerStore.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	containerStore.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	store.stored[taskID] = domain.WorkItem{
		ID: taskID, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemTask,
		Path: domain.RootPath(taskID), Depth: 1, Title: "Weekly shop", OrderKey: "a0",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	return h
}

func (h *lifecycleHarness) archive() ArchiveWorkItem { return ArchiveWorkItem{Lifecycle: h.writer} }
func (h *lifecycleHarness) unarchive() UnarchiveWorkItem {
	return UnarchiveWorkItem{Lifecycle: h.writer}
}

// One archive owes four things, and this is where they are counted: the stamp on the row, the event
// outwards, the change an offline client reads, and the audit entry.
func TestArchivingAnEntryWritesTheStampAndRecordsWhatItOwes(t *testing.T) {
	h := newLifecycleHarness()

	item, err := h.archive().Execute(t.Context(), itemActor(), LifecycleCommand{ItemID: taskID})
	if err != nil {
		t.Fatalf("archiving was refused: %v", err)
	}

	if !item.IsArchived() {
		t.Error("the entry came back unarchived")
	}
	if item.Version != 2 {
		t.Errorf("version %d, want 2", item.Version)
	}
	if stored := h.items.stored[taskID]; !stored.IsArchived() {
		t.Error("the stamp was not written")
	}

	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ItemArchived {
		t.Errorf("the events are %v, want one %s", h.events.appended, event.ItemArchived)
	}
	if len(h.changes.recorded) != 1 {
		t.Fatalf("%d changes recorded, want 1", len(h.changes.recorded))
	}
	if change := h.changes.recorded[0]; change.Op != changelog.Upsert || change.EntityID != taskID {
		t.Errorf("the change is %+v, want an upsert of the entry", change)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ItemArchivedAction {
		t.Errorf("the audit entries are %v, want one %s", h.audit.entries, ItemArchivedAction)
	}
	if !h.uow.committed {
		t.Error("the transaction was not committed")
	}
}

// The permission question is asked before the transaction opens, so that a refusal's audit entry
// survives (audit.md §7) - and it is asked against the path, because a membership at the hub applies
// downwards.
func TestArchivingAsksThePermissionAgainstTheWholePath(t *testing.T) {
	h := newLifecycleHarness()

	if _, err := h.archive().Execute(
		t.Context(), itemActor(), LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("archiving was refused: %v", err)
	}

	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d permission questions, want 1", len(h.authorizer.requests))
	}
	request := h.authorizer.requests[0]
	if request.Permission != service.PermissionWriteItems {
		t.Errorf("permission %s, want %s", request.Permission, service.PermissionWriteItems)
	}
	if request.Action != ItemArchivedAction {
		t.Errorf("action %s, want %s", request.Action, ItemArchivedAction)
	}
	// The tenant, the hub and the collection: a membership held at any of the three applies
	// downwards, and a path that stopped at the collection would refuse somebody who holds the right
	// higher up (domain-model.md §3.2).
	if len(request.Path) != 3 {
		t.Fatalf("the path is %v, want three levels", request.Path)
	}
	if request.Path[1].ID != hubID || request.Path[2].ID != collectionID {
		t.Errorf("the path is %v, want the hub and then the collection", request.Path)
	}
}

// A refusal writes nothing at all - not the stamp, not the event, not the change.
func TestARefusedArchiveWritesNothing(t *testing.T) {
	h := newLifecycleHarness()
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	if _, err := h.archive().Execute(
		t.Context(), itemActor(), LifecycleCommand{ItemID: taskID},
	); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the refusal reported %v", err)
	}
	if h.items.stored[taskID].IsArchived() {
		t.Error("a refused archive wrote the stamp")
	}
	if len(h.events.appended) != 0 || len(h.changes.recorded) != 0 {
		t.Error("a refused archive announced something")
	}
	if h.uow.writes != 0 {
		t.Error("a refused archive opened a write transaction")
	}
}

// Idempotence, and what it costs: nothing. A second archive spends no version, writes no event, and
// records nothing - which is what makes a retry after a lost response harmless rather than merely
// accepted.
func TestArchivingTwiceChangesNothingTheSecondTime(t *testing.T) {
	h := newLifecycleHarness()
	ctx, actor := t.Context(), itemActor()

	first, err := h.archive().Execute(ctx, actor, LifecycleCommand{ItemID: taskID})
	if err != nil {
		t.Fatalf("the first archive was refused: %v", err)
	}

	second, err := h.archive().Execute(ctx, actor, LifecycleCommand{ItemID: taskID})
	if err != nil {
		t.Fatalf("the second archive was refused: %v", err)
	}

	if second.Version != first.Version {
		t.Errorf("the second archive spent a version: %d then %d", first.Version, second.Version)
	}
	if !second.ArchivedAt.Equal(*first.ArchivedAt) {
		t.Error("the second archive re-dated the first")
	}
	if len(h.events.appended) != 1 {
		t.Errorf("%d events, want 1", len(h.events.appended))
	}
	if len(h.audit.entries) != 1 {
		t.Errorf("%d audit entries, want 1", len(h.audit.entries))
	}
}

// The If-Match is honoured even when the change itself would have been a no-op: the state the caller
// was reasoning about is not the state that is there, and that is worth being told about
// (api-guidelines.md §5).
func TestAStaleVersionIsRefusedEvenWhenTheArchiveWouldChangeNothing(t *testing.T) {
	h := newLifecycleHarness()
	ctx, actor := t.Context(), itemActor()

	if _, err := h.archive().Execute(ctx, actor, LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("the first archive was refused: %v", err)
	}

	_, err := h.archive().Execute(ctx, actor, LifecycleCommand{ItemID: taskID, ExpectedVersion: 1})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version on a no-op reported %v", err)
	}
}

// I-C3 reaches the entries: a collection in an archived hub is read-only, and archiving something
// inside it is a write into a subtree nobody may write to.
func TestArchivingIsRefusedInsideAnArchivedSubtree(t *testing.T) {
	h := newLifecycleHarness()
	archivedAt := now.Add(-time.Hour)
	collection := h.containers.stored[collectionID]
	collection.ParentArchivedAt = &archivedAt
	h.containers.stored[collectionID] = collection

	_, err := h.archive().Execute(t.Context(), itemActor(), LifecycleCommand{ItemID: taskID})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("archiving inside an archived hub reported %v, want a conflict", err)
	}
}

// The archive is a decision about work that is still there. An entry on its way out of the system is
// not something to take out of use, and the answer names the restore that would help.
func TestArchivingAnEntryInTheTrashIsAConflict(t *testing.T) {
	h := newLifecycleHarness()
	trashed := h.items.stored[taskID]
	deletedAt := now.Add(-time.Hour)
	trashed.DeletedAt = &deletedAt
	trashed.TrashBatchID = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	h.items.stored[taskID] = trashed

	_, err := h.archive().Execute(t.Context(), itemActor(), LifecycleCommand{ItemID: taskID})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("archiving a trashed entry reported %v, want a conflict", err)
	}
	if detail := shared.AsError(err).DetailCode; detail != "items.trashed" {
		t.Errorf("the detail code is %q, want items.trashed", detail)
	}
}

// And back out again: the stamp is lifted, and the event says so rather than saying "restored" -
// that name belongs to the trash.
func TestUnarchivingLiftsTheStampAndAnnouncesItsOwnEvent(t *testing.T) {
	h := newLifecycleHarness()
	ctx, actor := t.Context(), itemActor()

	if _, err := h.archive().Execute(ctx, actor, LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("archiving was refused: %v", err)
	}

	item, err := h.unarchive().Execute(ctx, actor, LifecycleCommand{ItemID: taskID})
	if err != nil {
		t.Fatalf("unarchiving was refused: %v", err)
	}
	if item.IsArchived() {
		t.Error("the entry is still archived")
	}
	if len(h.events.appended) != 2 || h.events.appended[1].Type != event.ItemUnarchived {
		t.Errorf("the second event is %v, want %s", h.events.appended, event.ItemUnarchived)
	}
	if h.audit.entries[1].Action != ItemUnarchivedAction {
		t.Errorf("the audit action is %s, want %s", h.audit.entries[1].Action, ItemUnarchivedAction)
	}
}

// Unarchiving something that is not archived is the state the caller asked for, and costs nothing.
func TestUnarchivingSomethingThatIsNotArchivedChangesNothing(t *testing.T) {
	h := newLifecycleHarness()

	item, err := h.unarchive().Execute(t.Context(), itemActor(), LifecycleCommand{ItemID: taskID})
	if err != nil {
		t.Fatalf("unarchiving was refused: %v", err)
	}
	if item.Version != 1 {
		t.Errorf("version %d, want 1 - nothing should have been spent", item.Version)
	}
	if len(h.events.appended) != 0 {
		t.Errorf("%d events, want none", len(h.events.appended))
	}
}

// The audit entry answers "who took this out of use, and when" and carries no user content: no
// title, no notes (rule 10). Both stamps travel, because an entry can be archived and in the trash
// at once and one line should say which.
func TestTheArchiveAuditEntryCarriesTheStampsAndNoUserContent(t *testing.T) {
	h := newLifecycleHarness()

	if _, err := h.archive().Execute(
		t.Context(), itemActor(), LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("archiving was refused: %v", err)
	}

	// Changes() masks per field and hands back `{field: {"to": …}}`, which is the shape the column
	// holds - so the assertion reads the trail as an auditor would rather than as the caller wrote it.
	recorded := h.audit.entries[0].Changes
	for _, field := range []string{"type", "collection_id", "archived_at", "deleted_at"} {
		if _, present := recorded[field]; !present {
			t.Errorf("the audit entry does not record %s", field)
		}
	}
	for field, entry := range recorded {
		if to, _ := entry.(map[string]any)["to"].(string); to == "Weekly shop" {
			t.Errorf("the entry's title reached the audit trail as %s", field)
		}
	}
	if to, _ := recorded["archived_at"].(map[string]any)["to"].(string); to == "" {
		t.Error("the audit entry records no archive stamp")
	}
	if to, _ := recorded["deleted_at"].(map[string]any)["to"].(string); to != "" {
		t.Errorf("the audit entry claims a deletion: %q", to)
	}
}

// All three channels reach the same handler, which is the whole point of the catalogue (arc42 §4).
// The catalogue's own validation is what refuses a missing identifier, so the use case does not
// repeat it - but the handler still has to read the field it declared.
func TestTheArchiveVerbsAreReachableThroughTheCatalogue(t *testing.T) {
	h := newLifecycleHarness()

	for _, c := range []struct {
		name       string
		descriptor usecase.Descriptor
	}{
		{"archive", h.archive().Descriptor()},
		{"unarchive", h.unarchive().Descriptor()},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.descriptor.Handler == nil {
				t.Fatal("the descriptor carries no handler")
			}
			out, err := c.descriptor.Handler.Invoke(t.Context(), itemActor(), usecase.Input{
				"item_id": taskID.String(),
			})
			if err != nil {
				t.Fatalf("invoking through the catalogue: %v", err)
			}
			if out.String("id") != taskID.String() {
				t.Errorf("the answer is about %q, want %q", out.String("id"), taskID)
			}
		})
	}
}

// The four lifecycle verbs each leave their own step. Four verbs rather than two with a direction,
// because a person reading a history has to be able to tell an archive from its undoing without
// opening the change set.
func TestEachLifecycleVerbLeavesItsOwnStepInTheHistory(t *testing.T) {
	cases := []struct {
		name string
		run  func(*testing.T, *lifecycleHarness) error
		want activity.Verb
	}{
		{"archived", func(t *testing.T, h *lifecycleHarness) error {
			_, err := h.archive().Execute(t.Context(), itemActor(), LifecycleCommand{ItemID: taskID})
			return err
		}, activity.ItemArchived},
		{"unarchived", func(t *testing.T, h *lifecycleHarness) error {
			if _, err := h.archive().Execute(
				t.Context(), itemActor(), LifecycleCommand{ItemID: taskID}); err != nil {
				return err
			}
			_, err := h.unarchive().Execute(t.Context(), itemActor(), LifecycleCommand{ItemID: taskID})
			return err
		}, activity.ItemUnarchived},
		{"trashed", func(t *testing.T, h *lifecycleHarness) error {
			_, err := h.trash().Execute(t.Context(), itemActor(), LifecycleCommand{ItemID: taskID})
			return err
		}, activity.ItemTrashed},
		{"restored", func(t *testing.T, h *lifecycleHarness) error {
			if _, err := h.trash().Execute(
				t.Context(), itemActor(), LifecycleCommand{ItemID: taskID}); err != nil {
				return err
			}
			_, err := h.restore().Execute(t.Context(), itemActor(), LifecycleCommand{ItemID: taskID})
			return err
		}, activity.ItemRestored},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newLifecycleHarness()
			if err := c.run(t, h); err != nil {
				t.Fatalf("%s was refused: %v", c.name, err)
			}

			last := h.history.entries[len(h.history.entries)-1]
			if last.Verb != c.want {
				t.Errorf("the history reads %v, want %s last", h.history.verbs(), c.want)
			}
			// The verb is the whole of it: a diff of the stamp it wrote would say the same thing a
			// second time in a worse way.
			if len(last.ChangeSet) != 0 {
				t.Errorf("the step carries %v, want the verb alone", last.ChangeSet)
			}
		})
	}
}

// A deletion is one act and is recorded once, on the entry somebody deleted. One step on each of
// four hundred descendants would be four hundred steps nobody performed.
func TestADeletedSubtreeLeavesOneStepAndNotOnePerDescendant(t *testing.T) {
	h := newLifecycleHarness()
	h.withSubtree()

	if _, err := h.trash().Execute(
		t.Context(), itemActor(), LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("trashing was refused: %v", err)
	}

	step := h.history.only(t)
	if step.ItemID != taskID {
		t.Errorf("the step is about %s, want the entry the deletion named", step.ItemID)
	}
}

// A verb that changes nothing announces nothing, and that includes the history: archiving something
// twice is a retry, not a second act.
func TestArchivingTwiceLeavesOneStepInTheHistory(t *testing.T) {
	h := newLifecycleHarness()

	for range 2 {
		if _, err := h.archive().Execute(
			t.Context(), itemActor(), LifecycleCommand{ItemID: taskID}); err != nil {
			t.Fatalf("archiving was refused: %v", err)
		}
	}

	if step := h.history.only(t); step.Verb != activity.ItemArchived {
		t.Errorf("the history recorded %s", step.Verb)
	}
}
