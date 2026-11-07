// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Workflows against the real boundary (UC-FLW-02 checks 1, 3, 4; UC-FLW-05 checks 2, 3, gate
// SG-3): a workspace stores a workflow with its draft, names it for a collection, counts and moves
// the entries by status - and the workspace next door neither reads, uses nor moves any of it.
func TestAWorkspacesWorkflowsAreItsOwn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	item := seedWorkItemForRules(ctx, t, tenantA)
	admin := adminPool(ctx, t)
	var collectionText string
	if err := admin.QueryRow(ctx, `SELECT collection_id::text FROM work_item WHERE id = $1`, item.String()).Scan(&collectionText); err != nil {
		t.Fatalf("reading the collection: %v", err)
	}
	collection := shared.MustParseID(collectionText)
	key := "wf_" + freshID(t).String()[24:]
	for _, statement := range []string{
		`DELETE FROM workflow_scheme WHERE container_id = '` + collection.String() + `'`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	workflows := postgres.NewWorkflowRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC().Truncate(time.Microsecond)
	created, err := domain.NewWorkflow(domain.NewWorkflowInput{
		ID: freshID(t), TenantID: tenantA, Key: key, Name: "Service",
		Shape: domain.WorkflowShape{
			Statuses: []domain.WorkflowStatus{
				{Key: "todo", Name: "To do", Category: domain.CategoryTodo},
				{Key: "waiting", Name: "Waiting", Category: domain.CategoryInProgress},
				{Key: "done", Name: "Done", Category: domain.CategoryDone},
			},
			Steps: []domain.WorkflowStep{{Key: "finish", To: "done", Requires: []string{"custom_fields.resolution"},
				Checks: []domain.StepCheck{{Key: "positive", Holds: "true", Message: "Above zero"}}}},
		},
		By: authorA, Now: now, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	drafted, err := created.WithDraft(domain.WorkflowShape{
		Statuses: []domain.WorkflowStatus{
			{Key: "todo", Name: "To do", Category: domain.CategoryTodo},
			{Key: "done", Name: "Done", Category: domain.CategoryDone},
		},
	}, now, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if err := workflows.Insert(ctx, created); err != nil {
			t.Fatalf("storing A's workflow: %v", err)
		}
		if err := workflows.Update(ctx, drafted, created.Version); err != nil {
			t.Fatalf("saving the draft: %v", err)
		}
		rows, err := workflows.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var read domain.Workflow
		for _, row := range rows {
			if row.Key == key {
				read = row
			}
		}
		if read.Draft == nil || len(read.Published.Statuses) != 3 || read.Published.Steps[0].Checks[0].Message != "Above zero" {
			t.Errorf("the shape and its draft came back as %+v", read)
		}
		if err := workflows.SetScheme(ctx, domain.WorkflowScheme{ContainerID: collection, WorkflowKey: key}, now); err != nil {
			t.Fatalf("using it in the collection: %v", err)
		}
		if err := workflows.Delete(ctx, key); !errors.Is(err, shared.ErrConflict) {
			t.Errorf("a workflow in use was deleted: %v", err)
		}
		groups, err := workflows.StatusGroups(ctx, []shared.ID{collection})
		if err != nil || len(groups) == 0 {
			t.Fatalf("counting: %v %v", groups, err)
		}
		moved, err := workflows.MoveStatuses(ctx, domain.StatusMove{
			CollectionID: collection, EntryType: "task", From: "", To: "waiting",
		}, now)
		if err != nil || len(moved) == 0 {
			t.Fatalf("moving: %v %v", moved, err)
		}
		return nil
	})

	// The workspace next door sees, uses and moves nothing of A's.
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		rows, err := workflows.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row.Key == key {
				t.Errorf("B listed A's workflow")
			}
		}
		schemes, _ := workflows.Schemes(ctx)
		for _, scheme := range schemes {
			if scheme.ContainerID == collection {
				t.Errorf("B read A's scheme")
			}
		}
		if err := workflows.Update(ctx, drafted, 2); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("B changed A's workflow: %v", err)
		}
		if moved, err := workflows.MoveStatuses(ctx, domain.StatusMove{
			CollectionID: collection, EntryType: "task", From: "waiting", To: "todo",
		}, now); err != nil || len(moved) != 0 {
			t.Errorf("B moved A's entries: %v %v", moved, err)
		}
		if groups, _ := workflows.StatusGroups(ctx, []shared.ID{collection}); len(groups) != 0 {
			t.Errorf("B counted A's entries: %v", groups)
		}
		return nil
	})

	var status string
	if err := admin.QueryRow(ctx, `SELECT coalesce(status, '') FROM work_item WHERE id = $1`, item.String()).Scan(&status); err != nil || status != "waiting" {
		t.Fatalf("the entry's status is %q: %v", status, err)
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if err := workflows.RemoveScheme(ctx, collection, ""); err != nil {
			t.Fatal(err)
		}
		if err := workflows.Delete(ctx, key); err != nil {
			t.Errorf("an unused workflow could not be deleted: %v", err)
		}
		return nil
	})
}
