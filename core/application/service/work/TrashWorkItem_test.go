// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

func (h *lifecycleHarness) trash() TrashWorkItem     { return TrashWorkItem{Lifecycle: h.writer} }
func (h *lifecycleHarness) restore() RestoreWorkItem { return RestoreWorkItem{Lifecycle: h.writer} }

// withSubtree hangs a work package and an activity under the task, so that a deletion has something
// to take with it.
func (h *lifecycleHarness) withSubtree() (domain.WorkItem, domain.WorkItem) {
	task := h.items.stored[taskID]
	pack := domain.WorkItem{
		ID: packageID, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemWorkPackage,
		ParentID: taskID, Path: task.ChildPath(packageID), Depth: 2, Title: "Dairy aisle",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	activity := domain.WorkItem{
		ID: activityID, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemActivity,
		ParentID: packageID, Path: pack.ChildPath(activityID), Depth: 3, Title: "Milk",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[packageID] = pack
	h.items.stored[activityID] = activity
	return pack, activity
}

// One deletion takes the subtree with it under one batch, and says how much it took (I-C2).
func TestTrashingTakesTheSubtreeUnderOneBatch(t *testing.T) {
	h := newLifecycleHarness()
	h.withSubtree()

	item, err := h.trash().Execute(t.Context(), itemActor(), LifecycleCommand{ItemID: taskID})
	if err != nil {
		t.Fatalf("trashing was refused: %v", err)
	}

	if !item.IsTrashed() {
		t.Error("the entry came back out of the trash")
	}
	batch := item.TrashBatchID
	if batch.IsZero() {
		t.Fatal("the deletion carries no batch")
	}
	for _, id := range []shared.ID{taskID, packageID, activityID} {
		stored := h.items.stored[id]
		if !stored.IsTrashed() || stored.TrashBatchID != batch {
			t.Errorf("%s is in batch %q, want the deletion's %q", stored.Title, stored.TrashBatchID, batch)
		}
	}

	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ItemTrashed {
		t.Fatalf("the events are %v, want one %s", h.events.appended, event.ItemTrashed)
	}
	if size := h.events.appended[0].Payload["subtree_size"]; size != 3 {
		t.Errorf("subtree_size = %v, want 3", size)
	}
}

// An offline client is told to remove its copy rather than to apply a state change - and the entry
// carries no payload, because a tombstone with content would be a copy of the deleted entry living
// on in the log (offline-sync.md §3.1).
func TestTrashingRecordsADeletionWithNoPayload(t *testing.T) {
	h := newLifecycleHarness()

	if _, err := h.trash().Execute(
		t.Context(), itemActor(), LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("trashing was refused: %v", err)
	}

	if len(h.changes.recorded) != 1 {
		t.Fatalf("%d changes recorded, want 1", len(h.changes.recorded))
	}
	change := h.changes.recorded[0]
	if change.Op != changelog.Delete {
		t.Errorf("the change is a %s, want a %s", change.Op, changelog.Delete)
	}
	if change.Payload != nil {
		t.Errorf("the tombstone carries a payload: %v", change.Payload)
	}
	if change.ContainerID != collectionID {
		t.Errorf("the change is filed under %q, want the collection %q", change.ContainerID, collectionID)
	}
}

// The acceptance criterion of this task: a restore takes exactly the batch. A separate deletion
// inside the same subtree is somebody else's decision and stays where it is.
func TestRestoringTakesExactlyTheBatch(t *testing.T) {
	h := newLifecycleHarness()
	_, activity := h.withSubtree()
	ctx, actor := t.Context(), itemActor()

	// The activity is deleted on its own first, then the whole task on top of it.
	if _, err := h.trash().Execute(ctx, actor, LifecycleCommand{ItemID: activity.ID}); err != nil {
		t.Fatalf("the first deletion was refused: %v", err)
	}
	own := h.items.stored[activityID].TrashBatchID

	if _, err := h.trash().Execute(ctx, actor, LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("the second deletion was refused: %v", err)
	}
	if adopted := h.items.stored[activityID].TrashBatchID; adopted != own {
		t.Fatalf("the activity was adopted into batch %q, want its own %q", adopted, own)
	}

	if _, err := h.restore().Execute(ctx, actor, LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("the restore was refused: %v", err)
	}

	for _, id := range []shared.ID{taskID, packageID} {
		if h.items.stored[id].IsTrashed() {
			t.Errorf("%s is still in the trash", h.items.stored[id].Title)
		}
	}
	if !h.items.stored[activityID].IsTrashed() {
		t.Error("the activity's own deletion was undone by somebody else's restore")
	}
}

// An entry that was archived when it was deleted comes back archived. Restoring undoes the deletion
// and nothing else - anything narrower would make Restore a silent unarchive as well.
func TestRestoringDoesNotLiftTheArchive(t *testing.T) {
	h := newLifecycleHarness()
	ctx, actor := t.Context(), itemActor()

	if _, err := h.archive().Execute(ctx, actor, LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("archiving was refused: %v", err)
	}
	if _, err := h.trash().Execute(ctx, actor, LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("trashing was refused: %v", err)
	}

	item, err := h.restore().Execute(ctx, actor, LifecycleCommand{ItemID: taskID})
	if err != nil {
		t.Fatalf("the restore was refused: %v", err)
	}
	if item.IsTrashed() {
		t.Error("the entry is still in the trash")
	}
	if !item.IsArchived() {
		t.Error("the restore lifted the archive stamp as well")
	}
}

// A live entry hanging off a deleted one would be visible in no list - every list it is read
// through starts at its parent - and would be quietly resurrected the day somebody restored that
// parent. The answer names the parent, so a client can offer to restore that instead.
func TestRestoringUnderATrashedParentIsRefused(t *testing.T) {
	h := newLifecycleHarness()
	pack, _ := h.withSubtree()
	ctx, actor := t.Context(), itemActor()

	if _, err := h.trash().Execute(ctx, actor, LifecycleCommand{ItemID: pack.ID}); err != nil {
		t.Fatalf("deleting the work package was refused: %v", err)
	}
	if _, err := h.trash().Execute(ctx, actor, LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("deleting the task was refused: %v", err)
	}

	_, err := h.restore().Execute(ctx, actor, LifecycleCommand{ItemID: pack.ID})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("restoring under a deleted parent reported %v, want a conflict", err)
	}
	if detail := shared.AsError(err).DetailCode; detail != "items.parent_trashed" {
		t.Errorf("the detail code is %q, want items.parent_trashed", detail)
	}
	if params := shared.AsError(err).Params; params["parent_id"] != taskID.String() {
		t.Errorf("the answer names %q, want the parent %q", params["parent_id"], taskID)
	}
}

// Deleting twice is the state the caller asked for, and costs nothing: no version, no event, and -
// the part that matters - no new deletion date. The retention period runs off that stamp, and a
// silently restarted one is thirty more days of storage nobody asked for.
func TestTrashingTwiceDoesNotRestartTheRetentionPeriod(t *testing.T) {
	h := newLifecycleHarness()
	ctx, actor := t.Context(), itemActor()

	first, err := h.trash().Execute(ctx, actor, LifecycleCommand{ItemID: taskID})
	if err != nil {
		t.Fatalf("the first deletion was refused: %v", err)
	}

	second, err := h.trash().Execute(ctx, actor, LifecycleCommand{ItemID: taskID})
	if err != nil {
		t.Fatalf("the second deletion was refused: %v", err)
	}
	if !second.DeletedAt.Equal(*first.DeletedAt) {
		t.Error("the second deletion re-dated the first")
	}
	if second.TrashBatchID != first.TrashBatchID {
		t.Error("the second deletion issued a new batch")
	}
	if len(h.events.appended) != 1 {
		t.Errorf("%d events, want 1", len(h.events.appended))
	}
}

// I-C3 reaches the deletion too: a collection in an archived hub is read-only, and deleting out of
// it is a write into a subtree nobody may write to.
func TestTrashingIsRefusedInsideAnArchivedSubtree(t *testing.T) {
	h := newLifecycleHarness()
	archivedAt := now.Add(-time.Hour)
	collection := h.containers.stored[collectionID]
	collection.ParentArchivedAt = &archivedAt
	h.containers.stored[collectionID] = collection

	_, err := h.trash().Execute(t.Context(), itemActor(), LifecycleCommand{ItemID: taskID})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("deleting inside an archived hub reported %v, want a conflict", err)
	}
}

