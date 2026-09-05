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
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The trash against the real database (B-10): the cascade, the batch that makes a restore exact,
// the view over both aggregates, the hard delete - and a cross-tenant negative for every one of
// them (gate SG-3).
//
// This is the first path in the system that removes data, so the negatives matter more here than
// anywhere else: a scope that leaked would not show up as a wrong answer but as somebody else's
// work being gone.

var (
	deletedAt  = created.Add(2 * time.Hour)
	restoredAt = created.Add(3 * time.Hour)

	// Who did the deleting, for the stamp migration 0070 added. A person rather than an automation,
	// because that is the case the trash screen exists to show.
	deletedByUser = work.DeletedBy{
		Kind: shared.ActorUser,
		ID:   shared.MustParseID("0192f000-0000-7000-8000-0000000000e1"),
	}
)

func trashRepo() postgres.TrashRepository { return postgres.NewTrashRepository(pageCursors()) }

// subtree writes a task with a work package and an activity under it, and hands back the three
// identifiers in that order. The shape every subtree test needs, written once.
func trashableSubtree(
	ctx context.Context, t *testing.T, tenant, author, collection shared.ID,
) (work.WorkItem, work.WorkItem, work.WorkItem) {
	t.Helper()
	repo := itemRepo()

	taskID, packageID, activityID := freshID(t), freshID(t), freshID(t)
	task := taskIn(tenant, author, collection, taskID, "Weekly shop", "a0")
	pkg := work.WorkItem{
		ID: packageID, TenantID: tenant, CollectionID: collection, Type: work.ItemWorkPackage,
		ParentID: taskID, Path: task.ChildPath(packageID), Depth: 2, Title: "Dairy aisle",
		OrderKey: "a0", CreatedBy: author, CreatedAt: created, UpdatedAt: created, Version: 1,
	}
	activity := work.WorkItem{
		ID: activityID, TenantID: tenant, CollectionID: collection, Type: work.ItemActivity,
		ParentID: packageID, Path: pkg.ChildPath(activityID), Depth: 3, Title: "Milk",
		OrderKey: "a0", CreatedBy: author, CreatedAt: created, UpdatedAt: created, Version: 1,
	}

	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		for _, item := range []work.WorkItem{task, pkg, activity} {
			if err := repo.Insert(ctx, item); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the subtree: %v", err)
	}
	return task, pkg, activity
}

// trashItem puts one item and its subtree into the trash, as the use case will: the domain stamps
// the row, the repository writes it.
func trashItem(
	ctx context.Context, t *testing.T, tenant shared.ID, item work.WorkItem, batch shared.ID,
	at time.Time,
) int {
	t.Helper()

	stamped, _, err := item.Trashed(at, batch, deletedByUser)
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}

	var moved int
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		moved, err = itemRepo().TrashSubtree(ctx, repository.ItemTrash{
			Item: stamped, Prefix: item.Path, BatchID: batch, ExpectedVersion: item.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("trashing the item: %v", err)
	}
	return moved
}

// I-C2, the first half: one deletion, one batch, over the whole subtree.
func TestTrashingAnItemTakesItsSubtreeUnderOneBatch(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task, pkg, activity := trashableSubtree(ctx, t, tenantA, authorA, collection)
	batch := freshID(t)

	if moved := trashItem(ctx, t, tenantA, task, batch, deletedAt); moved != 3 {
		t.Errorf("the deletion took %d rows, want 3", moved)
	}

	for _, item := range []work.WorkItem{task, pkg, activity} {
		stored := findItem(ctx, t, tenantA, item.ID)
		if !stored.IsTrashed() {
			t.Errorf("%s is not in the trash", stored.Title)
		}
		if stored.TrashBatchID != batch {
			t.Errorf("%s carries batch %q, want %q", stored.Title, stored.TrashBatchID, batch)
		}
		if !stored.DeletedAt.Equal(deletedAt) {
			t.Errorf("%s was deleted at %v, want %v", stored.Title, stored.DeletedAt, deletedAt)
		}
		if stored.Version != item.Version+1 {
			t.Errorf("%s is at version %d, want %d", stored.Title, stored.Version, item.Version+1)
		}
	}
}

