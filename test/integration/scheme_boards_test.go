// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	workflowservice "github.com/Jersyfi/hubtask/core/application/service/workflow"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A change of the workflow in force on a real board (UC-FLW-02 check 4, domain-model.md §3.5a):
// the preview answers the board - levels, columns with statuses, unmapped ones, moves -, the set
// writes the mapping and puts every entry into the column of its status, and taking the workflow
// away keeps the columns and completion and places the entries by completion.

func schemeDesk(ctx context.Context, t *testing.T) workflowservice.Desk {
	t.Helper()
	columns := boardRepair(ctx, t).Columns
	workflows := postgres.NewWorkflowRepository()
	journal := work.ActivityJournal{Entries: historyRepo(), IDs: columns.IDs}
	return workflowservice.Desk{
		Workflows: workflows, Containers: columns.Containers, Items: itemRepo(),
		Mover: work.StatusMover{
			Workflows: workflows, Activity: journal, Changes: columns.Changes, HLC: columns.HLC, Boards: columns.Boards,
		},
		Authorizer: columns.Authorizer, Changes: columns.Changes, Audit: columns.Audit,
		UnitOfWork: columns.UnitOfWork, Clock: columns.Clock, IDs: columns.IDs, HLC: columns.HLC, Text: text.Composing{},
		Boards: work.SchemeBoards{Columns: columns}, Gate: work.WorkflowGate{Workflows: workflows},
	}
}

func seedSchemeWorkflow(ctx context.Context, t *testing.T, tenant, author shared.ID) string {
	t.Helper()
	key := fmt.Sprintf("board_%d", fixtureCounter.Add(1))
	flow, err := domain.NewWorkflow(domain.NewWorkflowInput{
		ID: freshID(t), TenantID: tenant, Key: key, Name: "Board",
		Shape: domain.WorkflowShape{Statuses: []domain.WorkflowStatus{
			{Key: "todo", Name: "To do", Category: domain.CategoryTodo},
			{Key: "doing", Name: "Doing", Category: domain.CategoryInProgress},
			{Key: "review", Name: "Review", Category: domain.CategoryInProgress},
			{Key: "done", Name: "Done", Category: domain.CategoryDone},
		}, Initial: "todo"},
		By: author, Now: created, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.NewWorkflowRepository().Insert(ctx, flow)
	}); err != nil {
		t.Fatalf("seeding the workflow: %v", err)
	}
	return key
}

func TestASchemeChangeCarriesTheBoard(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	administrator := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantA, AccountID: authorA}
	desk := schemeDesk(ctx, t)
	key := seedSchemeWorkflow(ctx, t, tenantA, authorA)

	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	todo := seedRepairColumn(ctx, t, tenantA, collection, "a0", false)
	doing := seedRepairColumn(ctx, t, tenantA, collection, "a1", false)
	done := seedRepairColumn(ctx, t, tenantA, collection, "a2", true)
	inDoing := seedOnBoard(ctx, t, tenantA, authorA, collection, doing.ID, false, "")
	loose := seedOnBoard(ctx, t, tenantA, authorA, collection, "", true, "")
	inDone := seedOnBoard(ctx, t, tenantA, authorA, collection, done.ID, true, "")

	in := usecase.Input{"container_id": collection.String(), "workflow": key, "entry_type": "task"}
	preview, err := workflowservice.PreviewWorkflowScheme{Desk: desk}.Descriptor().Handler.Invoke(ctx, administrator, in)
	if err != nil {
		t.Fatalf("the preview: %v", err)
	}
	boards, _ := preview["boards"].([]usecase.Output)
	if len(boards) != 1 {
		t.Fatalf("boards %v, want this collection's", preview["boards"])
	}
	board := boards[0]
	if board["level_before"] != "COLUMNS" || board["level_after"] != "WORKFLOW" {
		t.Errorf("levels %v → %v", board["level_before"], board["level_after"])
	}
	// Open in Doing takes the initial status, so goes to To do; the loose done one into Done.
	if board["moved"] != 2 || board["column_only"] != 0 {
		t.Errorf("moved %v, column only %v; want 2, 0", board["moved"], board["column_only"])
	}
	unmapped, _ := board["unmapped"].([]usecase.Output)
	if len(unmapped) != 1 || unmapped[0]["status"] != "review" {
		t.Errorf("unmapped %v, want review", unmapped)
	}

	t.Run("the set writes the chosen columns and places every entry", func(t *testing.T) {
		in := usecase.Input{
			"container_id": collection.String(), "workflow": key, "entry_type": "task", "confirm": true,
			"columns": []any{
				map[string]any{"bucket_id": todo.ID.String(), "statuses": []any{"todo"}},
				map[string]any{"bucket_id": doing.ID.String(), "statuses": []any{"doing", "review"}},
				map[string]any{"bucket_id": done.ID.String(), "statuses": []any{"done"}},
			},
		}
		if _, err := (workflowservice.SetWorkflowScheme{Desk: desk}).Descriptor().Handler.Invoke(ctx, administrator, in); err != nil {
			t.Fatalf("the set: %v", err)
		}
		mapping := readMapping(ctx, t, tenantA, collection)
		if len(mapping) != 4 || !slices.Contains(mapping, domain.ColumnStatus{BucketID: doing.ID, Status: "review"}) {
			t.Errorf("mapping %v", mapping)
		}
		if entry := readEntry(ctx, t, tenantA, inDoing); entry.Status != "todo" || entry.BucketID != todo.ID {
			t.Errorf("open in Doing: %q in %s", entry.Status, entry.BucketID)
		}
		if entry := readEntry(ctx, t, tenantA, loose); entry.Status != "done" || entry.BucketID != done.ID {
			t.Errorf("done without a column: %q in %s", entry.Status, entry.BucketID)
		}
	})

	t.Run("a column of no board reached is refused", func(t *testing.T) {
		in := usecase.Input{
			"container_id": collection.String(), "workflow": "", "entry_type": "task",
			"columns": []any{map[string]any{"bucket_id": freshID(t).String(), "statuses": []any{"todo"}}},
		}
		_, err := workflowservice.PreviewWorkflowScheme{Desk: desk}.Descriptor().Handler.Invoke(ctx, administrator, in)
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("answered %v", err)
		}
	})

	t.Run("the workflow goes, columns and completion stay", func(t *testing.T) {
		in := usecase.Input{"container_id": collection.String(), "workflow": "", "entry_type": "task", "confirm": true}
		if _, err := (workflowservice.SetWorkflowScheme{Desk: desk}).Descriptor().Handler.Invoke(ctx, administrator, in); err != nil {
			t.Fatalf("the removal: %v", err)
		}
		if mapping := readMapping(ctx, t, tenantA, collection); len(mapping) != 0 {
			t.Errorf("the mapping stayed: %v", mapping)
		}
		if len(liveColumns(ctx, t, tenantA, collection)) != 3 {
			t.Error("the columns went with the workflow")
		}
		for id, want := range map[shared.ID]shared.ID{inDoing: todo.ID, loose: done.ID, inDone: done.ID} {
			entry := readEntry(ctx, t, tenantA, id)
			if entry.Status != "" || entry.BucketID != want || entry.Completion.IsCompleted != (want == done.ID) {
				t.Errorf("entry %s: %q in %s, done %v", id, entry.Status, entry.BucketID, entry.Completion.IsCompleted)
			}
		}
	})

	t.Run("another workspace's collection is not found", func(t *testing.T) {
		stranger := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantB, AccountID: authorA}
		_, err := workflowservice.PreviewWorkflowScheme{Desk: desk}.Descriptor().Handler.Invoke(ctx, stranger, in)
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("answered %v", err)
		}
	})
}

