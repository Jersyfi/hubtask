// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A list becoming a board and a board a list again (L0 ↔ L1, domain-model.md §3.5a) on a real
// database: the preview counts what the switch writes, the board starts with an open and a done
// column holding the entries by completion, the list keeps completion and takes the columns off,
// a workflow refuses both, and another workspace's collection is never reached.
func TestAListBecomesABoardAndBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	administrator := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantA, AccountID: authorA}
	columns := boardColumns(ctx, t)
	switcher := work.SwitchBoard{Columns: columns, Workflow: work.WorkflowGate{Workflows: postgres.NewWorkflowRepository()}}

	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	open := seedOnBoard(ctx, t, tenantA, authorA, collection, "", false, "")
	done := seedOnBoard(ctx, t, tenantA, authorA, collection, "", true, "")

	dry, err := switcher.Execute(ctx, administrator, collection, domain.StateModeColumns, true)
	if err != nil {
		t.Fatalf("the preview: %v", err)
	}
	if dry.Before != domain.StateModeNone || dry.After != domain.StateModeColumns || dry.Created != 2 || dry.Moved != 2 {
		t.Errorf("preview %+v", dry)
	}
	if len(liveColumns(ctx, t, tenantA, collection)) != 0 {
		t.Fatal("the preview created columns")
	}

	report, err := switcher.Execute(ctx, administrator, collection, domain.StateModeColumns, false)
	if err != nil {
		t.Fatalf("to a board: %v", err)
	}
	report.DryRun = true
	if report != dry {
		t.Errorf("the switch did %+v, the preview said %+v", report, dry)
	}
	board := liveColumns(ctx, t, tenantA, collection)
	if len(board) != 2 || board[0].IsDoneBucket || !board[1].IsDoneBucket {
		t.Fatalf("columns %+v, want open then done", board)
	}
	if entry := readEntry(ctx, t, tenantA, open); entry.BucketID != board[0].ID || entry.Completion.IsCompleted {
		t.Errorf("the open entry stands in %s", entry.BucketID)
	}
	if entry := readEntry(ctx, t, tenantA, done); entry.BucketID != board[1].ID || !entry.Completion.IsCompleted {
		t.Errorf("the completed entry stands in %s", entry.BucketID)
	}

	if _, err := switcher.Execute(ctx, administrator, collection, domain.StateModeColumns, false); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a board made a board again: %v", err)
	}

	back, err := switcher.Execute(ctx, administrator, collection, domain.StateModeNone, false)
	if err != nil || back.Removed != 2 || back.Moved != 2 {
		t.Fatalf("back to a list: %+v, %v", back, err)
	}
	if len(liveColumns(ctx, t, tenantA, collection)) != 0 {
		t.Error("the columns stayed")
	}
	for id, completed := range map[shared.ID]bool{open: false, done: true} {
		if entry := readEntry(ctx, t, tenantA, id); !entry.BucketID.IsZero() || entry.Completion.IsCompleted != completed {
			t.Errorf("entry %s in %s, done %v", id, entry.BucketID, entry.Completion.IsCompleted)
		}
	}

	t.Run("a workflow refuses the switch", func(t *testing.T) {
		key := seedSchemeWorkflow(ctx, t, tenantA, authorA)
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			return postgres.NewWorkflowRepository().SetScheme(ctx, domain.WorkflowScheme{ContainerID: collection, WorkflowKey: key}, created)
		}); err != nil {
			t.Fatal(err)
		}
		_, err := switcher.Execute(ctx, administrator, collection, domain.StateModeColumns, true)
		var coded *shared.Error
		if !errors.As(err, &coded) || coded.DetailCode != "board.workflow_runs" {
			t.Errorf("answered %v", err)
		}
	})

	t.Run("another workspace's collection is not found", func(t *testing.T) {
		stranger := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantB, AccountID: authorA}
		if _, err := switcher.Execute(ctx, stranger, collection, domain.StateModeColumns, true); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("answered %v", err)
		}
	})
}