// I-C2, the second half, and the acceptance criterion this task is judged on: a restore takes
// exactly the batch, and leaves a separate deletion inside the same subtree where it is.
func TestRestoringTakesExactlyTheBatchAndNoOtherDeletion(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task, pkg, activity := trashableSubtree(ctx, t, tenantA, authorA, collection)

	// The activity is deleted on its own first, and then the whole task on top of it.
	own, whole := freshID(t), freshID(t)
	trashItem(ctx, t, tenantA, activity, own, deletedAt)
	if moved := trashItem(ctx, t, tenantA, task, whole, deletedAt.Add(time.Hour)); moved != 2 {
		t.Errorf("the second deletion took %d rows, want 2 - the activity was already in the trash", moved)
	}

	if stored := findItem(ctx, t, tenantA, activity.ID); stored.TrashBatchID != own {
		t.Fatalf("the activity was adopted into batch %q, want its own %q", stored.TrashBatchID, own)
	}

	trashed := findItem(ctx, t, tenantA, task.ID)
	restored, _, err := trashed.Restored(restoredAt)
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}

	var back int
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		back, err = itemRepo().RestoreBatch(ctx, repository.ItemTrash{
			Item: restored, BatchID: whole, ExpectedVersion: trashed.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("restoring the batch: %v", err)
	}
	if back != 2 {
		t.Errorf("the restore brought back %d rows, want 2", back)
	}

	for _, item := range []work.WorkItem{task, pkg} {
		if stored := findItem(ctx, t, tenantA, item.ID); stored.IsTrashed() {
			t.Errorf("%s is still in the trash", stored.Title)
		}
	}
	if stored := findItem(ctx, t, tenantA, activity.ID); !stored.IsTrashed() {
		t.Error("the activity's own deletion was undone by somebody else's restore")
	}
}

// A container's deletion reaches two tables and three levels: the hub, its collections, and every
// entry in them.
func TestTrashingAHubTakesItsCollectionsAndTheirEntries(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hubID, collectionID := hubWithCollection(ctx, t, tenantA, authorA)
	trashableSubtree(ctx, t, tenantA, authorA, collectionID)
	batch := freshID(t)

	hub := findContainer(ctx, t, tenantA, hubID)
	stamped, _, err := hub.Trashed(deletedAt, batch, deletedByUser)
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}

	var cascade repository.Cascade
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		cascade, err = containerRepo().TrashSubtree(ctx, repository.ContainerTrash{
			Container: stamped, BatchID: batch, ExpectedVersion: hub.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("trashing the hub: %v", err)
	}

	if len(cascade.Collections) != 1 || cascade.Collections[0] != collectionID {
		t.Errorf("the cascade names %v, want the one collection %q", cascade.Collections, collectionID)
	}
	if cascade.Items != 3 {
		t.Errorf("the cascade took %d entries, want 3", cascade.Items)
	}
	if stored := findContainer(ctx, t, tenantA, collectionID); stored.TrashBatchID != batch {
		t.Errorf("the collection carries batch %q, want the hub's %q", stored.TrashBatchID, batch)
	}
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM work_item WHERE collection_id = $1 AND trash_batch_id = $2`,
		collectionID.String(), batch.String()); rows != 3 {
		t.Errorf("%d entries carry the hub's batch, want 3", rows)
	}
}

// And back out again, whole.
func TestRestoringAHubBringsBackItsCollectionsAndTheirEntries(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hubID, collectionID := hubWithCollection(ctx, t, tenantA, authorA)
	trashableSubtree(ctx, t, tenantA, authorA, collectionID)
	batch := freshID(t)

	hub := findContainer(ctx, t, tenantA, hubID)
	stamped, _, err := hub.Trashed(deletedAt, batch, deletedByUser)
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := containerRepo().TrashSubtree(ctx, repository.ContainerTrash{
			Container: stamped, BatchID: batch, ExpectedVersion: hub.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("trashing the hub: %v", err)
	}

	trashed := findContainer(ctx, t, tenantA, hubID)
	restored, _, err := trashed.Restored(restoredAt)
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}

	var cascade repository.Cascade
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		cascade, err = containerRepo().RestoreBatch(ctx, repository.ContainerTrash{
			Container: restored, BatchID: batch, ExpectedVersion: trashed.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("restoring the hub: %v", err)
	}

	if len(cascade.Collections) != 1 {
		t.Errorf("the restore brought back %d collections, want 1", len(cascade.Collections))
	}
	if cascade.Items != 3 {
		t.Errorf("the restore brought back %d entries, want 3", cascade.Items)
	}
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM work_item WHERE collection_id = $1 AND deleted_at IS NOT NULL`,
		collectionID.String()); rows != 0 {
		t.Errorf("%d entries are still in the trash after the restore", rows)
	}
}

