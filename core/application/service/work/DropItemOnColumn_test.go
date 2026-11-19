// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// A drop without a workflow moves the entry, and its completion follows the column (UC-WRK-05).
func TestADropOnABoardWithoutAWorkflowMoves(t *testing.T) {
	h := coupledUpdateHarness()
	h.handler.Completion.Boards = h.handler.Boards
	drop := DropItemOnColumn{Update: h.handler, Step: TakeStep{Completion: h.handler.Completion}}

	answer, err := drop.Execute(t.Context(), actorFixture(), DropCommand{ItemID: taskID, BucketID: doneColumn})
	if err != nil {
		t.Fatalf("dropping: %v", err)
	}
	if answer.Outcome != domain.DropMoved || answer.Item.BucketID != doneColumn || !answer.Item.Completion.IsCompleted {
		t.Errorf("answered %s, in %s, done %v", answer.Outcome, answer.Item.BucketID, answer.Item.Completion.IsCompleted)
	}
	_, err = drop.Execute(t.Context(), actorFixture(), DropCommand{ItemID: taskID})
	refusedWith(t, err, "board.column_required")
	_, err = drop.Execute(t.Context(), actorFixture(), DropCommand{ItemID: taskID, BucketID: accountID})
	refusedWith(t, err, "buckets.not_found")
}

// workflowBoard maps the review workflow onto the three columns: To do, In review, and Done with
// Rejected in the done column.
func workflowBoard(t *testing.T, role identity.Role) (*completionHarness, DropItemOnColumn) {
	t.Helper()
	h, _ := workflowHarness(t)
	h.writer.Workflow.Roles = roleOf{role: role}
	board := &buckets{stored: map[shared.ID]domain.Bucket{}}
	threeColumnBoard(board)
	h.writer.Boards = Boards{Buckets: board, Statuses: &mapping{rows: []domain.ColumnStatus{
		{BucketID: todoColumn, Status: "todo"}, {BucketID: doingColumn, Status: "review"},
		{BucketID: doneColumn, Status: "done"}, {BucketID: doneColumn, Status: "rejected"},
	}}}
	task := h.items.stored[taskID]
	task.BucketID = doingColumn
	task.CustomFields = map[string]any{"resolution": "fixed"}
	h.items.stored[taskID] = task
	return h, DropItemOnColumn{Step: TakeStep{Completion: h.writer}}
}

// Under a workflow a drop takes the one step, offers several and writes nothing then, and refuses
// none with a reason (UC-FLW-06, UC-WRK-05, UC-FLW-03 check 8).
func TestADropUnderAWorkflowTakesChoosesOrRefuses(t *testing.T) {
	t.Run("one step is taken", func(t *testing.T) {
		h, drop := workflowBoard(t, identity.RoleMember)
		answer, err := drop.Execute(t.Context(), actorFixture(), DropCommand{ItemID: taskID, BucketID: todoColumn})
		if err != nil {
			t.Fatalf("dropping: %v", err)
		}
		if answer.Outcome != domain.DropStep || answer.Steps[0].Key != "back" || answer.Item.Status != "todo" {
			t.Errorf("answered %s %+v, status %s", answer.Outcome, answer.Steps, answer.Item.Status)
		}
		if h.items.stored[taskID].BucketID != todoColumn {
			t.Errorf("the column did not follow the status: %s", h.items.stored[taskID].BucketID)
		}
	})
	t.Run("several are offered and nothing is written", func(t *testing.T) {
		h, drop := workflowBoard(t, identity.RoleOwner)
		answer, err := drop.Execute(t.Context(), actorFixture(), DropCommand{ItemID: taskID, BucketID: doneColumn})
		if err != nil {
			t.Fatalf("dropping: %v", err)
		}
		if answer.Outcome != domain.DropChoose || len(answer.Steps) != 2 {
			t.Errorf("answered %s %+v", answer.Outcome, answer.Steps)
		}
		if len(h.items.completions) != 0 || h.items.stored[taskID].Status != "review" {
			t.Error("a choice wrote the entry")
		}
		answer, err = drop.Execute(t.Context(), actorFixture(), DropCommand{ItemID: taskID, BucketID: doneColumn, Step: "accept"})
		if err != nil || answer.Outcome != domain.DropStep || answer.Item.Status != "done" {
			t.Errorf("the chosen step: %s %s, %v", answer.Outcome, answer.Item.Status, err)
		}
	})
	t.Run("a step the person may not take is refused as blocked", func(t *testing.T) {
		h, drop := workflowBoard(t, identity.RoleMember)
		task := h.items.stored[taskID]
		task.Status, task.BucketID = "todo", todoColumn
		h.items.stored[taskID] = task
		_, err := drop.Execute(t.Context(), actorFixture(), DropCommand{ItemID: taskID, BucketID: doneColumn})
		refusedWith(t, err, "board.step_blocked")
	})
}
