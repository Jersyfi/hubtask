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

	workflowservice "github.com/Jersyfi/hubtask/core/application/service/workflow"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A board whose entry types run their own workflows (domain-model.md §3.5a): its columns show the
// statuses of every workflow on it, a status key in one column at most. A change of the scheme for
// one type takes its columns - against a real database, another workspace untouched.

// perTypeBoard is a hub running one workflow for one type and another for a second, and a
// collection whose columns were made before the mapping: they name statuses but show none.
type perTypeBoard struct {
	hub, collection       shared.ID
	story, bug            string
	storyFlow, bugFlow    string
	todo, doing, done     domain.Bucket
	storyDoing, bugTriage shared.ID
}

func seedTypedWorkflow(ctx context.Context, t *testing.T, tenant, author shared.ID, shape domain.WorkflowShape) string {
	t.Helper()
	key := fmt.Sprintf("typed_%d", fixtureCounter.Add(1))
	flow, err := domain.NewWorkflow(domain.NewWorkflowInput{
		ID: freshID(t), TenantID: tenant, Key: key, Name: "Typed", Shape: shape, By: author, Now: created, Text: text.Composing{},
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

func seedTaskType(ctx context.Context, t *testing.T, tenant shared.ID) string {
	t.Helper()
	key := fmt.Sprintf("kind_%d", fixtureCounter.Add(1))
	kind, err := domain.NewEntryType(domain.NewEntryTypeInput{
		ID: freshID(t), TenantID: tenant, Key: key, Level: domain.ItemTask, Name: "Kind", Icon: "bug", ColorToken: "red",
		Behaviours: domain.EntryBehaviours{Completable: true}, Now: created, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.NewEntryTypeRepository().Insert(ctx, kind)
	}); err != nil {
		t.Fatalf("seeding the type: %v", err)
	}
	return key
}

func seedTyped(ctx context.Context, t *testing.T, tenant, author, collection shared.ID, entryType, status string) shared.ID {
	t.Helper()
	item := taskIn(tenant, author, collection, freshID(t), freshName(t), "a0")
	item.EntryType, item.Status = entryType, status
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		_, err := itemRepo().Insert(ctx, item)
		return err
	}); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}
	return item.ID
}

func seedNamedColumn(ctx context.Context, t *testing.T, tenant, collection shared.ID, name, orderKey string, done bool) domain.Bucket {
	t.Helper()
	column := bucketIn(tenant, collection, freshID(t), name, orderKey)
	column.IsDoneBucket = done
	if err := write(ctx, t, tenant, func(ctx context.Context) error { return bucketRepo().Insert(ctx, column) }); err != nil {
		t.Fatalf("seeding the column: %v", err)
	}
	return column
}

func seedPerTypeBoard(ctx context.Context, t *testing.T, tenant, author shared.ID) perTypeBoard {
	t.Helper()
	var b perTypeBoard
	b.hub, b.collection = hubWithCollection(ctx, t, tenant, author)
	b.story, b.bug = seedTaskType(ctx, t, tenant), seedTaskType(ctx, t, tenant)
	b.storyFlow = seedTypedWorkflow(ctx, t, tenant, author, domain.WorkflowShape{Initial: "todo", Statuses: []domain.WorkflowStatus{
		{Key: "todo", Name: "To do", Category: domain.CategoryTodo},
		{Key: "doing", Name: "Doing", Category: domain.CategoryInProgress},
		{Key: "done", Name: "Done", Category: domain.CategoryDone},
	}})
	b.bugFlow = seedTypedWorkflow(ctx, t, tenant, author, domain.WorkflowShape{Initial: "triage", Statuses: []domain.WorkflowStatus{
		{Key: "triage", Name: "Triage", Category: domain.CategoryTodo},
		{Key: "doing", Name: "Doing", Category: domain.CategoryInProgress},
		{Key: "done", Name: "Done", Category: domain.CategoryDone},
	}})
	b.todo = seedNamedColumn(ctx, t, tenant, b.collection, "To do", "a0", false)
	b.doing = seedNamedColumn(ctx, t, tenant, b.collection, "Doing", "a1", false)
	b.done = seedNamedColumn(ctx, t, tenant, b.collection, "Done", "a2", true)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		workflows := postgres.NewWorkflowRepository()
		if err := workflows.SetScheme(ctx, domain.WorkflowScheme{ContainerID: b.hub, EntryType: b.story, WorkflowKey: b.storyFlow}, created); err != nil {
			return err
		}
		return workflows.SetScheme(ctx, domain.WorkflowScheme{ContainerID: b.hub, EntryType: b.bug, WorkflowKey: b.bugFlow}, created)
	}); err != nil {
		t.Fatalf("seeding the scheme: %v", err)
	}
	b.storyDoing = seedTyped(ctx, t, tenant, author, b.collection, b.story, "doing")
	b.bugTriage = seedTyped(ctx, t, tenant, author, b.collection, b.bug, "triage")
	return b
}