// The trash view: one entry per deletion, not one per deleted row. A hub with a collection and
// three entries under it is one line, because it was one act.
func TestTheTrashListsTheRootOfEachDeletionOnce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hubID, collectionID := hubWithCollection(ctx, t, tenantA, authorA)
	trashableSubtree(ctx, t, tenantA, authorA, collectionID)

	// A second, unrelated deletion in another hub, so that the page has something to be a page of.
	otherCollection := collectionFor(ctx, t, tenantA, authorA)
	loose, _, _ := trashableSubtree(ctx, t, tenantA, authorA, otherCollection)
	trashItem(ctx, t, tenantA, loose, freshID(t), deletedAt.Add(time.Hour))

	hubBatch := freshID(t)
	hub := findContainer(ctx, t, tenantA, hubID)
	stamped, _, err := hub.Trashed(deletedAt, hubBatch, deletedByUser)
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := containerRepo().TrashSubtree(ctx, repository.ContainerTrash{
			Container: stamped, BatchID: hubBatch, ExpectedVersion: hub.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("trashing the hub: %v", err)
	}

	page := listTrash(ctx, t, tenantA, repository.Page{Size: 50})

	roots := map[shared.ID]work.TrashEntry{}
	for _, entry := range page.Entries {
		roots[entry.ID] = entry
	}
	if _, listed := roots[hubID]; !listed {
		t.Error("the deleted hub is not in the trash")
	}
	if _, listed := roots[collectionID]; listed {
		t.Error("the collection is listed separately, although it went with its hub")
	}
	if entry, listed := roots[loose.ID]; !listed {
		t.Error("the separately deleted task is not in the trash")
	} else if entry.Kind != work.TrashItemKind || entry.Subtype != string(work.ItemTask) {
		t.Errorf("the task is listed as %s/%s", entry.Kind, entry.Subtype)
	}
	for _, entry := range page.Entries {
		if entry.ID != hubID && entry.CollectionID == collectionID {
			t.Errorf("%q came along with the hub and is listed in its own right", entry.Title)
		}
	}

	// Newest first, which is the order somebody looking for what they just deleted needs.
	if len(page.Entries) < 2 {
		t.Fatalf("the trash holds %d entries, want at least 2", len(page.Entries))
	}
	if page.Entries[0].DeletedAt.Before(page.Entries[1].DeletedAt) {
		t.Error("the trash is not ordered newest deletion first")
	}
}

