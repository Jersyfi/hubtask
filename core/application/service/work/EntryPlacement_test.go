// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-WRK-24 check 3, placement: the destination decides the level; what an entry carries is kept.

func TestAnEntryMovesOnlyToALevelTheDestinationOffers(t *testing.T) {
	h := newPlacementHarness()
	h.writer.Gate = gateOver(h.items, h.containers)
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	// The work package is hidden in List: putting it under the other task keeps it at a level the
	// collection does not offer.
	_, err := MoveWorkItem{Placement: h.writer}.Execute(t.Context(), placementActor(), MoveWorkItemCommand{
		ItemID: movedPackID, TargetParentID: targetTaskID, ParentGiven: true,
	})
	refusedFor(t, err, "items.level_hidden")
	// Reordering it among its siblings is a write on it too.
	_, err = ReorderWorkItem{Placement: h.writer}.Execute(t.Context(), placementActor(), ReorderWorkItemCommand{
		ItemID: movedPackID,
	})
	refusedFor(t, err, "items.level_hidden")
	if len(h.items.moves) != 0 || len(h.items.ranks) != 0 {
		t.Fatal("a refused placement wrote something")
	}

	// A task is offered: reordering it, with the hidden work package and activity under it, is
	// not refused.
	if _, err := (ReorderWorkItem{Placement: h.writer}).Execute(t.Context(), placementActor(), ReorderWorkItemCommand{
		ItemID: sourceTaskID,
	}); err != nil {
		t.Fatalf("reordering a shown task was refused: %v", err)
	}
}

func TestACopyIntoANarrowerCollectionKeepsEveryValue(t *testing.T) {
	h := newDuplicateHarness(t)
	task := h.withTask()
	h.withSubtree(task)
	h.withField(farCollectionID, "priority")
	h.handler.Gate = gateOver(h.items, h.containers)
	narrowTo(t, h.containers, farCollectionID, domain.PresetList)

	result, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{
		ItemID: task.ID, IncludeSubtree: true, TargetCollectionID: farCollectionID,
	})
	if err != nil {
		t.Fatalf("copying a task into List was refused: %v", err)
	}
	if result.Copied != 3 {
		t.Errorf("copied %d entries, want the task with its hidden work package and activity", result.Copied)
	}
	copied := h.items.stored[result.Item.ID]
	if copied.Notes != task.Notes || copied.Cover == nil || copied.AssigneeID != task.AssigneeID {
		t.Errorf("the copy lost values its destination hides: %+v", copied)
	}
	for _, reference := range result.DroppedReferences {
		if reference.Code == "items.capability_not_supported" || reference.Code == "items.capability_hidden" {
			t.Errorf("a value the structure hides was dropped: %+v", reference)
		}
	}
}

func TestACopyIsNotPlacedOnAHiddenLevel(t *testing.T) {
	h := newDuplicateHarness(t)
	task := h.withTask()
	pack, _ := h.withSubtree(task)
	h.handler.Gate = gateOver(h.items, h.containers)
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	_, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{ItemID: pack.ID})
	refusedFor(t, err, "items.level_hidden")
}
