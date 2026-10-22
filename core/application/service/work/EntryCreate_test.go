// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-WRK-24 check 3, creation: an entry is created only at a level its collection offers, with
// only the values it offers.
func TestAnEntryIsCreatedOnlyAtAnOfferedLevel(t *testing.T) {
	h := newItemHarness()
	h.handler.Gate = gateOver(h.items, h.containers)
	task := h.withTask()
	pack := h.withWorkPackage(task)
	ctx := context.Background()

	narrowTo(t, h.containers, collectionID, domain.PresetTasks)

	_, _, err := h.handler.Execute(ctx, itemActor(), CreateWorkItemCommand{
		Type: domain.ItemWorkPackage, CollectionID: collectionID, ParentID: task.ID, Title: "Aisle",
	})
	refusedFor(t, err, "items.level_hidden")

	// An activity directly under a task is offered by Tasks; one under the hidden work package is
	// not, though its type is.
	if _, _, err := h.handler.Execute(ctx, itemActor(), CreateWorkItemCommand{
		Type: domain.ItemActivity, CollectionID: collectionID, ParentID: task.ID, Title: "Call",
	}); err != nil {
		t.Fatalf("an activity under a task was refused in Tasks: %v", err)
	}
	_, _, err = h.handler.Execute(ctx, itemActor(), CreateWorkItemCommand{
		Type: domain.ItemActivity, CollectionID: collectionID, ParentID: pack.ID, Title: "Call",
	})
	refusedFor(t, err, "items.level_hidden")
}

func TestAHiddenValueIsRefusedOnCreate(t *testing.T) {
	h := newItemHarness()
	h.handler.Gate = gateOver(h.items, h.containers)
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	cmd := taskCommand()
	cmd.Notes = "Oat, not soy"
	_, _, err := h.handler.Execute(context.Background(), itemActor(), cmd)
	refusedFor(t, err, "items.capability_hidden")
	if len(h.items.inserted) != 0 {
		t.Fatalf("a refused create inserted %d entries", len(h.items.inserted))
	}

	if _, _, err := h.handler.Execute(context.Background(), itemActor(), taskCommand()); err != nil {
		t.Fatalf("a plain task was refused in List: %v", err)
	}
}