// The audit trail is where a deletion is answered for. It records both stamps and no user content:
// no title, no notes (rule 10).
func TestTheDeletionAuditEntryRecordsTheStampsAndNoUserContent(t *testing.T) {
	h := newLifecycleHarness()

	if _, err := h.trash().Execute(
		t.Context(), itemActor(), LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("trashing was refused: %v", err)
	}

	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ItemTrashedAction {
		t.Fatalf("the audit entries are %v, want one %s", h.audit.entries, ItemTrashedAction)
	}
	recorded := h.audit.entries[0].Changes
	if to, _ := recorded["deleted_at"].(map[string]any)["to"].(string); to == "" {
		t.Error("the audit entry records no deletion stamp")
	}
	for field, entry := range recorded {
		if to, _ := entry.(map[string]any)["to"].(string); to == "Weekly shop" {
			t.Errorf("the entry's title reached the audit trail as %s", field)
		}
	}
}

// The MCP annotation an agent client reads before asking a person to confirm (ai-first.md §1.1).
// Reversible, and still destructive: what it takes with it is a subtree somebody may not have
// realised was there.
func TestTheDeletionIsDeclaredDestructiveAndTheRestoreIsNot(t *testing.T) {
	h := newLifecycleHarness()

	if !h.trash().Descriptor().Destructive {
		t.Error("TrashWorkItem is not declared destructive")
	}
	if h.restore().Descriptor().Destructive {
		t.Error("RestoreWorkItem is declared destructive")
	}
}

