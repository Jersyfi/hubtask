// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// I-C2: one deletion, one batch, over the hub, its collections and their entries.
func TestDeletingAHubTakesItsSubtreeUnderOneBatch(t *testing.T) {
	h := newContainerHarness()
	hub := h.withHub(hubID, "Private")
	h.withCollection()
	// What the item repository would report as having gone with the containers.
	h.containers.cascadedItems = 17

	deleted, err := TrashContainer{Writer: h.writer}.Execute(t.Context(), actor(),
		ContainerLifecycleCommand{ContainerID: hubID})
	if err != nil {
		t.Fatalf("the deletion was refused: %v", err)
	}

	if !deleted.IsTrashed() {
		t.Error("the hub came back out of the trash")
	}
	batch := deleted.TrashBatchID
	if batch.IsZero() {
		t.Fatal("the deletion carries no batch")
	}
	if stored := h.containers.stored[shoppingID]; stored.TrashBatchID != batch {
		t.Errorf("the collection carries batch %q, want the hub's %q", stored.TrashBatchID, batch)
	}
	if deleted.Version != hub.Version+1 {
		t.Errorf("version %d, want %d", deleted.Version, hub.Version+1)
	}

	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ContainerDeleted {
		t.Fatalf("the events are %v, want one %s", h.events.appended, event.ContainerDeleted)
	}
	payload := h.events.appended[0].Payload
	if payload["collections"] != 1 || payload["items"] != 17 {
		t.Errorf("the cascade is %v/%v, want 1/17", payload["collections"], payload["items"])
	}
	if payload["trash_batch_id"] != batch.String() {
		t.Errorf("the event names batch %v, want %q", payload["trash_batch_id"], batch)
	}
}

// Only the owner may delete a container - the one thing an administrator cannot do, because it takes
// a subtree with it (domain-model.md §3.2). The restore asks the same right: an owner's deletion must
// not be undoable by somebody the matrix deliberately excluded from making it.
func TestBothDirectionsAskForTheRightToDeleteAContainer(t *testing.T) {
	for _, c := range []struct {
		name    string
		execute func(*containerHarness) error
		action  string
	}{
		{"deleting", func(h *containerHarness) error {
			_, err := TrashContainer{Writer: h.writer}.Execute(t.Context(), actor(),
				ContainerLifecycleCommand{ContainerID: shoppingID})
			return err
		}, string(ContainerDeletedAction)},
		{"restoring", func(h *containerHarness) error {
			_, err := RestoreContainer{Writer: h.writer}.Execute(t.Context(), actor(),
				ContainerLifecycleCommand{ContainerID: shoppingID})
			return err
		}, string(ContainerRestoredAction)},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newContainerHarness()
			h.withCollection()

			if err := c.execute(h); err != nil {
				t.Fatalf("%s was refused: %v", c.name, err)
			}
			if len(h.authorizer.requests) != 1 {
				t.Fatalf("%d permission questions, want 1", len(h.authorizer.requests))
			}
			request := h.authorizer.requests[0]
			if request.Permission != service.PermissionDeleteContainer {
				t.Errorf("permission %s, want %s", request.Permission, service.PermissionDeleteContainer)
			}
			if string(request.Action) != c.action {
				t.Errorf("action %s, want %s", request.Action, c.action)
			}
		})
	}
}

// A refusal writes nothing at all, and the audit entry it produces lives outside this transaction so
// that it survives the rollback (audit.md §7).
func TestARefusedDeletionWritesNothing(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	_, err := TrashContainer{Writer: h.writer}.Execute(t.Context(), actor(),
		ContainerLifecycleCommand{ContainerID: shoppingID})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the refusal reported %v", err)
	}
	if len(h.containers.trashed) != 0 || len(h.events.appended) != 0 {
		t.Error("a refused deletion wrote something")
	}
	if h.uow.writes != 0 {
		t.Error("a refused deletion opened a write transaction")
	}
}

