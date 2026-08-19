// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The three placement methods against a real database (B-08). Each gets a cross-tenant negative, because the
// tenant boundary is row level security underneath and the only way to know it reaches a new statement is to
// try it (gate SG-3).

// movableSubtree writes a task, a work package under it and an activity under that, and returns the three.
// Written directly rather than through the create use case, because these tests are about the statements and
// want the paths under their own control.
func movableSubtree(ctx context.Context, t *testing.T, tenant, author, collection shared.ID) (task, pack, activity work.WorkItem) {
	t.Helper()

	taskID, packID, activityID := freshID(t), freshID(t), freshID(t)
	task = taskIn(tenant, author, collection, taskID, freshName(t), "a0")
	pack = taskIn(tenant, author, collection, packID, freshName(t), "a0")
	pack.Type, pack.ParentID, pack.Path, pack.Depth = work.ItemWorkPackage, taskID, task.ChildPath(packID), 2
	activity = taskIn(tenant, author, collection, activityID, freshName(t), "a0")
	activity.Type, activity.ParentID = work.ItemActivity, packID
	activity.Path, activity.Depth = pack.ChildPath(activityID), 3

	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		items := itemRepo()
		for _, item := range []work.WorkItem{task, pack, activity} {
			if err := items.Insert(ctx, item); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the subtree: %v", err)
	}
	return task, pack, activity
}

// findWorkItem reads an item back, failing the test rather than returning an error: every caller here wants
// the item and none of them has anything to do with a failure.
func findWorkItem(ctx context.Context, t *testing.T, tenant, id shared.ID) work.WorkItem {
	t.Helper()

	var item work.WorkItem
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		item, err = itemRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading item %s: %v", id, err)
	}
	return item
}

// seedRootTask writes one task directly in a collection.
func seedRootTask(ctx context.Context, t *testing.T, tenant, author, collection shared.ID) shared.ID {
	t.Helper()

	id := freshID(t)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return itemRepo().
			Insert(ctx, taskIn(tenant, author, collection, id, freshName(t), "a0"))
	}); err != nil {
		t.Fatalf("seeding the task: %v", err)
	}
	return id
}

