// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
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

type mapping struct {
	rows     []domain.ColumnStatus
	placed   []placedByStatus
	cells    []domain.BoardCell
	statused []placedByStatus
	// completedIn are the columns whose open entries were completed.
	completedIn []shared.ID
}

// completedBy checks that a bulk completion wrote the entry's history: completed, by the actor,
// with the cause.
func completedBy(t *testing.T, history *journal, cause string) {
	t.Helper()
	for _, entry := range history.entries {
		if entry.Verb == activity.ItemCompleted && entry.ItemID == taskID && entry.Actor.ID == actor().AccountID {
			if got, _ := entry.ChangeSet["cause"].(map[string]any); got["to"] == cause {
				return
			}
		}
	}
	t.Errorf("no completion history with cause %s: %+v", cause, history.entries)
}

// unplaced marks a PlaceUnplaced call among the placements.
var unplaced = shared.MustParseID("0192f000-0000-7000-8000-0000000b00ff")

// placedByStatus is one PlaceByStatus call.
type placedByStatus struct {
	status    string
	column    shared.ID
	completed bool
	from      shared.ID
}

func (m *mapping) CompleteInColumn(_ context.Context, _ shared.ID, column shared.ID, _ shared.ID, _ time.Time) ([]repository.MovedEntry, error) {
	m.completedIn = append(m.completedIn, column)
	return []repository.MovedEntry{{ID: taskID, Version: 5}}, nil
}

func (m *mapping) PlaceUnplaced(_ context.Context, _ shared.ID, completed bool, column shared.ID, _ time.Time) ([]repository.MovedEntry, error) {
	m.placed = append(m.placed, placedByStatus{completed: completed, from: unplaced, column: column})
	return nil, nil
}

func (m *mapping) Cells(context.Context, shared.ID) ([]domain.BoardCell, error) { return m.cells, nil }

func (m *mapping) SetStatusInColumn(_ context.Context, _ shared.ID, column shared.ID, completed bool, status string, _ time.Time) ([]repository.MovedEntry, error) {
	m.statused = append(m.statused, placedByStatus{status: status, column: column, completed: completed})
	return []repository.MovedEntry{{ID: taskID, Version: 4}}, nil
}

func (m *mapping) PlaceByCompletion(_ context.Context, _ shared.ID, completed bool, from, column shared.ID, _ time.Time) ([]repository.MovedEntry, error) {
	m.placed = append(m.placed, placedByStatus{completed: completed, from: from, column: column})
	return []repository.MovedEntry{{ID: taskID, Version: 3}}, nil
}

func (m *mapping) PlaceByStatus(_ context.Context, _ shared.ID, status string, column shared.ID, _ time.Time) ([]repository.MovedEntry, error) {
	m.placed = append(m.placed, placedByStatus{status: status, column: column})
	return []repository.MovedEntry{{ID: taskID, Version: 3}}, nil
}

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