// Every container the act covered gets its own change log entry, and nothing below them does. The
// split follows the visibility filter: a device that follows one collection rather than the hub above
// it would never see a change filed under the hub alone (offline-sync.md §3.1).
func TestEachContainerOfTheDeletionIsAnnouncedToOfflineClients(t *testing.T) {
	h := newContainerHarness()
	h.withHub(hubID, "Private")
	h.withCollection()
	h.containers.cascadedItems = 17

	if _, err := (TrashContainer{Writer: h.writer}).Execute(t.Context(), actor(),
		ContainerLifecycleCommand{ContainerID: hubID}); err != nil {
		t.Fatalf("the deletion was refused: %v", err)
	}

	if len(h.changes.recorded) != 2 {
		t.Fatalf("%d change log entries, want 2 - the hub and its collection", len(h.changes.recorded))
	}
	announced := map[shared.ID]changelog.Change{}
	for _, change := range h.changes.recorded {
		announced[change.EntityID] = change
		if change.Op != changelog.Delete {
			t.Errorf("%s is announced as a %s, want a %s", change.EntityID, change.Op, changelog.Delete)
		}
		if change.Payload != nil {
			t.Errorf("the tombstone of %s carries a payload: %v", change.EntityID, change.Payload)
		}
	}
	// The collection is filed under itself, so a device subscribed to it sees its own disappearance.
	if scope := announced[shoppingID].ContainerID; scope != shoppingID {
		t.Errorf("the collection is filed under %q, want itself", scope)
	}
	if scope := announced[hubID].ContainerID; scope != hubID {
		t.Errorf("the hub is filed under %q, want itself", scope)
	}
}

// And back out again, whole - with the batch that named the deletion cleared.
func TestRestoringAContainerBringsBackTheBatch(t *testing.T) {
	h := newContainerHarness()
	h.withHub(hubID, "Private")
	h.withCollection()
	h.containers.cascadedItems = 17
	ctx := t.Context()

	deleted, err := TrashContainer{Writer: h.writer}.Execute(ctx, actor(),
		ContainerLifecycleCommand{ContainerID: hubID})
	if err != nil {
		t.Fatalf("the deletion was refused: %v", err)
	}

	restored, err := RestoreContainer{Writer: h.writer}.Execute(ctx, actor(),
		ContainerLifecycleCommand{ContainerID: hubID})
	if err != nil {
		t.Fatalf("the restore was refused: %v", err)
	}

	if restored.IsTrashed() || !restored.TrashBatchID.IsZero() {
		t.Errorf("the hub is still in the trash: %+v", restored)
	}
	if h.containers.stored[shoppingID].IsTrashed() {
		t.Error("the collection is still in the trash")
	}
	if len(h.containers.restored) != 1 || h.containers.restored[0].BatchID != deleted.TrashBatchID {
		t.Errorf("the restore was keyed on %v, want the deletion's batch %q",
			h.containers.restored, deleted.TrashBatchID)
	}
	if h.events.appended[1].Type != event.ContainerRestored {
		t.Errorf("the second event is %s, want %s", h.events.appended[1].Type, event.ContainerRestored)
	}
}

// Deleting twice is the state the caller asked for and costs nothing - and, the part that matters,
// does not re-date the deletion: the retention period runs off that stamp.
func TestDeletingAContainerTwiceDoesNotRestartTheRetentionPeriod(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()
	ctx := t.Context()

	first, err := TrashContainer{Writer: h.writer}.Execute(ctx, actor(),
		ContainerLifecycleCommand{ContainerID: shoppingID})
	if err != nil {
		t.Fatalf("the first deletion was refused: %v", err)
	}

	second, err := TrashContainer{Writer: h.writer}.Execute(ctx, actor(),
		ContainerLifecycleCommand{ContainerID: shoppingID})
	if err != nil {
		t.Fatalf("the second deletion was refused: %v", err)
	}
	if !second.DeletedAt.Equal(*first.DeletedAt) || second.TrashBatchID != first.TrashBatchID {
		t.Error("the second deletion re-dated the first or issued a new batch")
	}
	if len(h.events.appended) != 1 {
		t.Errorf("%d events, want 1", len(h.events.appended))
	}
}

