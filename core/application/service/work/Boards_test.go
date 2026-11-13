// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The doors that keep column and completion together on a board without a workflow, and the
// column following the status under one (domain-model.md §3.5a, UC-WRK-05 check 6).

var (
	todoColumn  = shared.MustParseID("0192f000-0000-7000-8000-0000000b0001")
	doingColumn = shared.MustParseID("0192f000-0000-7000-8000-0000000b0002")
	doneColumn  = shared.MustParseID("0192f000-0000-7000-8000-0000000b0003")
)

// threeColumnBoard puts Offen, In Arbeit and Erledigt on the collection's board.
func threeColumnBoard(store *buckets) {
	for i, column := range []domain.Bucket{
		{ID: todoColumn, Name: "Offen"},
		{ID: doingColumn, Name: "In Arbeit"},
		{ID: doneColumn, Name: "Erledigt", IsDoneBucket: true},
	} {
		column.TenantID, column.CollectionID, column.Version = tenantID, collectionID, 1
		column.OrderKey = string(rune('a'+i)) + "0"
		store.stored[column.ID] = column
	}
}

type mapping struct{ rows []domain.ColumnStatus }

func (m *mapping) List(context.Context, shared.ID) ([]domain.ColumnStatus, error) { return m.rows, nil }
func (m *mapping) Replace(_ context.Context, _ shared.ID, rows []domain.ColumnStatus) error {
	m.rows = rows
	return nil
}

func lastCompletion(t *testing.T, store *items) domain.WorkItem {
	t.Helper()
	if len(store.completions) == 0 {
		t.Fatal("no completion was written")
	}
	return store.completions[len(store.completions)-1].item
}

// Door: complete. Completing an entry on a board moves it into the done column.
func TestCompletingMovesIntoTheDoneColumn(t *testing.T) {
	h := newCompletionHarness(domain.CompletionManual)
	board := &buckets{stored: map[shared.ID]domain.Bucket{}}
	threeColumnBoard(board)
	h.writer.Boards = Boards{Buckets: board}
	task := h.items.stored[taskID]
	task.BucketID = doingColumn
	h.items.stored[taskID] = task

	if _, err := (CompleteWorkItem{Completion: h.writer}).Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: taskID}); err != nil {
		t.Fatalf("completing: %v", err)
	}
	if got := lastCompletion(t, h.items); got.BucketID != doneColumn || !got.Completion.IsCompleted {
		t.Errorf("completed into %s, done %v", got.BucketID, got.Completion.IsCompleted)
	}
}

// Door: reopen. Reopening an entry in the done column moves it into the first column.
func TestReopeningMovesOutOfTheDoneColumn(t *testing.T) {
	h := newCompletionHarness(domain.CompletionManual)
	board := &buckets{stored: map[shared.ID]domain.Bucket{}}
	threeColumnBoard(board)
	h.writer.Boards = Boards{Buckets: board}
	task := h.items.stored[taskID].Completed(accountID, now)
	task.BucketID = doneColumn
	h.items.stored[taskID] = task

	if _, err := (ReopenWorkItem{Completion: h.writer}).Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: taskID}); err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if got := lastCompletion(t, h.items); got.BucketID != todoColumn || got.Completion.IsCompleted {
		t.Errorf("reopened into %s, done %v", got.BucketID, got.Completion.IsCompleted)
	}
}

func coupledUpdateHarness() *updateHarness {
	h := newUpdateHarness()
	threeColumnBoard(h.buckets)
	h.handler.Boards = Boards{Buckets: h.buckets}
	h.handler.Completion = CompletionWriter{
		Items: h.items, Containers: h.containers, Profiles: h.handler.Profiles,
		Authorizer: h.authorizer, Events: h.events, Changes: h.changes, Audit: h.audit,
		Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
		UnitOfWork: h.uow, Clock: clock.Fixed(updateNow), IDs: &ids{}, HLC: &hlcSource{},
	}
	return h
}

// Door: the column move. Into the done column completes; out of it reopens.
func TestAColumnMoveCarriesCompletion(t *testing.T) {
	t.Run("into the done column", func(t *testing.T) {
		h := coupledUpdateHarness()
		target := doneColumn
		item, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
			ItemID: taskID, Attributes: domain.ItemAttributes{BucketID: &target},
		})
		if err != nil {
			t.Fatalf("moving: %v", err)
		}
		if item.BucketID != doneColumn || !item.Completion.IsCompleted {
			t.Errorf("moved into %s, done %v", item.BucketID, item.Completion.IsCompleted)
		}
	})
	t.Run("out of the done column", func(t *testing.T) {
		h := coupledUpdateHarness()
		task := h.items.stored[taskID].Completed(accountID, updateNow)
		task.BucketID = doneColumn
		h.items.stored[taskID] = task
		target := doingColumn
		item, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
			ItemID: taskID, Attributes: domain.ItemAttributes{BucketID: &target},
		})
		if err != nil {
			t.Fatalf("moving: %v", err)
		}
		if item.BucketID != doingColumn || item.Completion.IsCompleted {
			t.Errorf("moved into %s, done %v", item.BucketID, item.Completion.IsCompleted)
		}
	})
	t.Run("under a workflow the column is not set by hand", func(t *testing.T) {
		h := coupledUpdateHarness()
		task := h.items.stored[taskID]
		task.Status = "doing"
		h.items.stored[taskID] = task
		target := doneColumn
		_, err := h.handler.Execute(t.Context(), actorFixture(), UpdateCommand{
			ItemID: taskID, Attributes: domain.ItemAttributes{BucketID: &target},
		})
		if !errors.Is(err, shared.ErrConflict) {
			t.Errorf("a column set by hand under a workflow: %v", err)
		}
	})
}

// Door: create. An entry created into the done column is done.
func TestAnEntryCreatedIntoTheDoneColumnIsDone(t *testing.T) {
	h := newItemHarness()
	board := &buckets{stored: map[shared.ID]domain.Bucket{}}
	threeColumnBoard(board)
	h.handler.Buckets = board
	h.handler.Boards = Boards{Buckets: board}
	cmd := taskCommand()
	cmd.BucketID = doneColumn
	item, _, err := h.handler.Execute(t.Context(), itemActor(), cmd)
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if item.BucketID != doneColumn || !item.Completion.IsCompleted {
		t.Errorf("created into %s, done %v", item.BucketID, item.Completion.IsCompleted)
	}
	if !h.items.stored[item.ID].Completion.IsCompleted {
		t.Error("the completion was not written")
	}
}

// Door: a workflow step (TakeStep, a completion's step, the roll-up). The column follows the
// status - none for *Ohne Spalte*.
func TestUnderAWorkflowTheColumnFollowsTheStatus(t *testing.T) {
	board := &buckets{stored: map[shared.ID]domain.Bucket{}}
	threeColumnBoard(board)
	boards := Boards{Buckets: board, Statuses: &mapping{rows: []domain.ColumnStatus{
		{BucketID: todoColumn, Status: "open"}, {BucketID: doingColumn, Status: "doing"},
	}}}
	before := domain.WorkItem{Type: domain.ItemTask, CollectionID: collectionID, Status: "open", BucketID: todoColumn}
	after := before
	after.Status = "doing"
	got, err := boards.couple(t.Context(), before, after, accountID, now)
	if err != nil || got.BucketID != doingColumn {
		t.Errorf("stepped into %s: %v", got.BucketID, err)
	}
	after.Status = "review"
	got, err = boards.couple(t.Context(), before, after, accountID, now)
	if err != nil || !got.BucketID.IsZero() {
		t.Errorf("an unmapped status stands in %s: %v", got.BucketID, err)
	}
}
