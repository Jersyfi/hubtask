// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"

	activityrepo "github.com/Jersyfi/hubtask/core/application/repository/activity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/text"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// The board repair (domain-model.md §3.5a) against a real database: every case it repairs, a dry
// run that writes nothing, a second run that finds nothing, another workspace untouched, and a
// person who is not an administrator refused.

// onlyAdministrator lets one account through: the permission question itself is the application
// layer's and is tested there (RepairBoards_test.go).
type onlyAdministrator struct{ account shared.ID }

func (a onlyAdministrator) Authorize(_ context.Context, actor appshared.ActorContext, _ access.Request) error {
	if actor.AccountID != a.account {
		return shared.ErrForbidden
	}
	return nil
}

func boardRepair(ctx context.Context, t *testing.T) work.RepairBoards {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	fixed := portclock.Fixed(created)
	ids := clockadapter.NewUUIDv7(fixed)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	containers := containerRepo()
	boards := work.Boards{Buckets: bucketRepo(), Statuses: bucketStatusRepo()}
	return work.RepairBoards{
		Boards:   boards,
		Workflow: work.WorkflowGate{Workflows: postgres.NewWorkflowRepository()},
		Structure: work.StructureResolver{
			Containers: containers, Structures: postgres.NewStructureRepository(),
			Profiles: postgres.NewCapabilityProfileRepository(),
		},
		Columns: work.BucketWriter{
			Buckets: bucketRepo(), Containers: containers, Authorizer: onlyAdministrator{account: authorA},
			Events: postgres.NewOutbox(jobQueue(t)), Changes: postgres.NewChangeLog(),
			Audit: postgres.NewAuditSink(ids), UnitOfWork: unitOfWork, Clock: fixed, IDs: ids, HLC: hybrid,
			Text: textadapter.Forms{}, Boards: boards,
			Activity: work.ActivityJournal{Entries: historyRepo(), IDs: ids},
		},
	}
}

func seedRepairColumn(ctx context.Context, t *testing.T, tenant, collection shared.ID, orderKey string, done bool) domain.Bucket {
	t.Helper()
	column := bucketIn(tenant, collection, freshID(t), freshName(t), orderKey)
	column.IsDoneBucket = done
	if err := write(ctx, t, tenant, func(ctx context.Context) error { return bucketRepo().Insert(ctx, column) }); err != nil {
		t.Fatalf("seeding the column: %v", err)
	}
	return column
}

func seedOnBoard(ctx context.Context, t *testing.T, tenant, author, collection, column shared.ID, completed bool, status string) shared.ID {
	t.Helper()
	item := taskIn(tenant, author, collection, freshID(t), freshName(t), "a0")
	item.BucketID, item.Status = column, status
	if completed {
		item = item.Completed(author, created)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		if _, err := itemRepo().Insert(ctx, item); err != nil || !completed {
			return err
		}
		// The insert carries no completion; it is written as completing does.
		return itemRepo().SetCompletion(ctx, item, item.Version)
	}); err != nil {
		t.Fatalf("seeding the entry: %v", err)
	}
	return item.ID
}

func readEntry(ctx context.Context, t *testing.T, tenant, id shared.ID) domain.WorkItem {
	t.Helper()
	var item domain.WorkItem
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		item, err = itemRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the entry: %v", err)
	}
	return item
}

func liveColumns(ctx context.Context, t *testing.T, tenant, collection shared.ID) []domain.Bucket {
	t.Helper()
	var columns []domain.Bucket
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		columns, err = bucketRepo().List(ctx, collection)
		return err
	}); err != nil {
		t.Fatalf("reading the board: %v", err)
	}
	return columns
}