// The keyset walks the whole trash exactly once, with no row seen twice and none skipped - the
// off-by-one that shows up as a single missing entry rather than as a failure.
func TestTheTrashPagesWithoutLosingAnEntry(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantB, authorB)

	deleted := map[shared.ID]bool{}
	for i := range 5 {
		task, _, _ := trashableSubtree(ctx, t, tenantB, authorB, collection)
		trashItem(ctx, t, tenantB, task, freshID(t), deletedAt.Add(time.Duration(i)*time.Minute))
		deleted[task.ID] = true
	}

	seen := map[shared.ID]int{}
	cursor := ""
	for range 10 {
		page := listTrash(ctx, t, tenantB, repository.Page{Size: 2, Cursor: cursor})
		for _, entry := range page.Entries {
			seen[entry.ID]++
		}
		if !page.Info.HasMore {
			break
		}
		cursor = page.Info.NextCursor
	}

	for id := range deleted {
		switch seen[id] {
		case 1:
		case 0:
			t.Errorf("%s was skipped by the walk", id)
		default:
			t.Errorf("%s was returned %d times", id, seen[id])
		}
	}
}

func listTrash(
	ctx context.Context, t *testing.T, tenant shared.ID, page repository.Page,
) repository.TrashPage {
	t.Helper()

	var result repository.TrashPage
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		result, err = trashRepo().List(ctx, page)
		return err
	}); err != nil {
		t.Fatalf("reading the trash: %v", err)
	}
	return result
}

