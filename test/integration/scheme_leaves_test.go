// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	workflowservice "github.com/Jersyfi/hubtask/core/application/service/workflow"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// What the web app's "convert to columns" stands on before Workflows is switched off (UC-INS-21
// check 14, domain-model.md §3.5a): the preview of removing a scheme row answers, changing nothing,
// each open entry's status by collection and the board going from WORKFLOW to COLUMNS, counting the
// one open entry the done column holds as moved; the removal then keeps every other entry in the
// column that showed its status, done ones done, moves that open one to the first column, and
// clears the status.
func TestRemovingAWorkflowKeepsEveryEntryInTheColumnOfItsStatus(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	administrator := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantA, AccountID: authorA}
	desk := schemeDesk(ctx, t)
	key := seedSchemeWorkflow(ctx, t, tenantA, authorA)

	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	todo := seedBoardColumn(ctx, t, tenantA, collection, "a0", false)
	doing := seedBoardColumn(ctx, t, tenantA, collection, "a1", false)
	done := seedBoardColumn(ctx, t, tenantA, collection, "a2", true)
	if _, err := (workflowservice.SetWorkflowScheme{Desk: desk}).Descriptor().Handler.Invoke(ctx, administrator, usecase.Input{
		"container_id": collection.String(), "workflow": key, "entry_type": "task", "confirm": true,
		"columns": []any{
			map[string]any{"bucket_id": todo.ID.String(), "statuses": []any{"todo"}},
			map[string]any{"bucket_id": doing.ID.String(), "statuses": []any{"doing", "review"}},
			map[string]any{"bucket_id": done.ID.String(), "statuses": []any{"done"}},
		},
	}); err != nil {
		t.Fatalf("running the workflow: %v", err)
	}
	inTodo := seedOnBoard(ctx, t, tenantA, authorA, collection, todo.ID, false, "todo")
	inDoing := seedOnBoard(ctx, t, tenantA, authorA, collection, doing.ID, false, "doing")
	inReview := seedOnBoard(ctx, t, tenantA, authorA, collection, doing.ID, false, "review")
	inDone := seedOnBoard(ctx, t, tenantA, authorA, collection, done.ID, true, "done")
	// Open, yet standing in a status the done column shows.
	openInDone := seedOnBoard(ctx, t, tenantA, authorA, collection, done.ID, false, "done")

	leave := usecase.Input{"container_id": collection.String(), "entry_type": "task"}
	preview, err := workflowservice.PreviewWorkflowScheme{Desk: desk}.Descriptor().Handler.Invoke(ctx, administrator, leave)
	if err != nil {
		t.Fatalf("the preview: %v", err)
	}
	open := map[string]int{}
	for _, move := range preview["moves"].([]usecase.Output) {
		if move["collection_id"] != collection.String() || move["to"] != nil {
			t.Errorf("a move %v; wants every status of this collection to none", move)
		}
		if from, _ := move["from"].(string); move["completed"] == false {
			open[from] += move["count"].(int)
		}
	}
	if open["todo"] != 1 || open["doing"] != 1 || open["review"] != 1 || open["done"] != 1 || len(open) != 4 {
		t.Errorf("open entries by status %v; wants todo, doing, review and done once each", open)
	}
	boards, _ := preview["boards"].([]usecase.Output)
	if len(boards) != 1 || boards[0]["level_before"] != "WORKFLOW" || boards[0]["level_after"] != "COLUMNS" ||
		boards[0]["done_bucket_id"] != done.ID.String() || boards[0]["moved"] != 1 {
		t.Fatalf("the board %v; wants WORKFLOW → COLUMNS, the done column kept, the open one in it moved", boards)
	}
	if entry := readEntry(ctx, t, tenantA, inDoing); entry.Status != "doing" {
		t.Fatalf("the preview changed the entry: %q", entry.Status)
	}

	leave["confirm"] = true
	if _, err := (workflowservice.SetWorkflowScheme{Desk: desk}).Descriptor().Handler.Invoke(ctx, administrator, leave); err != nil {
		t.Fatalf("the removal: %v", err)
	}
	for id, want := range map[shared.ID]shared.ID{
		inTodo: todo.ID, inDoing: doing.ID, inReview: doing.ID, inDone: done.ID, openInDone: todo.ID,
	} {
		entry := readEntry(ctx, t, tenantA, id)
		if entry.Status != "" || entry.BucketID != want || entry.Completion.IsCompleted != (id == inDone) {
			t.Errorf("entry %s: %q in %s, done %v; wants no status in %s", id, entry.Status, entry.BucketID, entry.Completion.IsCompleted, want)
		}
	}
}
