// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	sourceTaskID = shared.MustParseID("0192f000-0000-7000-8000-000000000601")
	movedPackID  = shared.MustParseID("0192f000-0000-7000-8000-000000000602")
	leafID       = shared.MustParseID("0192f000-0000-7000-8000-000000000603")
	targetTaskID = shared.MustParseID("0192f000-0000-7000-8000-000000000604")
)

// placementHarness is a task with a work package under it and an activity under that, plus a second task to
// move things to. The shape a move has to walk, and small enough to assert on exactly.
type placementHarness struct {
	writer     PlacementWriter
	items      *items
	containers *containers
	events     *events
	changes    *changes
	audit      *sink
	authorizer *authorizer
	uow        *unitOfWork
}

func newPlacementHarness() *placementHarness {
	store := &items{stored: map[shared.ID]domain.WorkItem{}}
	containerStore := &containers{stored: map[shared.ID]domain.Container{}}

	h := &placementHarness{
		items: store, containers: containerStore,
		events: &events{}, changes: &changes{}, audit: &sink{},
		authorizer: &authorizer{}, uow: &unitOfWork{},
	}
	h.writer = PlacementWriter{
		Items: store, Containers: containerStore, Profiles: &profiles{rows: systemProfiles()},
		Authorizer: h.authorizer, Events: h.events, Changes: h.changes, Audit: h.audit,
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}

	containerStore.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	containerStore.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}

	source := placedItem(sourceTaskID, domain.ItemTask, "", domain.RootPath(sourceTaskID), 1)
	target := placedItem(targetTaskID, domain.ItemTask, "", domain.RootPath(targetTaskID), 1)
	pack := placedItem(movedPackID, domain.ItemWorkPackage, sourceTaskID, source.ChildPath(movedPackID), 2)
	leaf := placedItem(leafID, domain.ItemActivity, movedPackID, pack.ChildPath(leafID), 3)
	for _, item := range []domain.WorkItem{source, target, pack, leaf} {
		store.stored[item.ID] = item
	}
	return h
}

func placedItem(id shared.ID, itemType domain.ItemType, parent shared.ID, path string, depth int) domain.WorkItem {
	return domain.WorkItem{
		ID: id, TenantID: tenantID, CollectionID: collectionID, Type: itemType, ParentID: parent,
		Path: path, Depth: depth, Title: "Something", OrderKey: "a0",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
}

func placementActor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantID, AccountID: accountID,
	}
}