// stampItemColumn sets a lifecycle column the repository does not write. InsertWorkItem writes neither
// archived_at nor deleted_at - those are use cases of their own (B-06, B-10) - so a fixture that set the field
// on the struct would be silently dropped, and a test concluding anything from it would prove nothing.
func stampItemColumn(ctx context.Context, t *testing.T, id shared.ID, column string) {
	t.Helper()

	// The column name is a constant of this test and never a value from anywhere else, which is what keeps
	// CLAUDE.md rule 9 intact here, where sqlc cannot express "either of two columns".
	switch column {
	case "archived_at", "deleted_at":
	default:
		t.Fatalf("stampItemColumn was asked for the column %q, which it does not know", column)
	}

	tag, err := adminPool(ctx, t).Exec(ctx,
		"UPDATE work_item SET "+column+" = $2 WHERE id = $1", id.String(), created)
	if err != nil {
		t.Fatalf("stamping %s: %v", column, err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("stamping %s matched %d rows, want 1", column, tag.RowsAffected())
	}
}

// The acceptance criterion: the subtree's paths are rewritten in the same transaction as the move.
func TestMovingAnItemRewritesItsWholeSubtree(t *testing.T) {
	background := context.Background()
	collection := collectionFor(background, t, tenantA, authorA)
	_, pack, activity := movableSubtree(background, t, tenantA, authorA, collection)

	// A second task to move the work package under.
	destinationID := freshID(t)
	destination := taskIn(tenantA, authorA, collection, destinationID, freshName(t), "b0")
	if err := write(background, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, destination)
	}); err != nil {
		t.Fatalf("seeding the destination: %v", err)
	}

	at := created.Add(time.Hour)
	newPrefix := destination.ChildPath(pack.ID)

	var touched int
	if err := write(background, t, tenantA, func(ctx context.Context) error {
		var err error
		touched, _, err = itemRepo().MoveSubtree(ctx, repository.Move{
			Item:            pack,
			TargetParentID:  destinationID,
			CollectionID:    collection,
			OldPrefix:       pack.Path,
			NewPrefix:       newPrefix,
			DepthDelta:      0,
			OrderKey:        "a0",
			UpdatedAt:       at,
			ExpectedVersion: pack.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("moving the work package: %v", err)
	}

	// The work package and its activity: two rows.
	if touched != 2 {
		t.Errorf("the move touched %d rows, want 2", touched)
	}

	moved := findWorkItem(background, t, tenantA, pack.ID)
	if moved.ParentID != destinationID {
		t.Errorf("the parent is %s, want %s", moved.ParentID, destinationID)
	}
	if moved.Path != newPrefix {
		t.Errorf("the path is %q, want %q", moved.Path, newPrefix)
	}
	if moved.Version != pack.Version+1 {
		t.Errorf("the version is %d, want %d", moved.Version, pack.Version+1)
	}

	// The descendant is the point: its path has to have followed, and its own parent must not have changed.
	child := findWorkItem(background, t, tenantA, activity.ID)
	if want := newPrefix + activity.ID.String() + work.PathSeparator; child.Path != want {
		t.Errorf("the activity's path is %q, want %q", child.Path, want)
	}
	if child.ParentID != pack.ID {
		t.Errorf("the activity's parent moved to %s", child.ParentID)
	}
	if child.Depth != activity.Depth {
		t.Errorf("the activity's depth is %d, want %d", child.Depth, activity.Depth)
	}
}

// A subtree shifts every depth by the same amount. Exercised by moving one subtree's work package under
// another subtree's activity, which is two levels deeper.
//
// That placement is one the hierarchy service would refuse - an activity takes no children under the default
// profiles - and it is the repository under test here, not the domain. The point is the arithmetic over a
// subtree, and the table's own CHECK constrains only which type may sit at the top level.
func TestASubtreeShiftsEveryDepthByTheSameAmount(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	_, pack, activity := movableSubtree(ctx, t, tenantA, authorA, collection)
	_, _, deeper := movableSubtree(ctx, t, tenantA, authorA, collection)

	// The work package sits at depth 2; under an activity at depth 3 it lands at 4.
	newPrefix := deeper.ChildPath(pack.ID)
	delta := (deeper.Depth + 1) - pack.Depth

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, _, err := itemRepo().MoveSubtree(ctx, repository.Move{
			Item: pack, TargetParentID: deeper.ID, CollectionID: collection,
			OldPrefix: pack.Path, NewPrefix: newPrefix, DepthDelta: delta,
			OrderKey: "c0", UpdatedAt: created.Add(time.Hour), ExpectedVersion: pack.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("moving under a deeper parent: %v", err)
	}

	moved := findWorkItem(ctx, t, tenantA, pack.ID)
	if moved.Depth != pack.Depth+delta {
		t.Errorf("the moved item's depth is %d, want %d", moved.Depth, pack.Depth+delta)
	}
	// The descendant shifts by the same amount, which is what makes the whole rewrite one statement.
	child := findWorkItem(ctx, t, tenantA, activity.ID)
	if child.Depth != activity.Depth+delta {
		t.Errorf("the activity's depth is %d, want %d", child.Depth, activity.Depth+delta)
	}
	if want := newPrefix + activity.ID.String() + work.PathSeparator; child.Path != want {
		t.Errorf("the activity's path is %q, want %q", child.Path, want)
	}
}

// A trashed descendant is rewritten too: its path still describes where a restore would put it, and leaving it
// behind would point that restore at an ancestor which has moved.
func TestATrashedDescendantIsRewrittenWithTheSubtree(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	_, pack, activity := movableSubtree(ctx, t, tenantA, authorA, collection)
	stampItemColumn(ctx, t, activity.ID, "deleted_at")

	destinationID := freshID(t)
	destination := taskIn(tenantA, authorA, collection, destinationID, freshName(t), "b0")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, destination)
	}); err != nil {
		t.Fatalf("seeding the destination: %v", err)
	}

	newPrefix := destination.ChildPath(pack.ID)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, _, err := itemRepo().MoveSubtree(ctx, repository.Move{
			Item: pack, TargetParentID: destinationID, CollectionID: collection,
			OldPrefix: pack.Path, NewPrefix: newPrefix, DepthDelta: 0,
			OrderKey: "c0", UpdatedAt: created.Add(time.Hour), ExpectedVersion: pack.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("moving: %v", err)
	}

	child := findWorkItem(ctx, t, tenantA, activity.ID)
	if want := newPrefix + activity.ID.String() + work.PathSeparator; child.Path != want {
		t.Errorf("the trashed activity's path is %q, want %q", child.Path, want)
	}
}

// The lost update a move must not perform: the placement carries the lock, so a stale version stops the move
// before any path is rewritten.
func TestAMoveWithAStaleVersionRewritesNothing(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	_, pack, activity := movableSubtree(ctx, t, tenantA, authorA, collection)

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, _, err := itemRepo().MoveSubtree(ctx, repository.Move{
			Item: pack, TargetParentID: pack.ParentID, CollectionID: collection,
			OldPrefix: pack.Path, NewPrefix: pack.Path, DepthDelta: 0,
			OrderKey: "c0", UpdatedAt: created.Add(time.Hour),
			ExpectedVersion: pack.Version + 7,
		})
		return err
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale version answered %v", err)
	}

	// And nothing moved, which is what "before any path is rewritten" means.
	if after := findWorkItem(ctx, t, tenantA, activity.ID); after.Path != activity.Path {
		t.Errorf("the activity's path became %q despite the conflict", after.Path)
	}
}