// A deletion schedules its own cleanup, in the transaction that made it. It is not only convenient:
// nothing in this system may enumerate tenants, so no scheduler can create one sweep per tenant -
// and a deletion whose cleanup was never scheduled would sit in the trash until somebody deleted
// something else, with nothing to say why.
func TestADeletionSchedulesTheTenantsRetentionSweep(t *testing.T) {
	h := newLifecycleHarness()
	ctx, actor := t.Context(), itemActor()

	if _, err := h.trash().Execute(ctx, actor, LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("trashing was refused: %v", err)
	}

	if len(h.jobs.enqueued) != 1 {
		t.Fatalf("%d jobs enqueued, want 1", len(h.jobs.enqueued))
	}
	request := h.jobs.enqueued[0]
	if request.Kind != queue.KindRetentionSweep {
		t.Errorf("the job is a %s, want a %s", request.Kind, queue.KindRetentionSweep)
	}
	if request.TenantID != tenantID {
		t.Errorf("the job is for %q, want the tenant %q", request.TenantID, tenantID)
	}
	// The dedupe key is the tenant, so a request that deletes twenty entries leaves one job rather
	// than twenty (queue.Request.DedupeKey).
	if request.DedupeKey != tenantID.String() {
		t.Errorf("the dedupe key is %q, want the tenant", request.DedupeKey)
	}
}

// The other verbs ask for nothing. Archiving puts nothing on a clock, and a restore takes something
// off one - a sweep asked for by either would be a job scheduled for work that does not exist.
func TestOnlyTheDeletionSchedulesASweep(t *testing.T) {
	h := newLifecycleHarness()
	ctx, actor := t.Context(), itemActor()

	if _, err := h.archive().Execute(ctx, actor, LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("archiving was refused: %v", err)
	}
	if _, err := h.unarchive().Execute(ctx, actor, LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("unarchiving was refused: %v", err)
	}
	if len(h.jobs.enqueued) != 0 {
		t.Errorf("the archive verbs scheduled %v", h.jobs.enqueued)
	}

	if _, err := h.trash().Execute(ctx, actor, LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("trashing was refused: %v", err)
	}
	before := len(h.jobs.enqueued)

	if _, err := h.restore().Execute(ctx, actor, LifecycleCommand{ItemID: taskID}); err != nil {
		t.Fatalf("the restore was refused: %v", err)
	}
	if len(h.jobs.enqueued) != before {
		t.Errorf("the restore scheduled a sweep: %v", h.jobs.enqueued)
	}
}