// The acceptance criterion, and the invariant that has to be impossible rather than forbidden: the refusal
// reaches the caller and nothing is written.
func TestMovingIntoItsOwnSubtreeWritesNothing(t *testing.T) {
	h := newPlacementHarness()

	_, err := MoveWorkItem{Placement: h.writer}.Execute(t.Context(), placementActor(), MoveWorkItemCommand{
		ItemID: movedPackID, TargetParentID: leafID, ParentGiven: true,
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("answered %v, want a validation error", err)
	}
	if got := shared.AsError(err).DetailCode; got != "items.parent_in_own_subtree" {
		t.Errorf("the detail code is %q", got)
	}
	if len(h.items.moves) != 0 || len(h.items.ranks) != 0 {
		t.Error("a refused move wrote something")
	}
	if len(h.events.appended) != 0 || len(h.audit.entries) != 0 {
		t.Error("a refused move announced or recorded something")
	}
}

// The acceptance criterion for the subtree: the move carries the prefixes the rewrite needs, and it is one
// call inside one transaction.
func TestMovingRewritesTheSubtreeInOneTransaction(t *testing.T) {
	h := newPlacementHarness()
	h.items.previousKey = "a0"
	h.items.subtreeSize = 2

	result, err := MoveWorkItem{Placement: h.writer}.Execute(t.Context(), placementActor(), MoveWorkItemCommand{
		ItemID: movedPackID, TargetParentID: targetTaskID, ParentGiven: true,
	})
	if err != nil {
		t.Fatalf("moving: %v", err)
	}

	if len(h.items.moves) != 1 {
		t.Fatalf("%d subtree moves, want 1", len(h.items.moves))
	}
	move := h.items.moves[0]

	source := h.items.stored[sourceTaskID]
	target := h.items.stored[targetTaskID]
	if move.OldPrefix != source.ChildPath(movedPackID) {
		t.Errorf("the old prefix is %q", move.OldPrefix)
	}
	if want := target.ChildPath(movedPackID); move.NewPrefix != want {
		t.Errorf("the new prefix is %q, want %q", move.NewPrefix, want)
	}
	// Both destinations are tasks at depth 1, so the subtree does not shift.
	if move.DepthDelta != 0 {
		t.Errorf("the depth delta is %d, want 0", move.DepthDelta)
	}
	if move.TargetParentID != targetTaskID {
		t.Errorf("the target parent is %s", move.TargetParentID)
	}
	if result.SubtreeSize != 2 {
		t.Errorf("the result reports %d rows moved, want 2", result.SubtreeSize)
	}
	if !h.uow.committed {
		t.Error("the transaction did not commit")
	}
	// A move is a write, and reading the state it decides from happens in a read-only transaction first.
	if h.uow.writes != 1 {
		t.Errorf("%d write transactions, want 1", h.uow.writes)
	}
}

// A reorder is a move that keeps its parent, and the difference shows in what it writes: one row, not a
// subtree. That is the whole reason the two are separate use cases over one dependency set.
func TestReorderingWritesOneRowAndNoSubtree(t *testing.T) {
	h := newPlacementHarness()
	h.items.previousKey, h.items.nextKey = "a0", "a1"

	item, err := ReorderWorkItem{Placement: h.writer}.
		Execute(t.Context(), placementActor(), ReorderWorkItemCommand{ItemID: leafID})
	if err != nil {
		t.Fatalf("reordering: %v", err)
	}

	if len(h.items.moves) != 0 {
		t.Errorf("a reorder rewrote the subtree: %+v", h.items.moves)
	}
	if len(h.items.ranks) != 1 {
		t.Fatalf("%d rank writes, want 1", len(h.items.ranks))
	}
	// The rank comes from the ordering service over the neighbours the database reported, never from the
	// client: an insertion between two keys is what renumbers nothing else (offline-sync.md §4.2).
	written := h.items.ranks[0].item
	if written.OrderKey <= "a0" || written.OrderKey >= "a1" {
		t.Errorf("the new rank is %q, which is not between a0 and a1", written.OrderKey)
	}
	if item.OrderKey != written.OrderKey {
		t.Errorf("the answer says %q and the write said %q", item.OrderKey, written.OrderKey)
	}
	// The parent and the path are untouched, which is what "nothing moved" means.
	if written.ParentID != movedPackID || written.Path != h.items.stored[leafID].Path {
		t.Errorf("a reorder changed the placement: %+v", written)
	}
}

// The position is measured at the destination level, with the moving item excluded from its own level.
func TestThePositionIsMeasuredAtTheDestination(t *testing.T) {
	h := newPlacementHarness()
	h.items.previousKey = "a0"

	if _, err := (MoveWorkItem{Placement: h.writer}).Execute(t.Context(), placementActor(), MoveWorkItemCommand{
		ItemID: movedPackID, TargetParentID: targetTaskID, ParentGiven: true,
	}); err != nil {
		t.Fatalf("moving: %v", err)
	}

	if h.items.askedLevel.ParentID != targetTaskID {
		t.Errorf("the neighbours were read at parent %s, want %s", h.items.askedLevel.ParentID, targetTaskID)
	}
	if h.items.askedLevel.CollectionID != collectionID {
		t.Errorf("the neighbours were read in collection %s", h.items.askedLevel.CollectionID)
	}
}

// A sibling named for a position has to be at that level. Appending silently would ignore a client that asked
// for a place.
func TestAPositionNamingAStrangerIsRefused(t *testing.T) {
	h := newPlacementHarness()
	// Nothing at the destination level, so the named sibling cannot be found there.
	h.items.previousKey, h.items.nextKey = "", ""

	_, err := MoveWorkItem{Placement: h.writer}.Execute(t.Context(), placementActor(), MoveWorkItemCommand{
		ItemID: movedPackID, TargetParentID: targetTaskID, ParentGiven: true, BeforeItemID: leafID,
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("answered %v", err)
	}
	if got := shared.AsError(err).DetailCode; got != "items.before_item_not_in_level" {
		t.Errorf("the detail code is %q", got)
	}
}

// Omitting the parent leaves it alone; sending it as null asks for the top level and needs a collection. The
// two are different requests and the value cannot tell them apart, which is what ParentGiven is for.
func TestAnOmittedParentIsNotARequestForTheTopLevel(t *testing.T) {
	t.Run("omitted keeps the parent", func(t *testing.T) {
		h := newPlacementHarness()
		h.items.previousKey = "a0"

		if _, err := (MoveWorkItem{Placement: h.writer}).Execute(
			t.Context(), placementActor(), MoveWorkItemCommand{ItemID: movedPackID},
		); err != nil {
			t.Fatalf("moving: %v", err)
		}
		// Nothing reparented, so it is the one-row path.
		if len(h.items.moves) != 0 || len(h.items.ranks) != 1 {
			t.Errorf("an omitted parent moved the subtree: %d moves, %d ranks",
				len(h.items.moves), len(h.items.ranks))
		}
	})

	t.Run("null needs a collection", func(t *testing.T) {
		h := newPlacementHarness()

		_, err := MoveWorkItem{Placement: h.writer}.Execute(t.Context(), placementActor(), MoveWorkItemCommand{
			ItemID: movedPackID, ParentGiven: true,
		})
		if !errors.Is(err, shared.ErrValidation) {
			t.Fatalf("answered %v", err)
		}
		if got := shared.AsError(err).DetailCode; got != "items.collection_or_parent_required" {
			t.Errorf("the detail code is %q", got)
		}
	})
}

// A collection that contradicts the chosen parent is refused rather than one of the two being preferred
// quietly.
func TestACollectionContradictingTheParentIsRefused(t *testing.T) {
	h := newPlacementHarness()
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000006ff")
	h.containers.stored[other] = domain.Container{
		ID: other, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Elsewhere", OrderKey: "a1", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}

	_, err := MoveWorkItem{Placement: h.writer}.Execute(t.Context(), placementActor(), MoveWorkItemCommand{
		ItemID: movedPackID, TargetParentID: targetTaskID, ParentGiven: true, TargetCollectionID: other,
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("answered %v", err)
	}
	if got := shared.AsError(err).DetailCode; got != "items.collection_contradicts_parent" {
		t.Errorf("the detail code is %q", got)
	}
}

// The move announces where the item came from, which is what lets a client rewrite its own subtree from one
// event rather than one per descendant.
func TestTheMoveAnnouncesWhereItCameFrom(t *testing.T) {
	h := newPlacementHarness()
	h.items.previousKey = "a0"
	before := h.items.stored[movedPackID]

	if _, err := (MoveWorkItem{Placement: h.writer}).Execute(t.Context(), placementActor(), MoveWorkItemCommand{
		ItemID: movedPackID, TargetParentID: targetTaskID, ParentGiven: true,
	}); err != nil {
		t.Fatalf("moving: %v", err)
	}

	if len(h.events.appended) != 1 {
		t.Fatalf("%d events, want 1", len(h.events.appended))
	}
	announcement := h.events.appended[0]
	if announcement.Type != event.ItemMoved {
		t.Errorf("the event type is %s", announcement.Type)
	}
	if announcement.Payload["from_path"] != before.Path {
		t.Errorf("from_path is %v, want %q", announcement.Payload["from_path"], before.Path)
	}
	if announcement.Payload["from_parent_id"] != sourceTaskID.String() {
		t.Errorf("from_parent_id is %v", announcement.Payload["from_parent_id"])
	}
	if announcement.Payload["to_parent_id"] != targetTaskID.String() {
		t.Errorf("to_parent_id is %v", announcement.Payload["to_parent_id"])
	}

	// One change log entry and one audit entry, in the same transaction (test AT-5), and no title in the trail
	// (rule 10).
	if len(h.changes.recorded) != 1 || len(h.audit.entries) != 1 {
		t.Errorf("%d change entries and %d audit entries", len(h.changes.recorded), len(h.audit.entries))
	}
	if _, present := h.audit.entries[0].Changes["title"]; present {
		t.Error("the audit entry carries the title")
	}
}

// A trashed or archived item is not editable except through Restore or Unarchive (I-W4).
func TestMovingATrashedOrArchivedItemIsRefused(t *testing.T) {
	for name, mutate := range map[string]func(*domain.WorkItem){
		"trashed":  func(item *domain.WorkItem) { item.DeletedAt = &now },
		"archived": func(item *domain.WorkItem) { item.ArchivedAt = &now },
	} {
		t.Run(name, func(t *testing.T) {
			h := newPlacementHarness()
			item := h.items.stored[movedPackID]
			mutate(&item)
			h.items.stored[movedPackID] = item

			_, err := MoveWorkItem{Placement: h.writer}.Execute(
				t.Context(), placementActor(), MoveWorkItemCommand{
					ItemID: movedPackID, TargetParentID: targetTaskID, ParentGiven: true,
				})
			if !errors.Is(err, shared.ErrConflict) {
				t.Fatalf("answered %v, want a conflict", err)
			}
			if len(h.items.moves) != 0 {
				t.Error("something was written")
			}
		})
	}
}

// A refusal writes nothing, and the permission question is asked before the transaction so the DENIED entry
// survives (audit.md §7).
func TestARefusedMoveWritesNothing(t *testing.T) {
	h := newPlacementHarness()
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	_, err := MoveWorkItem{Placement: h.writer}.Execute(t.Context(), placementActor(), MoveWorkItemCommand{
		ItemID: movedPackID, TargetParentID: targetTaskID, ParentGiven: true,
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("answered %v", err)
	}
	if h.uow.writes != 0 {
		t.Error("a refused move opened a write transaction")
	}
}

// A move within one collection asks one permission question; a move between collections asks two. Taking an
// item out of a collection is a change to that collection as much as to the destination.
func TestACrossCollectionMoveAsksAboutBothCollections(t *testing.T) {
	h := newPlacementHarness()
	h.items.previousKey = "a0"

	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000006ee")
	h.containers.stored[elsewhere] = domain.Container{
		ID: elsewhere, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Elsewhere", OrderKey: "a1", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	far := placedItem(
		shared.MustParseID("0192f000-0000-7000-8000-0000000006ed"), domain.ItemTask, "",
		domain.RootPath(shared.MustParseID("0192f000-0000-7000-8000-0000000006ed")), 1)
	far.CollectionID = elsewhere
	h.items.stored[far.ID] = far

	if _, err := (MoveWorkItem{Placement: h.writer}).Execute(t.Context(), placementActor(), MoveWorkItemCommand{
		ItemID: movedPackID, TargetParentID: far.ID, ParentGiven: true,
	}); err != nil {
		t.Fatalf("moving across collections: %v", err)
	}

	if len(h.authorizer.requests) != 2 {
		t.Fatalf("%d permission questions asked, want 2", len(h.authorizer.requests))
	}
	// The subtree carries the new collection with it.
	if len(h.items.moves) != 1 || h.items.moves[0].CollectionID != elsewhere {
		t.Errorf("the subtree moved to collection %+v", h.items.moves)
	}
}

// A board belongs to a collection, so only the entries directly in it have a place on one. A work
// package is refused by the capability rather than by a placeholder (B-09, domain-model.md §2).
func TestAWorkPackageHasNoPlaceOnABoard(t *testing.T) {
	h := newPlacementHarness()
	handler := MoveWorkItem{Placement: h.writer}

	_, err := handler.invoke(t.Context(), placementActor(), usecase.Input{
		"item_id":          movedPackID.String(),
		"target_bucket_id": shared.MustParseID("0192f000-0000-7000-8000-0000000006ec").String(),
	})
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("answered %v", err)
	}
	if got := shared.AsError(err).DetailCode; got != "items.capability_not_supported" {
		t.Errorf("the detail code is %q", got)
	}
}

// Both are writes and both declare their audit obligation, which gate SG-13 insists on. They share one action
// code, because an auditor asking "who moved this" means both.
func TestBothPlacementOperationsDeclareTheirAudit(t *testing.T) {
	for _, descriptor := range []usecase.Descriptor{
		MoveWorkItem{}.Descriptor(), ReorderWorkItem{}.Descriptor(),
	} {
		t.Run(descriptor.Name, func(t *testing.T) {
			if descriptor.ReadOnly {
				t.Error("a write is declared read-only")
			}
			if !descriptor.Audit.Required || descriptor.Audit.Action != ItemMovedAction {
				t.Errorf("the audit declaration is %+v", descriptor.Audit)
			}
			if descriptor.TokenScope != itemsWrite {
				t.Errorf("the token scope is %q", descriptor.TokenScope)
			}
		})
	}
}

// The output of a move is the contract's MoveResult shape, with the empty list present.
func TestTheMoveOutputCarriesAnEmptyDroppedReferences(t *testing.T) {
	h := newPlacementHarness()
	h.items.previousKey = "a0"

	out, err := MoveWorkItem{Placement: h.writer}.invoke(t.Context(), placementActor(), usecase.Input{
		"item_id":          movedPackID.String(),
		"target_parent_id": targetTaskID.String(),
	})
	if err != nil {
		t.Fatalf("moving: %v", err)
	}

	if _, ok := out["item"].(usecase.Output); !ok {
		t.Errorf("the output carries no item: %+v", out)
	}
	dropped, ok := out["dropped_references"].([]usecase.Output)
	if !ok {
		t.Fatalf("dropped_references is %T", out["dropped_references"])
	}
	if len(dropped) != 0 {
		t.Errorf("dropped_references is not empty: %+v", dropped)
	}
}

// The repository's level is what the neighbours are read at, and a reorder reads it at the item's own level.
func TestAReorderMeasuresItsOwnLevel(t *testing.T) {
	h := newPlacementHarness()
	h.items.previousKey = "a0"

	if _, err := (ReorderWorkItem{Placement: h.writer}).
		Execute(t.Context(), placementActor(), ReorderWorkItemCommand{ItemID: leafID}); err != nil {
		t.Fatalf("reordering: %v", err)
	}

	want := repository.Level{CollectionID: collectionID, ParentID: movedPackID}
	if h.items.askedLevel != want {
		t.Errorf("the neighbours were read at %+v, want %+v", h.items.askedLevel, want)
	}
}

// otherCollection puts a second collection and a task in it into the harness, and hands back the
// task: a move to another collection needs somewhere to move to.
func (h *placementHarness) otherCollection() domain.WorkItem {
	elsewhere := shared.MustParseID("0192f000-0000-7000-8000-0000000006ee")
	h.containers.stored[elsewhere] = domain.Container{
		ID: elsewhere, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Elsewhere", OrderKey: "a1", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	id := shared.MustParseID("0192f000-0000-7000-8000-0000000006ed")
	far := placedItem(id, domain.ItemTask, "", domain.RootPath(id), 1)
	far.CollectionID = elsewhere
	h.items.stored[id] = far
	return far
}

// A move to another collection takes the entry off the board it was on, and reports the loss: a
// board that silently reassigned somebody's cards would be indistinguishable from one that lost
// them (I-W6).
func TestAMoveToAnotherCollectionReportsWhatItDropped(t *testing.T) {
	h := newPlacementHarness()
	h.items.previousKey = "a0"
	far := h.otherCollection()

	lost := shared.MustParseID("0192f000-0000-7000-8000-0000000006e1")
	h.items.dropped = []domain.DroppedReference{domain.DroppedBucket(movedPackID, lost)}

	// The entry was on the board of the collection it is leaving.
	moving := h.items.stored[movedPackID]
	moving.BucketID = lost
	h.items.stored[movedPackID] = moving

	result, err := (MoveWorkItem{Placement: h.writer}).Execute(
		t.Context(), placementActor(), MoveWorkItemCommand{
			ItemID: movedPackID, TargetParentID: far.ID, ParentGiven: true,
		})
	if err != nil {
		t.Fatalf("the move was refused: %v", err)
	}

	if len(result.DroppedReferences) != 1 {
		t.Fatalf("the losses are %+v, want the column", result.DroppedReferences)
	}
	if reference := result.DroppedReferences[0]; reference.Kind != domain.ReferenceBucket ||
		reference.ID != lost || reference.ItemID != movedPackID {
		t.Errorf("unexpected loss: %+v", reference)
	}
	if !result.Item.BucketID.IsZero() {
		t.Errorf("the entry is still in column %s", result.Item.BucketID)
	}
}

// A move that resolves everything reports an empty list rather than none: a client that iterates
// the losses should not have to nil-check the field.
func TestAMoveThatDropsNothingReportsAnEmptyList(t *testing.T) {
	h := newPlacementHarness()
	h.items.previousKey = "a0"
	far := h.otherCollection()

	result, err := (MoveWorkItem{Placement: h.writer}).Execute(
		t.Context(), placementActor(), MoveWorkItemCommand{
			ItemID: movedPackID, TargetParentID: far.ID, ParentGiven: true,
		})
	if err != nil {
		t.Fatalf("the move was refused: %v", err)
	}
	if result.DroppedReferences == nil || len(result.DroppedReferences) != 0 {
		t.Errorf("the losses are %+v, want an empty list", result.DroppedReferences)
	}
}