// Gate SG-3 for MoveSubtree.
func TestAMoveCannotReachAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	_, pack, activity := movableSubtree(ctx, t, tenantA, authorA, collection)

	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, _, err := itemRepo().MoveSubtree(ctx, repository.Move{
			Item: pack, TargetParentID: pack.ParentID, CollectionID: collection,
			OldPrefix: pack.Path, NewPrefix: pack.Path + "moved/", DepthDelta: 0,
			OrderKey: "c0", UpdatedAt: created.Add(time.Hour), ExpectedVersion: pack.Version,
		})
		return err
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("moving across the boundary answered %v", err)
	}
	if after := findWorkItem(ctx, t, tenantA, activity.ID); after.Path != activity.Path {
		t.Error("tenant B rewrote tenant A's subtree")
	}
}

// The acceptance criterion for the ranks: an insertion between two neighbours renumbers nothing else.
func TestReorderingBetweenTwoNeighboursTouchesOneRow(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	first, second, third := freshID(t), freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		items := itemRepo()
		for id, key := range map[shared.ID]string{first: "a0", second: "a1", third: "a2"} {
			if err := items.Insert(ctx, taskIn(tenantA, authorA, collection, id, freshName(t), key)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the level: %v", err)
	}

	level := repository.Level{CollectionID: collection}

	// Where the third item goes if it is placed before the second: between the first and the second.
	var previous, next string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		previous, next, err = itemRepo().Neighbours(ctx, level, second, third)
		return err
	}); err != nil {
		t.Fatalf("reading the neighbours: %v", err)
	}
	if previous != "a0" || next != "a1" {
		t.Fatalf("the bounds are %q and %q, want a0 and a1", previous, next)
	}

	moving := findWorkItem(ctx, t, tenantA, third)
	ranked := moving
	ranked.OrderKey = "a0V"
	ranked.UpdatedAt = created.Add(time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetOrderKey(ctx, ranked, moving.Version)
	}); err != nil {
		t.Fatalf("reordering: %v", err)
	}

	// The two neighbours are untouched, which is the whole point of a fractional index.
	for id, key := range map[shared.ID]string{first: "a0", second: "a1"} {
		if after := findWorkItem(ctx, t, tenantA, id); after.OrderKey != key {
			t.Errorf("%s was renumbered to %q", id, after.OrderKey)
		}
	}
	if after := findWorkItem(ctx, t, tenantA, third); after.OrderKey != "a0V" {
		t.Errorf("the moved item's rank is %q", after.OrderKey)
	}
}

// The item being moved is excluded from its own level: measuring a new position against the rank it is
// leaving would place it next to where it already is.
func TestTheMovingItemIsNotItsOwnNeighbour(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	first, second := freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		items := itemRepo()
		if err := items.Insert(ctx, taskIn(tenantA, authorA, collection, first, freshName(t), "a0")); err != nil {
			return err
		}
		return items.Insert(ctx, taskIn(tenantA, authorA, collection, second, freshName(t), "a1"))
	}); err != nil {
		t.Fatalf("seeding: %v", err)
	}

	level := repository.Level{CollectionID: collection}

	// Appending the second item: the bound below it must be the first, not itself.
	var previous, next string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		previous, next, err = itemRepo().Neighbours(ctx, level, "", second)
		return err
	}); err != nil {
		t.Fatalf("reading the neighbours: %v", err)
	}
	if previous != "a0" || next != "" {
		t.Errorf("appending measured against %q and %q, want a0 and nothing", previous, next)
	}
}

// An empty level has no bounds at all, which the ordering service reads as "the first key".
func TestAnEmptyLevelHasNoBounds(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)

	var previous, next string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		previous, next, err = itemRepo().
			Neighbours(ctx, repository.Level{CollectionID: collection}, "", "")
		return err
	}); err != nil {
		t.Fatalf("reading the neighbours: %v", err)
	}
	if previous != "" || next != "" {
		t.Errorf("an empty level answered %q and %q", previous, next)
	}
}

// Gate SG-3 for Neighbours and SetOrderKey.
func TestTheRankMethodsCannotReachAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := seedRootTask(ctx, t, tenantA, authorA, collection)
	item := findWorkItem(ctx, t, tenantA, id)

	t.Run("neighbours", func(t *testing.T) {
		var previous, next string
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			previous, next, err = itemRepo().
				Neighbours(ctx, repository.Level{CollectionID: collection}, "", "")
			return err
		}); err != nil {
			t.Fatalf("reading as tenant B: %v", err)
		}
		if previous != "" || next != "" {
			t.Errorf("tenant B measured against %q and %q in tenant A's collection", previous, next)
		}
	})

	t.Run("set order key", func(t *testing.T) {
		ranked := item
		ranked.OrderKey = "z9"
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return itemRepo().SetOrderKey(ctx, ranked, item.Version)
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Fatalf("writing across the boundary answered %v", err)
		}
		if after := findWorkItem(ctx, t, tenantA, id); after.OrderKey == "z9" {
			t.Error("tenant B reordered tenant A's item")
		}
	})
}
