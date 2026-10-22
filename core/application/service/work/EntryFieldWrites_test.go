// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// refusedFor asserts a write was refused for what the collection hides, with the detail named.
func refusedFor(t *testing.T, err error, detail string) {
	t.Helper()
	var refusal *shared.Error
	if !errors.As(err, &refusal) || !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("answered %v, want capability_not_supported", err)
	}
	if refusal.DetailCode != detail {
		t.Fatalf("detail %q, want %s", refusal.DetailCode, detail)
	}
}

// UC-WRK-24 check 3, the entry's own fields: in a List collection a due date is hidden, so setting
// one and clearing the one it holds are both refused, nothing is written, and the date is there
// again, writable, once the collection offers dates.
func TestAHiddenDueDateIsNeitherSetNorCleared(t *testing.T) {
	h := newDueDateHarness(t, dueDateProfiles())
	item := h.withItem(domain.ItemTask)
	held, _ := domain.NewDueDate(&berlinFriday, false, "Europe/Berlin")
	item.Due = held
	h.items.stored[dueItem] = item
	gate := gateOver(h.items, h.containers)
	h.set.Due.Gate, h.clear.Due.Gate = gate, gate
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	_, err := h.set.Execute(t.Context(), actor(), dueCmd(t))
	refusedFor(t, err, "items.capability_hidden")
	_, err = h.clear.Execute(t.Context(), actor(), DueDateCommand{ItemID: dueItem})
	refusedFor(t, err, "items.capability_hidden")
	if len(h.items.dueDates) != 0 || len(h.events.appended) != 0 || len(h.history.entries) != 0 {
		t.Fatalf("a refused write left %d writes, %d events, %d history entries",
			len(h.items.dueDates), len(h.events.appended), len(h.history.entries))
	}
	if h.items.stored[dueItem].Due == nil {
		t.Fatal("the hidden due date is gone")
	}

	widen(h.containers, collectionID)
	if _, err := h.clear.Execute(t.Context(), actor(), DueDateCommand{ItemID: dueItem}); err != nil {
		t.Fatalf("widened, clearing was refused: %v", err)
	}
}

func TestHiddenNotesAreRefusedAndAHiddenLevelIsNotRenamed(t *testing.T) {
	h := newUpdateHarness()
	task := completionItem(taskID, domain.ItemTask, "")
	pack := completionItem(packageID, domain.ItemWorkPackage, taskID)
	pack.Path = task.ChildPath(packageID)
	h.items.stored[taskID], h.items.stored[packageID] = task, pack
	h.handler.Gate = gateOver(h.items, h.containers)
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	notes, title := "Oat, not soy", "Renamed"
	_, err := h.handler.Execute(t.Context(), actor(), UpdateCommand{
		ItemID: taskID, Attributes: domain.ItemAttributes{Notes: &notes},
	})
	refusedFor(t, err, "items.capability_hidden")

	_, err = h.handler.Execute(t.Context(), actor(), UpdateCommand{
		ItemID: packageID, Attributes: domain.ItemAttributes{Title: &title},
	})
	refusedFor(t, err, "items.level_hidden")

	// The task itself is offered: its title is written as before.
	if _, err := h.handler.Execute(t.Context(), actor(), UpdateCommand{
		ItemID: taskID, Attributes: domain.ItemAttributes{Title: &title},
	}); err != nil {
		t.Fatalf("renaming a shown task was refused: %v", err)
	}
}

func TestAHiddenCoverIsNeitherSetNorCleared(t *testing.T) {
	h := newCoverHarness(t)
	h.withItem(domain.ItemTask, &domain.Cover{Kind: domain.CoverColor, ColorToken: "surface.sand"})
	gate := gateOver(h.items, h.containers)
	h.set.Cover.Gate, h.clear.Cover.Gate = gate, gate
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	_, err := h.set.Execute(t.Context(), actor(), colourCmd("surface.moss"))
	refusedFor(t, err, "items.capability_hidden")
	_, err = h.clear.Execute(t.Context(), actor(), CoverCommand{ItemID: coveredItem})
	refusedFor(t, err, "items.capability_hidden")
	if h.items.stored[coveredItem].Cover == nil {
		t.Fatal("the hidden cover is gone")
	}
}

func TestAHiddenAssigneeIsNeitherSetNorRemoved(t *testing.T) {
	h := newAssignmentHarness(t)
	h.withItem(domain.ItemTask)
	gate := gateOver(h.items, h.containers)
	h.assign.Assignment.Gate, h.unassign.Assignment.Gate = gate, gate
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	_, err := h.assign.Execute(t.Context(), actor(), assignCmd())
	refusedFor(t, err, "items.capability_hidden")
	_, err = h.unassign.Execute(t.Context(), actor(), AssignmentCommand{ItemID: assignedItem})
	refusedFor(t, err, "items.capability_hidden")

	widen(h.containers, collectionID)
	if _, err := h.assign.Execute(t.Context(), actor(), assignCmd()); err != nil {
		t.Fatalf("widened, assigning was refused: %v", err)
	}
}

func TestAHiddenCustomFieldIsRefused(t *testing.T) {
	h := newValueHarness(t)
	h.withItem(domain.ItemTask)
	h.withDefinition(t, collectionID, "budget", domain.CustomFieldNumber)
	h.set.Gate = gateOver(h.items, h.containers)
	collection := h.containers.stored[collectionID]
	collection.Structure = domain.StructureLayer{Values: domain.StructureValues{
		Fields: map[domain.ItemType]domain.LevelFields{
			domain.ItemTask: {Capabilities: []domain.Capability{domain.CapabilityCompletion}},
		},
	}}
	h.containers.stored[collectionID] = collection

	_, err := h.set.Execute(t.Context(), actor(), SetCustomFieldCommand{
		ItemID: filledItem, Key: "budget", Value: 120.0,
	})
	refusedFor(t, err, "items.capability_hidden")
}

func TestAnEntryOfAHiddenLevelIsNotCompleted(t *testing.T) {
	h := newCompletionHarness(domain.CompletionManual)
	h.writer.Gate = gateOver(h.items, h.containers)
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	_, err := CompleteWorkItem{Completion: h.writer}.Execute(t.Context(), actor(), CompletionCommand{ItemID: packageID})
	refusedFor(t, err, "items.level_hidden")
	if h.completed(packageID) {
		t.Fatal("the hidden work package was completed")
	}
	// Completion is never hidden: a task in List is completed as ever.
	if _, err := (CompleteWorkItem{Completion: h.writer}).Execute(t.Context(), actor(), CompletionCommand{ItemID: taskID}); err != nil {
		t.Fatalf("completing a shown task was refused: %v", err)
	}
}
