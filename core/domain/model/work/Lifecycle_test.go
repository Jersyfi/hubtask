// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	trashedAt = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	batchOne  = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	batchTwo  = shared.MustParseID("0192f000-0000-7000-8000-0000000000b2")
)

// The state machine of domain-model.md §3.4, walked edge by edge. Each case starts from a stored
// state, applies one verb, and says where the two stamps end up - which is the whole of the
// machine, because the two stamps are the state.
func TestTheItemLifecycleWalksTheDocumentedEdges(t *testing.T) {
	archivedEarlier := trashedAt.Add(-24 * time.Hour)

	for _, test := range []struct {
		name string
		// from is the stored state the verb is applied to.
		archived, deleted *time.Time
		batch             shared.ID
		apply             func(WorkItem) (WorkItem, []FieldChange, error)
		// want is where the stamps end up, and which fields the change set names.
		wantArchived, wantDeleted bool
		wantBatch                 shared.ID
		wantFields                []string
		wantErr                   error
	}{
		{
			name:         "active, archived",
			apply:        func(i WorkItem) (WorkItem, []FieldChange, error) { return i.Archived(trashedAt) },
			wantArchived: true, wantFields: []string{FieldArchivedAt},
		},
		{
			name:       "archived, unarchived",
			archived:   &archivedEarlier,
			apply:      func(i WorkItem) (WorkItem, []FieldChange, error) { return i.Unarchived(trashedAt) },
			wantFields: []string{FieldArchivedAt},
		},
		{
			name:        "active, trashed",
			apply:       func(i WorkItem) (WorkItem, []FieldChange, error) { return i.Trashed(trashedAt, batchOne) },
			wantDeleted: true, wantBatch: batchOne, wantFields: []string{FieldDeletedAt},
		},
		{
			// The documented edge Archived --> Trashed. The archive stamp goes into the trash with
			// the item rather than being lifted on the way in.
			name:         "archived, trashed - and it stays archived",
			archived:     &archivedEarlier,
			apply:        func(i WorkItem) (WorkItem, []FieldChange, error) { return i.Trashed(trashedAt, batchOne) },
			wantArchived: true, wantDeleted: true, wantBatch: batchOne,
			wantFields: []string{FieldDeletedAt},
		},
		{
			// …and back out again the way it went in. Restore undoes the deletion and nothing else.
			name:     "trashed while archived, restored - and it is archived again",
			archived: &archivedEarlier, deleted: &trashedAt, batch: batchOne,
			apply:        func(i WorkItem) (WorkItem, []FieldChange, error) { return i.Restored(laterOn) },
			wantArchived: true, wantFields: []string{FieldDeletedAt},
		},
		{
			name:    "trashed, restored",
			deleted: &trashedAt, batch: batchOne,
			apply:      func(i WorkItem) (WorkItem, []FieldChange, error) { return i.Restored(laterOn) },
			wantFields: []string{FieldDeletedAt},
		},
		{
			// Not an edge the machine draws: the archive is a decision about work that is still
			// there. A conflict, so that a client is told restoring first is what would help.
			name:    "trashed, archived - refused",
			deleted: &trashedAt, batch: batchOne,
			apply:   func(i WorkItem) (WorkItem, []FieldChange, error) { return i.Archived(laterOn) },
			wantErr: shared.ErrConflict,
		},
		{
			name:    "trashed, unarchived - refused",
			deleted: &trashedAt, batch: batchOne,
			apply:   func(i WorkItem) (WorkItem, []FieldChange, error) { return i.Unarchived(laterOn) },
			wantErr: shared.ErrConflict,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := updatable(t)
			item.ArchivedAt, item.DeletedAt, item.TrashBatchID = test.archived, test.deleted, test.batch

			after, changes, err := test.apply(item)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("the transition was refused: %v", err)
			}

			if got := after.IsArchived(); got != test.wantArchived {
				t.Errorf("archived %v, want %v", got, test.wantArchived)
			}
			if got := after.IsTrashed(); got != test.wantDeleted {
				t.Errorf("trashed %v, want %v", got, test.wantDeleted)
			}
			if after.TrashBatchID != test.wantBatch {
				t.Errorf("batch %q, want %q", after.TrashBatchID, test.wantBatch)
			}
			if got := fieldNames(changes); !equalStrings(got, test.wantFields) {
				t.Errorf("the change set names %v, want %v", got, test.wantFields)
			}
		})
	}
}