// The hard delete, and the order the schema insists on: a hub whose collections are still there
// refuses to go, which is ON DELETE RESTRICT doing the remembering rather than this code.
func TestPurgingRemovesTheSubtreeAndTakesTheContainersInOrder(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hubID, collectionID := hubWithCollection(ctx, t, tenantA, authorA)
	task, _, _ := trashableSubtree(ctx, t, tenantA, authorA, collectionID)

	var ids []shared.ID
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		ids, err = trashRepo().SubtreeIDs(ctx, task.Path)
		return err
	}); err != nil {
		t.Fatalf("reading the subtree: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("the subtree holds %d identifiers, want 3", len(ids))
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		gone, err := trashRepo().PurgeItems(ctx, ids)
		if err != nil {
			return err
		}
		if gone != 3 {
			t.Errorf("%d rows were purged, want 3", gone)
		}
		return nil
	}); err != nil {
		t.Fatalf("purging the subtree: %v", err)
	}
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM work_item WHERE id = $1`, task.ID.String()); rows != 0 {
		t.Error("the purged task is still there")
	}

	// The hub before its collection is refused, and the other way round works.
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := trashRepo().PurgeContainers(ctx, []shared.ID{hubID})
		return err
	})
	if err == nil {
		t.Error("a hub was purged while its collection was still there")
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := trashRepo().PurgeContainers(ctx, []shared.ID{collectionID}); err != nil {
			return err
		}
		_, err := trashRepo().PurgeContainers(ctx, []shared.ID{hubID})
		return err
	}); err != nil {
		t.Fatalf("purging the collection and then the hub: %v", err)
	}
	if rows := countIn(ctx, t,
		`SELECT count(*) FROM container WHERE id IN ($1, $2)`,
		hubID.String(), collectionID.String()); rows != 0 {
		t.Error("the purged containers are still there")
	}
}

// An empty list is not a statement. The commonest retention run is the one with nothing to do, and
// it should cost no round trip at all.
func TestPurgingNothingIsNotAStatement(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		// A read-only transaction: a DELETE reaching the database here would fail outright, which
		// is what makes this an assertion rather than a hope.
		if _, err := trashRepo().PurgeItems(ctx, nil); err != nil {
			return err
		}
		_, err := trashRepo().PurgeContainers(ctx, nil)
		return err
	}); err != nil {
		t.Errorf("purging an empty list reached the database: %v", err)
	}
}

// The cross-tenant negatives, one per method that writes or reads across the boundary (gate SG-3).
//
// Every one of them is the same shape and the same answer: the other tenant's rows are not there,
// so the update matches nothing and the caller is told what it is told when a row has moved on. The
// answers are deliberately indistinguishable - a caller that could tell "it belongs to somebody
// else" from "it changed" would have a way to confirm that an identifier exists elsewhere
// (multi-tenancy.md §2).
func TestNoTenantsTrashReachesAnother(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hubID, collectionID := hubWithCollection(ctx, t, tenantA, authorA)
	task, _, _ := trashableSubtree(ctx, t, tenantA, authorA, collectionID)
	batch := freshID(t)

	stampedItem, _, err := task.Trashed(deletedAt, batch, deletedByUser)
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}
	hub := findContainer(ctx, t, tenantA, hubID)
	stampedHub, _, err := hub.Trashed(deletedAt, batch, deletedByUser)
	if err != nil {
		t.Fatalf("the transition was refused: %v", err)
	}

	t.Run("trashing an item", func(t *testing.T) {
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := itemRepo().TrashSubtree(ctx, repository.ItemTrash{
				Item: stampedItem, Prefix: task.Path, BatchID: batch, ExpectedVersion: task.Version,
			})
			return err
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("trashing across the boundary reported %v", err)
		}
	})

	t.Run("archiving an item", func(t *testing.T) {
		archived, _, err := task.Archived(deletedAt)
		if err != nil {
			t.Fatalf("the transition was refused: %v", err)
		}
		err = write(ctx, t, tenantB, func(ctx context.Context) error {
			return itemRepo().SetArchived(ctx, archived, task.Version)
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("archiving across the boundary reported %v", err)
		}
	})

	t.Run("trashing a container", func(t *testing.T) {
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := containerRepo().TrashSubtree(ctx, repository.ContainerTrash{
				Container: stampedHub, BatchID: batch, ExpectedVersion: hub.Version,
			})
			return err
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("trashing a container across the boundary reported %v", err)
		}
	})

	// The rest of the deletion is now real, so that the reads below have something to fail to find.
	trashItem(ctx, t, tenantA, task, batch, deletedAt)

	t.Run("listing the trash", func(t *testing.T) {
		page := listTrash(ctx, t, tenantB, repository.Page{Size: 50})
		for _, entry := range page.Entries {
			if entry.ID == task.ID {
				t.Error("another tenant's deletion is in this tenant's trash")
			}
		}
	})

	t.Run("reading a subtree", func(t *testing.T) {
		var ids []shared.ID
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			ids, err = trashRepo().SubtreeIDs(ctx, task.Path)
			return err
		}); err != nil {
			t.Fatalf("reading a foreign subtree: %v", err)
		}
		if len(ids) != 0 {
			t.Errorf("another tenant's subtree came back with %d rows", len(ids))
		}
	})

	t.Run("restoring a batch", func(t *testing.T) {
		trashed := findItem(ctx, t, tenantA, task.ID)
		restored, _, err := trashed.Restored(restoredAt)
		if err != nil {
			t.Fatalf("the transition was refused: %v", err)
		}
		err = write(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := itemRepo().RestoreBatch(ctx, repository.ItemTrash{
				Item: restored, BatchID: batch, ExpectedVersion: trashed.Version,
			})
			return err
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("restoring across the boundary reported %v", err)
		}
		if stored := findItem(ctx, t, tenantA, task.ID); !stored.IsTrashed() {
			t.Error("another tenant's restore took this tenant's deletion out of the trash")
		}
	})

	t.Run("purging", func(t *testing.T) {
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			if _, err := trashRepo().PurgeItems(ctx, []shared.ID{task.ID}); err != nil {
				return err
			}
			_, err := trashRepo().PurgeContainers(ctx, []shared.ID{collectionID, hubID})
			return err
		}); err != nil {
			t.Fatalf("purging across the boundary: %v", err)
		}
		if rows := countIn(ctx, t,
			`SELECT count(*) FROM work_item WHERE id = $1`, task.ID.String()); rows != 1 {
			t.Error("another tenant purged this tenant's entry")
		}
		if rows := countIn(ctx, t,
			`SELECT count(*) FROM container WHERE id IN ($1, $2)`,
			hubID.String(), collectionID.String()); rows != 2 {
			t.Error("another tenant purged this tenant's containers")
		}
	})
}