func TestTheBoardRepair(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	administrator := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantA, AccountID: authorA}
	repair := boardRepair(ctx, t)

	// L1: a board without a workflow, an entry completed outside the done column and an open one in it.
	_, columns := hubWithCollection(ctx, t, tenantA, authorA)
	todo := seedRepairColumn(ctx, t, tenantA, columns, "a0", false)
	seedRepairColumn(ctx, t, tenantA, columns, "a1", false)
	done := seedRepairColumn(ctx, t, tenantA, columns, "a2", true)
	completedOutside := seedOnBoard(ctx, t, tenantA, authorA, columns, todo.ID, true, "")
	openInDone := seedOnBoard(ctx, t, tenantA, authorA, columns, done.ID, false, "")

	// L2: a workflow of three statuses, the first and the last mapped, an entry in the unmapped one.
	_, mapped := hubWithCollection(ctx, t, tenantA, authorA)
	first := seedRepairColumn(ctx, t, tenantA, mapped, "a0", false)
	finished := seedRepairColumn(ctx, t, tenantA, mapped, "a2", true)
	key := fmt.Sprintf("repair_%d", fixtureCounter.Add(1))
	flow, err := domain.NewWorkflow(domain.NewWorkflowInput{
		ID: freshID(t), TenantID: tenantA, Key: key, Name: "Repair",
		Shape: domain.WorkflowShape{Statuses: []domain.WorkflowStatus{
			{Key: "todo", Name: "To do", Category: domain.CategoryTodo},
			{Key: "doing", Name: "Doing", Category: domain.CategoryInProgress},
			{Key: "done", Name: "Done", Category: domain.CategoryDone},
		}},
		By: authorA, Now: created, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		workflows := postgres.NewWorkflowRepository()
		if err := workflows.Insert(ctx, flow); err != nil {
			return err
		}
		if err := workflows.SetScheme(ctx, domain.WorkflowScheme{ContainerID: mapped, WorkflowKey: key}, created); err != nil {
			return err
		}
		return bucketStatusRepo().Replace(ctx, mapped, []domain.ColumnStatus{
			{BucketID: first.ID, Status: "todo"}, {BucketID: finished.ID, Status: "done"},
		})
	}); err != nil {
		t.Fatalf("seeding the workflow: %v", err)
	}
	doing := seedOnBoard(ctx, t, tenantA, authorA, mapped, "", false, "doing")

	// Starting columns nobody used, on a collection whose default view is not the board.
	_, unused := hubWithCollection(ctx, t, tenantA, authorA)
	seedRepairColumn(ctx, t, tenantA, unused, "a0", false)
	seedRepairColumn(ctx, t, tenantA, unused, "a1", false)
	seedRepairColumn(ctx, t, tenantA, unused, "a2", true)

	// Another workspace's board, as broken as the first.
	_, elsewhere := hubWithCollection(ctx, t, tenantB, authorB)
	foreignDone := seedRepairColumn(ctx, t, tenantB, elsewhere, "a0", true)
	foreign := seedOnBoard(ctx, t, tenantB, authorB, elsewhere, foreignDone.ID, false, "")

	t.Run("a person who is not an administrator is refused", func(t *testing.T) {
		someone := administrator
		someone.AccountID = authorB
		if _, err := repair.Execute(ctx, someone, false); !errors.Is(err, shared.ErrForbidden) {
			t.Fatalf("answered %v", err)
		}
	})

	dry, err := repair.Execute(ctx, administrator, true)
	if err != nil {
		t.Fatalf("the dry run: %v", err)
	}
	if !dry.DryRun || dry.MovedIntoDone < 1 || dry.Completed < 1 || dry.ColumnsAdded < 1 || dry.ColumnsRemoved < 3 {
		t.Fatalf("the dry run counted %+v", dry)
	}
	if entry := readEntry(ctx, t, tenantA, openInDone); entry.Completion.IsCompleted {
		t.Fatal("the dry run wrote")
	}

	report, err := repair.Execute(ctx, administrator, false)
	if err != nil {
		t.Fatalf("the repair: %v", err)
	}
	if report != (work.BoardRepair{
		Boards: dry.Boards, DoneMarked: dry.DoneMarked, MovedIntoDone: dry.MovedIntoDone, Completed: dry.Completed,
		ColumnsAdded: dry.ColumnsAdded, PlacedByStatus: dry.PlacedByStatus, ColumnsRemoved: dry.ColumnsRemoved,
		StatusesMapped: dry.StatusesMapped,
	}) {
		t.Errorf("the run did %+v, the dry run counted %+v", report, dry)
	}

	t.Run("L1: completed entries go into the done column", func(t *testing.T) {
		if entry := readEntry(ctx, t, tenantA, completedOutside); entry.BucketID != done.ID || !entry.Completion.IsCompleted {
			t.Errorf("in %s, done %v", entry.BucketID, entry.Completion.IsCompleted)
		}
	})
	t.Run("L1: open entries in the done column are completed", func(t *testing.T) {
		if entry := readEntry(ctx, t, tenantA, openInDone); entry.BucketID != done.ID || !entry.Completion.IsCompleted {
			t.Errorf("in %s, done %v", entry.BucketID, entry.Completion.IsCompleted)
		}
		// History stays truthful: completed by the administrator, because of the repair.
		history := historyOf(ctx, t, tenantA, openInDone, activityrepo.Page{Size: 10})
		found := false
		for _, entry := range history.Entries {
			cause, _ := entry.ChangeSet["cause"].(map[string]any)
			found = found || (entry.Verb == "item.completed" && entry.Actor.ID == authorA && cause["to"] == "BOARD_REPAIR")
		}
		if !found {
			t.Errorf("no completion in the history by the administrator for the repair: %+v", history.Entries)
		}
	})
	t.Run("L2: a column per unmapped status in the workflow's order, entries placed by status", func(t *testing.T) {
		board := liveColumns(ctx, t, tenantA, mapped)
		mapping := readMapping(ctx, t, tenantA, mapped)
		if len(board) != 3 || len(mapping) != 3 {
			t.Fatalf("%d columns, mapping %v", len(board), mapping)
		}
		if board[0].ID != first.ID || board[2].ID != finished.ID {
			t.Errorf("the added column does not stand between to do and done: %+v", board)
		}
		shows := false
		for _, row := range mapping {
			shows = shows || (row.BucketID == board[1].ID && row.Status == "doing")
		}
		entry := readEntry(ctx, t, tenantA, doing)
		if entry.BucketID != board[1].ID || !shows || board[1].WipLimit != nil {
			t.Errorf("the entry in doing stands in %s; board %+v, mapping %v", entry.BucketID, board, mapping)
		}
	})
	t.Run("unused starting columns are taken off", func(t *testing.T) {
		if board := liveColumns(ctx, t, tenantA, unused); len(board) != 0 {
			t.Errorf("%d columns left", len(board))
		}
	})
	t.Run("another workspace is untouched", func(t *testing.T) {
		if entry := readEntry(ctx, t, tenantB, foreign); entry.Completion.IsCompleted {
			t.Error("tenant B's entry was completed by tenant A's repair")
		}
	})
	t.Run("a second run changes nothing", func(t *testing.T) {
		again, err := repair.Execute(ctx, administrator, false)
		if err != nil {
			t.Fatal(err)
		}
		if again.DoneMarked+again.MovedIntoDone+again.Completed+again.ColumnsAdded+again.PlacedByStatus+again.ColumnsRemoved+again.StatusesMapped != 0 {
			t.Errorf("the second run did %+v", again)
		}
	})
}