func TestASchemeChangeForOneTypeTakesTheBoardsColumns(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	administrator := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantA, AccountID: authorA}
	desk := schemeDesk(ctx, t)
	desk.Boards.Types = postgres.NewEntryTypeRepository()

	board := seedPerTypeBoard(ctx, t, tenantA, authorA)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketStatusRepo().Replace(ctx, board.collection, []domain.ColumnStatus{
			{BucketID: board.todo.ID, Status: "todo"}, {BucketID: board.doing.ID, Status: "doing"},
			{BucketID: board.done.ID, Status: "done"}, {BucketID: board.todo.ID, Status: "triage"},
		})
	}); err != nil {
		t.Fatal(err)
	}
	// The second type now runs the first type's workflow in this collection: triage leaves the board.
	in := usecase.Input{
		"container_id": board.collection.String(), "entry_type": board.bug, "workflow": board.storyFlow,
		"columns": []any{
			map[string]any{"bucket_id": board.todo.ID.String(), "statuses": []any{"todo"}},
			map[string]any{"bucket_id": board.doing.ID.String(), "statuses": []any{"doing"}},
			map[string]any{"bucket_id": board.done.ID.String(), "statuses": []any{"done"}},
		},
	}

	t.Run("the preview reaches the board the type stands on", func(t *testing.T) {
		preview, err := workflowservice.PreviewWorkflowScheme{Desk: desk}.Descriptor().Handler.Invoke(ctx, administrator, in)
		if err != nil {
			t.Fatalf("the preview: %v", err)
		}
		if boards, _ := preview["boards"].([]usecase.Output); len(boards) != 1 || boards[0]["collection_id"] != board.collection.String() {
			t.Errorf("boards %v", preview["boards"])
		}
	})
	t.Run("the set takes the columns chosen", func(t *testing.T) {
		set := usecase.Input{"confirm": true}
		for k, v := range in {
			set[k] = v
		}
		if _, err := (workflowservice.SetWorkflowScheme{Desk: desk}).Descriptor().Handler.Invoke(ctx, administrator, set); err != nil {
			t.Fatalf("the set: %v", err)
		}
		mapping := readMapping(ctx, t, tenantA, board.collection)
		if len(mapping) != 3 || slices.Contains(mapping, domain.ColumnStatus{BucketID: board.todo.ID, Status: "triage"}) {
			t.Errorf("mapping %v", mapping)
		}
		if entry := readEntry(ctx, t, tenantA, board.bugTriage); entry.Status != "todo" || entry.BucketID != board.todo.ID {
			t.Errorf("the entry: %q in %s", entry.Status, entry.BucketID)
		}
	})
	t.Run("another workspace's collection is not found", func(t *testing.T) {
		stranger := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantB, AccountID: authorB}
		_, err := workflowservice.PreviewWorkflowScheme{Desk: desk}.Descriptor().Handler.Invoke(ctx, stranger, in)
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("answered %v", err)
		}
	})
}