// boardElsewhere puts Offen, In Arbeit and Erledigt on the board of the harness's other collection
// and wires the placement writer to read boards and write completions.
func (h *placementHarness) boardElsewhere(collection shared.ID) *buckets {
	board := &buckets{stored: map[shared.ID]domain.Bucket{}}
	threeColumnBoard(board)
	for id, column := range board.stored {
		column.CollectionID = collection
		board.stored[id] = column
	}
	h.writer.Buckets = board
	h.writer.Completion = CompletionWriter{
		Boards: Boards{Buckets: board},
		Items:  h.items, Containers: h.containers, Profiles: h.writer.Profiles,
		Authorizer: h.authorizer, Events: h.events, Changes: h.changes, Audit: h.audit,
		Activity:   h.writer.Activity,
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	return board
}

// Door: a move into another collection. The entry takes the state of the column it is put in, and
// a completed one put in none goes into the done column.
func TestAnEntryMovedOntoAnotherBoardTakesTheStateOfItsColumn(t *testing.T) {
	move := func(t *testing.T, completed bool, column shared.ID) domain.WorkItem {
		t.Helper()
		h := newPlacementHarness()
		far := h.otherCollection()
		h.boardElsewhere(far.CollectionID)
		if completed {
			h.items.stored[targetTaskID] = h.items.stored[targetTaskID].Completed(accountID, now)
		}
		result, err := (MoveWorkItem{Placement: h.writer}).Execute(t.Context(), placementActor(), MoveWorkItemCommand{
			ItemID: targetTaskID, TargetCollectionID: far.CollectionID, ParentGiven: true,
			TargetBucketID: column, BucketGiven: !column.IsZero(),
		})
		if err != nil {
			t.Fatalf("moving: %v", err)
		}
		if stored := h.items.stored[targetTaskID]; stored.BucketID != result.Item.BucketID ||
			stored.Completion.IsCompleted != result.Item.Completion.IsCompleted {
			t.Errorf("answered %s/%v, stored %s/%v", result.Item.BucketID, result.Item.Completion.IsCompleted,
				stored.BucketID, stored.Completion.IsCompleted)
		}
		return result.Item
	}
	if got := move(t, false, doneColumn); got.BucketID != doneColumn || !got.Completion.IsCompleted {
		t.Errorf("into the done column: %s, done %v", got.BucketID, got.Completion.IsCompleted)
	}
	if got := move(t, true, doingColumn); got.BucketID != doingColumn || got.Completion.IsCompleted {
		t.Errorf("a completed entry into an open column: %s, done %v", got.BucketID, got.Completion.IsCompleted)
	}
	if got := move(t, true, ""); got.BucketID != doneColumn || !got.Completion.IsCompleted {
		t.Errorf("a completed entry into no column: %s, done %v", got.BucketID, got.Completion.IsCompleted)
	}
}

// Door: a copy (and an entry from a template, which is one). It arrives open, so out of the done
// column into the first.
func TestACopyArrivesOutsideTheDoneColumn(t *testing.T) {
	h := newDuplicateHarness(t)
	original := h.withTask()
	threeColumnBoard(h.buckets)
	h.handler.Boards = Boards{Buckets: h.buckets}
	completed := original.Completed(accountID, now)
	completed.BucketID = doneColumn
	h.items.stored[original.ID] = completed

	result, err := h.handler.Execute(t.Context(), actor(), DuplicateWorkItemCommand{ItemID: original.ID})
	if err != nil {
		t.Fatalf("copying: %v", err)
	}
	if result.Item.BucketID != todoColumn || result.Item.Completion.IsCompleted {
		t.Errorf("the copy stands in %s, done %v", result.Item.BucketID, result.Item.Completion.IsCompleted)
	}
}

// Door: a publish or a change of scheme moves statuses; the column follows each moved status.
func TestAStatusMovedByASchemeChangeTakesItsColumn(t *testing.T) {
	board := &buckets{stored: map[shared.ID]domain.Bucket{}}
	threeColumnBoard(board)
	statuses := &mapping{rows: []domain.ColumnStatus{{BucketID: doingColumn, Status: "doing"}}}
	recorded := &changes{}
	mover := StatusMover{
		Workflows: &workflowStore{}, Activity: ActivityJournal{Entries: &journal{}, IDs: &ids{}},
		Changes: recorded, HLC: &hlcSource{}, Boards: Boards{Buckets: board, Statuses: statuses},
	}
	if _, err := mover.Move(t.Context(), actorFixture(), []domain.StatusMove{
		{CollectionID: collectionID, EntryType: "task", From: "open", To: "doing"},
		{CollectionID: collectionID, EntryType: "task", From: "old", To: "review"},
	}, now); err != nil {
		t.Fatalf("moving: %v", err)
	}
	want := []placedByStatus{{status: "doing", column: doingColumn}, {status: "review"}}
	if len(statuses.placed) != 2 || statuses.placed[0] != want[0] || statuses.placed[1] != want[1] {
		t.Errorf("placed %+v, want %+v", statuses.placed, want)
	}
	columns := 0
	for _, change := range recorded.recorded {
		if change.Field == domain.FieldBucketID {
			columns++
		}
	}
	if columns != 2 {
		t.Errorf("%d column changes recorded for offline clients, want 2", columns)
	}
}

// Door: an import. Its rows carry their completion; the column follows - completed ones into the
// done column, open ones out of it into the first.
func TestAnImportsCompletionDecidesTheColumn(t *testing.T) {
	board := &buckets{stored: map[shared.ID]domain.Bucket{}}
	threeColumnBoard(board)
	statuses := &mapping{}
	settler := StatusSettler{Statuses: StatusMover{
		Workflows: &workflowStore{}, Activity: ActivityJournal{Entries: &journal{}, IDs: &ids{}},
		Changes: &changes{}, HLC: &hlcSource{}, Boards: Boards{Buckets: board, Statuses: statuses},
	}}
	if err := settler.Settle(t.Context(), actorFixture(), domain.Container{ID: collectionID}, now); err != nil {
		t.Fatalf("settling: %v", err)
	}
	want := []placedByStatus{
		{completed: true, column: doneColumn},
		{completed: false, from: doneColumn, column: todoColumn},
	}
	if len(statuses.placed) != 2 || statuses.placed[0] != want[0] || statuses.placed[1] != want[1] {
		t.Errorf("placed %+v, want %+v", statuses.placed, want)
	}
}

// Door: a column's done mark. Marking a column done takes the mark off the other; clearing the
// only one is refused.
func TestOneDoneColumnPerBoard(t *testing.T) {
	marked := true
	h := newWriterHarness()
	threeColumnBoard(h.buckets)
	board := &mapping{}
	history := &journal{}
	h.update.Writer.Boards = Boards{Buckets: h.buckets, Statuses: board}
	h.update.Writer.Activity = ActivityJournal{Entries: history, IDs: &ids{}}
	if _, err := h.update.Execute(t.Context(), actor(), UpdateBucketCommand{
		BucketID: doingColumn, Attributes: domain.BucketAttributes{IsDoneBucket: &marked},
	}); err != nil {
		t.Fatalf("marking: %v", err)
	}
	if h.buckets.stored[doneColumn].IsDoneBucket || !h.buckets.stored[doingColumn].IsDoneBucket {
		t.Errorf("done marks: Erledigt %v, In Arbeit %v", h.buckets.stored[doneColumn].IsDoneBucket,
			h.buckets.stored[doingColumn].IsDoneBucket)
	}
	unmarked := false
	for _, write := range h.buckets.written {
		unmarked = unmarked || (write.bucket.ID == doneColumn && write.expected == 1)
	}
	if !unmarked {
		t.Error("the mark was taken off without a write against its version")
	}
	// Done column and completion at once: the old done column's entries go with the mark, and the
	// open ones in the new done column are completed, each with its history.
	if len(board.placed) != 1 || board.placed[0] != (placedByStatus{completed: true, from: doneColumn, column: doingColumn}) {
		t.Errorf("placed %+v, want the completed entries of Erledigt into In Arbeit", board.placed)
	}
	if len(board.completedIn) != 1 || board.completedIn[0] != doingColumn {
		t.Errorf("completed in %v, want In Arbeit", board.completedIn)
	}
	completedBy(t, history, causeDoneMoved)

	cleared := false
	_, err := h.update.Execute(t.Context(), actor(), UpdateBucketCommand{
		BucketID: doingColumn, Attributes: domain.BucketAttributes{IsDoneBucket: &cleared},
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("clearing the only done column: %v", err)
	}
}

// Door: deleting a column. The done column names its successor, which its entries go into; under a
// mapped workflow the entries go by status, so into *Ohne Spalte*.
func TestDeletingAColumnKeepsTheBoardCoupled(t *testing.T) {
	t.Run("the done column asks which column means done instead", func(t *testing.T) {
		h, deleter := newDeleteHarness()
		threeColumnBoard(h.buckets)
		deleter.Writer.Boards = Boards{Buckets: h.buckets}
		_, err := deleter.Execute(t.Context(), actor(), DeleteBucketCommand{BucketID: doneColumn})
		var coded *shared.Error
		if !errors.As(err, &coded) || coded.DetailCode != "buckets.done_successor_required" ||
			coded.Params["suggested_bucket_id"] != doingColumn.String() {
			t.Fatalf("deleting the done column without a successor: %v", err)
		}
		if h.buckets.stored[doneColumn].DeletedAt != nil {
			t.Error("the refused deletion took the column off")
		}
	})
	t.Run("the done column with its successor", func(t *testing.T) {
		h, deleter := newDeleteHarness()
		threeColumnBoard(h.buckets)
		board := &mapping{}
		history := &journal{}
		deleter.Writer.Boards = Boards{Buckets: h.buckets, Statuses: board}
		deleter.Writer.Activity = ActivityJournal{Entries: history, IDs: &ids{}}
		deletion, err := deleter.Execute(t.Context(), actor(), DeleteBucketCommand{BucketID: doneColumn, NewDoneBucketID: todoColumn})
		if err != nil {
			t.Fatalf("deleting: %v", err)
		}
		if deletion.DoneBucketID != todoColumn || deletion.TargetBucketID != todoColumn {
			t.Errorf("successor %s, entries into %s", deletion.DoneBucketID, deletion.TargetBucketID)
		}
		if !h.buckets.stored[todoColumn].IsDoneBucket {
			t.Error("the successor carries no done mark")
		}
		if len(board.completedIn) != 1 || board.completedIn[0] != todoColumn {
			t.Errorf("completed in %v, want the successor's open entries", board.completedIn)
		}
		completedBy(t, history, causeDoneMoved)
	})
	t.Run("an open column passes over the done column", func(t *testing.T) {
		h, deleter := newDeleteHarness()
		threeColumnBoard(h.buckets)
		deleter.Writer.Boards = Boards{Buckets: h.buckets}
		deletion, err := deleter.Execute(t.Context(), actor(), DeleteBucketCommand{BucketID: todoColumn})
		if err != nil || deletion.TargetBucketID != doingColumn || !deletion.DoneBucketID.IsZero() {
			t.Errorf("deleted %+v: %v", deletion, err)
		}
	})
	t.Run("under a mapped workflow", func(t *testing.T) {
		h, deleter := newDeleteHarness()
		threeColumnBoard(h.buckets)
		deleter.Writer.Boards = Boards{Buckets: h.buckets, Statuses: &mapping{rows: []domain.ColumnStatus{
			{BucketID: todoColumn, Status: "open"},
		}}}
		deletion, err := deleter.Execute(t.Context(), actor(), DeleteBucketCommand{BucketID: todoColumn})
		if err != nil || !deletion.TargetBucketID.IsZero() {
			t.Errorf("deleted %+v: %v", deletion, err)
		}
	})
}

// Without a workflow the steps answer says the level: at L1 the columns are the entry's states,
// the done column completing, and the column it reads as; at L0 none (UC-FLW-06 check 5).
func TestTheStepsAnswerTheColumnsWithoutAWorkflow(t *testing.T) {
	h := newCompletionHarness(domain.CompletionManual)
	steps := ListEntrySteps{Completion: h.writer}
	answer, err := steps.Execute(t.Context(), actorFixture(), taskID)
	if err != nil || answer.StateMode != domain.StateModeNone || len(answer.Columns) != 0 {
		t.Fatalf("a list answered %s with %d columns: %v", answer.StateMode, len(answer.Columns), err)
	}
	board := &buckets{stored: map[shared.ID]domain.Bucket{}}
	threeColumnBoard(board)
	h.writer.Boards = Boards{Buckets: board}
	steps = ListEntrySteps{Completion: h.writer}
	answer, err = steps.Execute(t.Context(), actorFixture(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	if answer.StateMode != domain.StateModeColumns || len(answer.Columns) != 3 || !answer.Columns[2].IsDoneBucket ||
		answer.Column != todoColumn {
		t.Errorf("a board answered %s, columns %+v, standing in %s", answer.StateMode, answer.Columns, answer.Column)
	}
}