// Every verb applied twice changes nothing the second time, and - the part that matters more -
// keeps the first time's timestamp. A retry after a lost response must not re-date a deletion:
// the retention period runs off that stamp, and a silently restarted one is thirty more days of
// storage nobody asked for.
func TestTheLifecycleVerbsAreIdempotent(t *testing.T) {
	for _, test := range []struct {
		name  string
		apply func(WorkItem) (WorkItem, []FieldChange, error)
	}{
		{"archive", func(i WorkItem) (WorkItem, []FieldChange, error) { return i.Archived(trashedAt) }},
		{"trash", func(i WorkItem) (WorkItem, []FieldChange, error) { return i.Trashed(trashedAt, batchOne) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			once, _, err := test.apply(updatable(t))
			if err != nil {
				t.Fatalf("the first application was refused: %v", err)
			}

			twice, changes, err := test.apply(once)
			if err != nil {
				t.Fatalf("the second application was refused: %v", err)
			}
			if len(changes) != 0 {
				t.Errorf("the second application reports %d changes, want none", len(changes))
			}
			if !stampsEqual(once, twice) {
				t.Error("the second application moved a timestamp the first one set")
			}
		})
	}

	t.Run("unarchive and restore on an item in neither state", func(t *testing.T) {
		item := updatable(t)
		for _, apply := range []func(WorkItem) (WorkItem, []FieldChange, error){
			func(i WorkItem) (WorkItem, []FieldChange, error) { return i.Unarchived(trashedAt) },
			func(i WorkItem) (WorkItem, []FieldChange, error) { return i.Restored(trashedAt) },
		} {
			after, changes, err := apply(item)
			if err != nil || len(changes) != 0 {
				t.Errorf("lifting a stamp that is not set reported %v / %d changes", err, len(changes))
			}
			if after.UpdatedAt != item.UpdatedAt {
				t.Error("lifting a stamp that is not set touched updated_at")
			}
		}
	})
}

// The invariant a subtree deletion rests on (I-C2): a row already in the trash keeps its own
// deletion. Restamping it would fold somebody else's deletion into this batch, and restoring the
// batch would then bring back something this deletion never took.
func TestTrashingDoesNotAdoptAnEarlierDeletion(t *testing.T) {
	first, _, err := updatable(t).Trashed(trashedAt, batchOne)
	if err != nil {
		t.Fatalf("the first deletion was refused: %v", err)
	}

	second, changes, err := first.Trashed(laterOn, batchTwo)
	if err != nil {
		t.Fatalf("the second deletion was refused: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("the second deletion reports %d changes, want none", len(changes))
	}
	if second.TrashBatchID != batchOne {
		t.Errorf("the row was adopted into batch %q, want %q", second.TrashBatchID, batchOne)
	}
	if !second.DeletedAt.Equal(trashedAt) {
		t.Errorf("the deletion was re-dated to %v, want %v", second.DeletedAt, trashedAt)
	}
}

// Restoring clears the batch as well as the stamp. A row that kept a spent batch identifier would
// be swept back up by a restore of a deletion it is no longer part of.
func TestRestoringClearsTheBatch(t *testing.T) {
	trashed, _, err := updatable(t).Trashed(trashedAt, batchOne)
	if err != nil {
		t.Fatalf("the deletion was refused: %v", err)
	}

	restored, _, err := trashed.Restored(laterOn)
	if err != nil {
		t.Fatalf("the restore was refused: %v", err)
	}
	if !restored.TrashBatchID.IsZero() {
		t.Errorf("the restored row still carries batch %q", restored.TrashBatchID)
	}
}

// A batch identifier is generated through the ID generator port by whoever starts the deletion
// (arc42 §8.13). A missing one is this process getting it wrong, not a client - so it is an
// internal error rather than a validation failure, and it never reaches a client as advice.
func TestTrashingWithoutABatchIsADefect(t *testing.T) {
	if _, _, err := updatable(t).Trashed(trashedAt, ""); !errors.Is(err, shared.ErrInternal) {
		t.Errorf("a missing batch reported %v, want an internal error", err)
	}
	container := Container{ID: taskID, TenantID: itemTenant, Type: ContainerHub, Name: "Private"}
	if _, _, err := container.Trashed(trashedAt, ""); !errors.Is(err, shared.ErrInternal) {
		t.Errorf("a missing batch reported %v, want an internal error", err)
	}
}

// I-C3 governs the way into the trash as well: deleting out of an archived subtree is a write into
// one. A hub archived in its own right is a different case and goes in - Archived --> Trashed is a
// documented edge, and the stamp travels with it.
func TestTrashingAContainerRespectsTheArchivedSubtree(t *testing.T) {
	archivedEarlier := trashedAt.Add(-24 * time.Hour)

	inArchivedHub := Container{
		ID: taskID, TenantID: itemTenant, Type: ContainerCollection, ParentID: packageID,
		Name: "Shopping", ParentArchivedAt: &archivedEarlier,
	}
	if _, _, err := inArchivedHub.Trashed(trashedAt, batchOne); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("deleting out of an archived hub reported %v, want a conflict", err)
	}

	archivedHub := Container{
		ID: packageID, TenantID: itemTenant, Type: ContainerHub, Name: "Private",
		ArchivedAt: &archivedEarlier,
	}
	trashed, _, err := archivedHub.Trashed(trashedAt, batchOne)
	if err != nil {
		t.Fatalf("deleting an archived hub was refused: %v", err)
	}
	if !trashed.IsArchived() {
		t.Error("the archive stamp was lifted on the way into the trash")
	}
	if trashed.TrashBatchID != batchOne {
		t.Errorf("batch %q, want %q", trashed.TrashBatchID, batchOne)
	}
}