// I-C3: deleting out of an archived subtree is a write into one. The hub archived in its own right is
// a different case and goes in - Archived --> Trashed is a documented edge.
func TestDeletingOutOfAnArchivedHubIsRefusedAndTheHubItselfIsNot(t *testing.T) {
	h := newContainerHarness()
	collection := h.withCollection()
	collection.ParentArchivedAt = &archivedEarly
	h.containers.stored[shoppingID] = collection

	_, err := TrashContainer{Writer: h.writer}.Execute(t.Context(), actor(),
		ContainerLifecycleCommand{ContainerID: shoppingID})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("deleting out of an archived hub reported %v, want a conflict", err)
	}

	archivedHub := h.withHub(hubID, "Private")
	archivedHub.ArchivedAt = &archivedEarly
	h.containers.stored[hubID] = archivedHub

	deleted, err := TrashContainer{Writer: h.writer}.Execute(t.Context(), actor(),
		ContainerLifecycleCommand{ContainerID: hubID})
	if err != nil {
		t.Fatalf("deleting an archived hub was refused: %v", err)
	}
	if !deleted.IsArchived() {
		t.Error("the archive stamp was lifted on the way into the trash")
	}
}

// The audit entry says how much went. "Somebody deleted a hub" and "somebody deleted a hub with four
// hundred entries in it" are different events to whoever has to answer for the second.
func TestTheDeletionAuditEntryRecordsHowMuchWent(t *testing.T) {
	h := newContainerHarness()
	h.withHub(hubID, "Private")
	h.withCollection()
	h.containers.cascadedItems = 17

	if _, err := (TrashContainer{Writer: h.writer}).Execute(t.Context(), actor(),
		ContainerLifecycleCommand{ContainerID: hubID}); err != nil {
		t.Fatalf("the deletion was refused: %v", err)
	}

	entry := h.audit.entries[0]
	if entry.Action != ContainerDeletedAction {
		t.Fatalf("the audit action is %s, want %s", entry.Action, ContainerDeletedAction)
	}
	if entry.Severity != audit.SeverityNotice {
		t.Errorf("severity %s, want a notice - this is the act that takes work with it", entry.Severity)
	}
	for field, want := range map[string]string{"collections": "1", "items": "17"} {
		got, _ := entry.Changes[field].(map[string]any)["to"].(string)
		if got != want {
			t.Errorf("the entry records %s = %q, want %q", field, got, want)
		}
	}
	for field, change := range entry.Changes {
		if to, _ := change.(map[string]any)["to"].(string); to == "Private" || to == "Shopping" {
			t.Errorf("a container name reached the audit trail as %s", field)
		}
	}
}

// The deletion is declared destructive, so an agent client asks before calling it (ai-first.md §1.1).
func TestTheContainerDeletionIsDeclaredDestructiveAndTheRestoreIsNot(t *testing.T) {
	if !(TrashContainer{}).Descriptor().Destructive {
		t.Error("TrashContainer is not declared destructive")
	}
	if (RestoreContainer{}).Descriptor().Destructive {
		t.Error("RestoreContainer is declared destructive")
	}
}

// A container's deletion schedules the tenant's sweep for the same reason an entry's does, and the
// restore asks for nothing.
func TestDeletingAContainerSchedulesTheSweepAndRestoringDoesNot(t *testing.T) {
	h := newContainerHarness()
	h.withCollection()
	ctx := t.Context()

	if _, err := (TrashContainer{Writer: h.writer}).Execute(ctx, actor(),
		ContainerLifecycleCommand{ContainerID: shoppingID}); err != nil {
		t.Fatalf("the deletion was refused: %v", err)
	}
	if len(h.jobs.enqueued) != 1 || h.jobs.enqueued[0].Kind != queue.KindRetentionSweep {
		t.Fatalf("the jobs are %v, want one retention sweep", h.jobs.enqueued)
	}
	if h.jobs.enqueued[0].TenantID != tenantID {
		t.Errorf("the job is for %q, want the tenant", h.jobs.enqueued[0].TenantID)
	}

	if _, err := (RestoreContainer{Writer: h.writer}).Execute(ctx, actor(),
		ContainerLifecycleCommand{ContainerID: shoppingID}); err != nil {
		t.Fatalf("the restore was refused: %v", err)
	}
	if len(h.jobs.enqueued) != 1 {
		t.Errorf("the restore scheduled a sweep: %v", h.jobs.enqueued)
	}
}