// Workflow aus diesen Spalten (domain-model.md §3.5a, UC-FLW-02 check 10): a status per column, run for the tasks,
// each column showing its own, every entry taking its column's status; a dry run writes nothing.
func TestAWorkflowIsMadeFromABoardsColumns(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	administrator := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantA, AccountID: authorA}
	desk := schemeDesk(ctx, t)
	maker := workflowservice.MakeBoardWorkflow{Desk: desk}

	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	todo := seedRepairColumn(ctx, t, tenantA, collection, "a0", false)
	doing := seedRepairColumn(ctx, t, tenantA, collection, "a1", false)
	done := seedRepairColumn(ctx, t, tenantA, collection, "a2", true)
	inDoing := seedOnBoard(ctx, t, tenantA, authorA, collection, doing.ID, false, "")
	loose := seedOnBoard(ctx, t, tenantA, authorA, collection, "", true, "")

	dry, err := maker.Execute(ctx, administrator, collection, "", true)
	if err != nil {
		t.Fatalf("the dry run: %v", err)
	}
	if len(dry.Statuses) != 3 || dry.Entries != 2 || dry.Statuses[0].Status.Category != domain.CategoryTodo ||
		dry.Statuses[2].Status.Category != domain.CategoryDone || dry.Statuses[2].Count != 1 {
		t.Fatalf("dry run %+v", dry)
	}
	if mapping := readMapping(ctx, t, tenantA, collection); len(mapping) != 0 {
		t.Fatal("the dry run wrote a mapping")
	}

	made, err := maker.Execute(ctx, administrator, collection, "", false)
	if err != nil {
		t.Fatalf("making the workflow: %v", err)
	}
	mapping := readMapping(ctx, t, tenantA, collection)
	want := []domain.ColumnStatus{{BucketID: todo.ID, Status: "s1"}, {BucketID: doing.ID, Status: "s2"}, {BucketID: done.ID, Status: "s3"}}
	if !slices.Equal(mapping, want) {
		t.Errorf("mapping %v, want %v", mapping, want)
	}
	if entry := readEntry(ctx, t, tenantA, inDoing); entry.Status != "s2" || entry.BucketID != doing.ID {
		t.Errorf("the entry in Doing: %q in %s", entry.Status, entry.BucketID)
	}
	if entry := readEntry(ctx, t, tenantA, loose); entry.Status != "s3" || entry.BucketID != done.ID || !entry.Completion.IsCompleted {
		t.Errorf("the completed entry without a column: %q in %s", entry.Status, entry.BucketID)
	}
	runs := false
	err = read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		_, runs, err = desk.Gate.InForce(ctx, mustCollection(ctx, t, collection), "task")
		return err
	})
	if err != nil || !runs {
		t.Errorf("the workflow %s does not run for the tasks: %v", made.Key, err)
	}

	_, err = maker.Execute(ctx, administrator, collection, "", true)
	var coded *shared.Error
	if !errors.As(err, &coded) || coded.DetailCode != "board.workflow_runs" {
		t.Errorf("a second time: %v", err)
	}
}

func mustCollection(ctx context.Context, t *testing.T, id shared.ID) domain.Container {
	t.Helper()
	var found domain.Container
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		found, err = containerRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return found
}