// Every container verb applied twice changes nothing the second time, and keeps the first time's
// timestamp. The retention period runs off that stamp, so a silently restarted one is thirty more
// days of storage nobody asked for.
func TestTheContainerLifecycleVerbsAreIdempotent(t *testing.T) {
	hub := Container{ID: packageID, TenantID: itemTenant, Type: ContainerHub, Name: "Private"}

	once, _, err := hub.Trashed(trashedAt, batchOne)
	if err != nil {
		t.Fatalf("the first deletion was refused: %v", err)
	}

	twice, changes, err := once.Trashed(laterOn, batchTwo)
	if err != nil {
		t.Fatalf("the second deletion was refused: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("the second deletion reports %d changes, want none", len(changes))
	}
	if !twice.DeletedAt.Equal(trashedAt) || twice.TrashBatchID != batchOne {
		t.Error("the second deletion re-dated the first or adopted it into a new batch")
	}
}

// The container's way back, and that it clears the batch with the stamp.
func TestRestoringAContainer(t *testing.T) {
	hub := Container{ID: packageID, TenantID: itemTenant, Type: ContainerHub, Name: "Private"}

	trashed, _, err := hub.Trashed(trashedAt, batchOne)
	if err != nil {
		t.Fatalf("the deletion was refused: %v", err)
	}

	restored, changes, err := trashed.Restored(laterOn)
	if err != nil {
		t.Fatalf("the restore was refused: %v", err)
	}
	if restored.IsTrashed() || !restored.TrashBatchID.IsZero() {
		t.Error("the restored container is still in the trash")
	}
	if got := fieldNames(changes); !equalStrings(got, []string{FieldDeletedAt}) {
		t.Errorf("the change set names %v, want %v", got, []string{FieldDeletedAt})
	}

	again, changes, err := restored.Restored(laterOn)
	if err != nil || len(changes) != 0 {
		t.Errorf("restoring what is not in the trash reported %v / %d changes", err, len(changes))
	}
	if again.UpdatedAt != restored.UpdatedAt {
		t.Error("restoring what is not in the trash touched updated_at")
	}
}

func fieldNames(changes []FieldChange) []string {
	names := make([]string, 0, len(changes))
	for _, change := range changes {
		names = append(names, change.Field)
	}
	return names
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// stampsEqual compares the two lifecycle timestamps, which is what an idempotent verb has to leave
// alone.
func stampsEqual(a, b WorkItem) bool {
	return equalInstants(a.ArchivedAt, b.ArchivedAt) && equalInstants(a.DeletedAt, b.DeletedAt)
}

func equalInstants(a, b *time.Time) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	}
	return a.Equal(*b)
}
