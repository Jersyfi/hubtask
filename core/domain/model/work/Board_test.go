// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	colTodo  = shared.MustParseID("0192f000-0000-7000-8000-00000000c001")
	colDoing = shared.MustParseID("0192f000-0000-7000-8000-00000000c002")
	colDone  = shared.MustParseID("0192f000-0000-7000-8000-00000000c003")
	boardBy  = shared.MustParseID("0192f000-0000-7000-8000-00000000c0aa")
	boardAt  = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
)

func threeColumns() Board {
	return Board{Columns: []Bucket{
		{ID: colTodo, Name: "Offen"},
		{ID: colDoing, Name: "In Arbeit"},
		{ID: colDone, Name: "Erledigt", IsDoneBucket: true},
	}}
}

func reviewShape() WorkflowShape {
	return WorkflowShape{
		Statuses: []WorkflowStatus{
			{Key: "open", Name: "Offen", Category: CategoryTodo},
			{Key: "doing", Name: "Doing", Category: CategoryInProgress},
			{Key: "review", Name: "Review", Category: CategoryInProgress},
			{Key: "done", Name: "Fertig", Category: CategoryDone},
			{Key: "dropped", Name: "Verworfen", Category: CategoryDone},
		},
		Initial: "open",
		Steps: []WorkflowStep{
			{Key: "start", From: []string{"open"}, To: "doing"},
			{Key: "ask", From: []string{"doing"}, To: "review"},
			{Key: "finish", From: []string{"review"}, To: "done"},
			{Key: "drop", To: "dropped"},
			{Key: "close", From: []string{"doing"}, To: "done"},
		},
	}
}

func TestStateModeIsDerived(t *testing.T) {
	cases := []struct {
		name     string
		level    ItemType
		workflow bool
		columns  int
		want     StateMode
	}{
		{"a list", ItemTask, false, 0, StateModeNone},
		{"columns make a board", ItemTask, false, 3, StateModeColumns},
		{"a workflow wins over columns", ItemTask, true, 3, StateModeWorkflow},
		{"a work package never stands in a column", ItemWorkPackage, false, 3, StateModeNone},
		{"a work package under a workflow", ItemWorkPackage, true, 0, StateModeWorkflow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := StateModeOf(c.level, c.workflow, c.columns); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}

func TestAnEntryWithoutAColumnReadsByItsCompletion(t *testing.T) {
	board := threeColumns()
	open := WorkItem{Type: ItemTask}
	done := open.Completed(boardBy, boardAt)
	if got := board.ColumnOfEntry(open); got != colTodo {
		t.Errorf("an open entry reads as %s, want the first column", got)
	}
	if got := board.ColumnOfEntry(done); got != colDone {
		t.Errorf("a completed entry reads as %s, want the done column", got)
	}
	placed := WorkItem{Type: ItemTask, BucketID: colDoing}
	if got := board.ColumnOfEntry(placed); got != colDoing {
		t.Errorf("an entry in a column reads as %s", got)
	}
}

func TestABoardHasExactlyOneDoneColumn(t *testing.T) {
	if err := threeColumns().EnsureOneDone(); err != nil {
		t.Errorf("one done column refused: %v", err)
	}
	none := Board{Columns: []Bucket{{ID: colTodo}, {ID: colDoing}}}
	if err := none.EnsureOneDone(); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a board without a done column passed: %v", err)
	}
	two := Board{Columns: []Bucket{{ID: colTodo, IsDoneBucket: true}, {ID: colDone, IsDoneBucket: true}}}
	if err := two.EnsureOneDone(); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a board with two done columns passed: %v", err)
	}
	if err := (Board{}).EnsureOneDone(); err != nil {
		t.Errorf("a list has no done column to keep: %v", err)
	}
}

func TestColumnAndCompletionAreCoupled(t *testing.T) {
	board := threeColumns()
	openIn := func(column shared.ID) WorkItem { return WorkItem{Type: ItemTask, BucketID: column} }
	doneIn := func(column shared.ID) WorkItem { return openIn(column).Completed(boardBy, boardAt) }
	move := func(item WorkItem, column shared.ID) WorkItem { item.BucketID = column; return item }

	cases := []struct {
		name          string
		before, after WorkItem
		wantColumn    shared.ID
		wantDone      bool
	}{
		{"entering the done column completes", openIn(colDoing), move(openIn(colDoing), colDone), colDone, true},
		{"leaving the done column reopens", doneIn(colDone), move(doneIn(colDone), colTodo), colTodo, false},
		{"moving between open columns stays open", openIn(colTodo), move(openIn(colTodo), colDoing), colDoing, false},
		{"completing moves into the done column", openIn(colDoing), openIn(colDoing).Completed(boardBy, boardAt), colDone, true},
		{"completing without a column moves into the done column", openIn(""), openIn("").Completed(boardBy, boardAt), colDone, true},
		{"reopening moves out of the done column", doneIn(colDone), doneIn(colDone).Reopened(boardAt), colTodo, false},
		{"reopening outside the done column stays", doneIn(colDoing), doneIn(colDoing).Reopened(boardAt), colDoing, false},
		{"created in the done column is completed", WorkItem{Type: ItemTask}, openIn(colDone), colDone, true},
		{"nothing changed, nothing coupled", openIn(colDoing), openIn(colDoing), colDoing, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := board.Coupled(c.before, c.after, boardBy, boardAt)
			if got.BucketID != c.wantColumn || got.Completion.IsCompleted != c.wantDone {
				t.Errorf("got column %s done %v, want %s %v", got.BucketID, got.Completion.IsCompleted, c.wantColumn, c.wantDone)
			}
		})
	}

	t.Run("a list couples nothing", func(t *testing.T) {
		got := Board{}.Coupled(openIn(""), openIn("").Completed(boardBy, boardAt), boardBy, boardAt)
		if !got.BucketID.IsZero() {
			t.Errorf("a list gained a column: %s", got.BucketID)
		}
	})
	t.Run("a work package is never coupled", func(t *testing.T) {
		pack := WorkItem{Type: ItemWorkPackage}
		got := board.Coupled(pack, pack.Completed(boardBy, boardAt), boardBy, boardAt)
		if !got.BucketID.IsZero() {
			t.Errorf("a work package gained a column: %s", got.BucketID)
		}
	})
}

func TestAStatusSitsInOneColumnAndUnmappedOnesFallOutside(t *testing.T) {
	board := threeColumns()
	board.Mapping = []ColumnStatus{
		{BucketID: colTodo, Status: "open"}, {BucketID: colDoing, Status: "doing"},
		{BucketID: colDoing, Status: "review"}, {BucketID: colDone, Status: "done"},
	}
	shape := reviewShape()
	if got := board.StatusesOf(colDoing); !slices.Equal(got, []string{"doing", "review"}) {
		t.Errorf("the column shows %v", got)
	}
	if column, found := board.ColumnOfStatus("review"); !found || column != colDoing {
		t.Errorf("review sits in %s %v", column, found)
	}
	if _, found := board.ColumnOfStatus("dropped"); found {
		t.Error("an unmapped status found a column")
	}
	if got := board.Unmapped(shape); !slices.Equal(got, []string{"dropped"}) {
		t.Errorf("unmapped: %v", got)
	}

	if _, err := NewBoardMapping(board.Columns, board.Mapping, shape); err != nil {
		t.Errorf("a valid mapping refused: %v", err)
	}
	twice := append(slices.Clone(board.Mapping), ColumnStatus{BucketID: colDone, Status: "review"})
	if _, err := NewBoardMapping(board.Columns, twice, shape); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a status in two columns passed: %v", err)
	}
	unknown := []ColumnStatus{{BucketID: colTodo, Status: "nowhere"}}
	if _, err := NewBoardMapping(board.Columns, unknown, shape); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an unknown status passed: %v", err)
	}
	foreign := []ColumnStatus{{BucketID: boardBy, Status: "open"}}
	if _, err := NewBoardMapping(board.Columns, foreign, shape); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a column of another board passed: %v", err)
	}
}

func TestAColumnIsDoneOnlyWhenEveryStatusIsDone(t *testing.T) {
	shape := reviewShape()
	cases := []struct {
		statuses []string
		want     bool
	}{
		{[]string{"done"}, true},
		{[]string{"done", "dropped"}, true},
		{[]string{"done", "review"}, false},
		{nil, false},
	}
	for _, c := range cases {
		if got := ColumnCompletes(c.statuses, shape); got != c.want {
			t.Errorf("%v: got %v, want %v", c.statuses, got, c.want)
		}
	}
}

func TestTheDefaultMapping(t *testing.T) {
	shape := reviewShape()
	mapping, suggestions := DefaultMapping(threeColumns().Columns, shape)
	want := []ColumnStatus{
		{BucketID: colTodo, Status: "open"},    // same name
		{BucketID: colDone, Status: "done"},    // DONE into the done column
		{BucketID: colDone, Status: "dropped"}, // every DONE
		{BucketID: colDoing, Status: "doing"},  // the first IN_PROGRESS
	}
	if !slices.Equal(mapping, want) {
		t.Errorf("mapping %v, want %v", mapping, want)
	}
	if len(suggestions) != 1 || suggestions[0].Status != "review" || suggestions[0].BucketID != colTodo {
		t.Errorf("suggestions %v", suggestions)
	}

	t.Run("the first column takes the initial status", func(t *testing.T) {
		columns := []Bucket{{ID: colTodo, Name: "Eingang"}, {ID: colDone, Name: "Zu", IsDoneBucket: true}}
		mapping, _ := DefaultMapping(columns, shape)
		if !slices.Contains(mapping, ColumnStatus{BucketID: colTodo, Status: "open"}) {
			t.Errorf("the initial status is not in the first column: %v", mapping)
		}
	})
}

func TestADropResolvesToOneStepSeveralOrNone(t *testing.T) {
	shape := reviewShape()
	board := threeColumns()
	board.Mapping = []ColumnStatus{
		{BucketID: colTodo, Status: "open"}, {BucketID: colDoing, Status: "doing"},
		{BucketID: colDone, Status: "done"}, {BucketID: colDone, Status: "dropped"},
	}
	at := func(status string) WorkItem { return WorkItem{Type: ItemTask, Status: status} }
	stepKeys := func(plan DropPlan) []string {
		var keys []string
		for _, step := range plan.Steps {
			keys = append(keys, step.Key)
		}
		return keys
	}

	cases := []struct {
		name    string
		mode    StateMode
		item    WorkItem
		target  shared.ID
		offered []WorkflowStep
		want    DropOutcome
		steps   []string
		reason  string
	}{
		{"a board without workflow moves", StateModeColumns, WorkItem{Type: ItemTask}, colDone, nil, DropMoved, nil, ""},
		{"a list has no columns", StateModeNone, WorkItem{Type: ItemTask}, colDone, nil, DropRefused, nil, "board.no_columns"},
		{"one step is taken", StateModeWorkflow, at("open"), colDoing, shape.Steps, DropStep, []string{"start"}, ""},
		{"several steps are offered", StateModeWorkflow, at("doing"), colDone, shape.Steps, DropChoose, []string{"drop", "close"}, ""},
		{"the column it stands in moves nothing", StateModeWorkflow, at("doing"), colDoing, shape.Steps, DropMoved, nil, ""},
		{"no step leads there", StateModeWorkflow, at("open"), colDone, shape.Steps[:1], DropRefused, nil, "board.step_blocked"},
		{"no step exists", StateModeWorkflow, at("review"), colTodo, shape.Steps, DropRefused, nil, "board.no_step"},
		{"Ohne Spalte takes the unmapped statuses", StateModeWorkflow, at("doing"), "", shape.Steps, DropStep, []string{"ask"}, ""},
		{"a column without status refuses", StateModeWorkflow, at("open"), boardBy, shape.Steps, DropRefused, nil, "board.column_without_status"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan := board.PlanDrop(c.mode, c.item, c.target, shape, c.offered)
			if plan.Outcome != c.want || !slices.Equal(stepKeys(plan), c.steps) || plan.Reason != c.reason {
				t.Errorf("got %s %v %q, want %s %v %q", plan.Outcome, stepKeys(plan), plan.Reason, c.want, c.steps, c.reason)
			}
		})
	}
}

func TestAnEntryArrivingOnABoardTakesTheStateOfItsColumn(t *testing.T) {
	board := threeColumns()
	board.Mapping = []ColumnStatus{{BucketID: colDoing, Status: "doing"}}
	open := func(column shared.ID) WorkItem { return WorkItem{Type: ItemTask, BucketID: column} }
	done := func(column shared.ID) WorkItem { return open(column).Completed(boardBy, boardAt) }
	withStatus := func(status string) WorkItem { item := open(colTodo); item.Status = status; return item }

	cases := []struct {
		name       string
		item       WorkItem
		wantColumn shared.ID
		wantDone   bool
	}{
		{"put into the done column it completes", open(colDone), colDone, true},
		{"put into an open column it reopens", done(colDoing), colDoing, false},
		{"completed without a column it goes into the done column", done(""), colDone, true},
		{"open without a column it stays without", open(""), "", false},
		{"a column of another board counts as none", done(boardBy), colDone, true},
		{"under a workflow the status decides the column", withStatus("doing"), colDoing, false},
		{"an unmapped status stands in no column", withStatus("review"), "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := board.Arrived(c.item, boardBy, boardAt)
			if got.BucketID != c.wantColumn || got.Completion.IsCompleted != c.wantDone {
				t.Errorf("arrived in %s, done %v; want %s, %v", got.BucketID, got.Completion.IsCompleted, c.wantColumn, c.wantDone)
			}
		})
	}
	if got := (Board{}).Arrived(done(colDone), boardBy, boardAt); got.BucketID != colDone {
		t.Error("a collection without a board changed the entry")
	}
}

func TestACopyArrivesOpenOutsideTheDoneColumn(t *testing.T) {
	board := threeColumns()
	board.Mapping = []ColumnStatus{{BucketID: colTodo, Status: "open"}}
	cases := []struct {
		name string
		item WorkItem
		want shared.ID
	}{
		{"from the done column into the first", WorkItem{Type: ItemTask, BucketID: colDone}, colTodo},
		{"an open column is kept", WorkItem{Type: ItemTask, BucketID: colDoing}, colDoing},
		{"under a workflow by status", WorkItem{Type: ItemTask, BucketID: colDone, Status: "open"}, colTodo},
		{"an unmapped status in no column", WorkItem{Type: ItemTask, BucketID: colDone, Status: "doing"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := board.Copied(c.item); got.BucketID != c.want || got.Completion.IsCompleted {
				t.Errorf("copied into %s, done %v; want %s", got.BucketID, got.Completion.IsCompleted, c.want)
			}
		})
	}
}

func TestOneDoneColumnWhenTheMarkMoves(t *testing.T) {
	board := threeColumns()
	changed, err := board.MarkDone(colDoing, true)
	if err != nil {
		t.Fatalf("marking: %v", err)
	}
	if len(changed) != 1 || changed[0].ID != colDone || changed[0].IsDoneBucket {
		t.Errorf("the other done column kept its mark: %+v", changed)
	}
	if changed, err := board.MarkDone(colDone, true); err != nil || len(changed) != 0 {
		t.Errorf("marking the done column again: %+v, %v", changed, err)
	}
	if _, err := board.MarkDone(colDone, false); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("clearing the only done column: %v", err)
	}
	if changed, err := board.MarkDone(colTodo, false); err != nil || len(changed) != 0 {
		t.Errorf("clearing a column that has no mark: %+v, %v", changed, err)
	}
}

func TestDeletingAColumnPlacesItsEntries(t *testing.T) {
	board := threeColumns()
	if successor, found := board.DoneSuccessor(colDone); !found || successor.ID != colDoing {
		t.Errorf("the done column's successor is %s, want the rightmost column left", successor.ID)
	}
	if _, found := board.DoneSuccessor(colTodo); found {
		t.Error("deleting an open column named a done successor")
	}
	if target := board.TargetAfterDeleting(colTodo); target != colDoing {
		t.Errorf("an open column's entries go to %s, want the first open column left", target)
	}
	if target := board.TargetAfterDeleting(colDone); target != colDoing {
		t.Errorf("the done column's entries go to %s, want its successor", target)
	}
	board.Mapping = []ColumnStatus{{BucketID: colTodo, Status: "open"}}
	if target := board.TargetAfterDeleting(colTodo); !target.IsZero() {
		t.Errorf("under a mapped workflow the entries go to %s, want *Ohne Spalte*", target)
	}
	alone := Board{Columns: []Bucket{{ID: colDone, IsDoneBucket: true}}}
	if _, found := alone.DoneSuccessor(colDone); found {
		t.Error("the last column named a successor")
	}
}

func TestDeletingTheDoneColumnNamesItsSuccessor(t *testing.T) {
	board := threeColumns()
	_, err := board.ChooseDoneSuccessor(colDone, "")
	if !errors.Is(err, shared.ErrConflict) || detailOf(t, err) != "buckets.done_successor_required" {
		t.Fatalf("deleting the done column without a successor: %v, want buckets.done_successor_required", err)
	}
	if got, err := board.ChooseDoneSuccessor(colDone, colTodo); err != nil || got != colTodo {
		t.Errorf("the chosen successor is %s, %v, want the column chosen", got, err)
	}
	for _, wrong := range []shared.ID{colDone, boardBy} {
		if _, err := board.ChooseDoneSuccessor(colDone, wrong); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("successor %s: %v, want buckets.not_found", wrong, err)
		}
	}
	if got, err := board.ChooseDoneSuccessor(colTodo, ""); err != nil || !got.IsZero() {
		t.Errorf("deleting an open column asked for a successor: %s, %v", got, err)
	}
	alone := Board{Columns: []Bucket{{ID: colDone, IsDoneBucket: true}}}
	if got, err := alone.ChooseDoneSuccessor(colDone, ""); err != nil || !got.IsZero() {
		t.Errorf("deleting the last column asked for a successor: %s, %v", got, err)
	}
	board.Mapping = []ColumnStatus{{BucketID: colDone, Status: "done"}}
	if got, err := board.ChooseDoneSuccessor(colDone, ""); err != nil || got != colDoing {
		t.Errorf("under a workflow the mark goes to %s, %v, want the suggestion", got, err)
	}
}

func TestTheDoneColumnAfterTheWorkflowGoes(t *testing.T) {
	shape := reviewShape()
	board := threeColumns()
	if done, _ := board.DoneAfterWorkflow(shape); done.ID != colDone {
		t.Errorf("the marked column is kept: %s", done.ID)
	}
	board.Columns[2].IsDoneBucket = false
	board.Mapping = []ColumnStatus{{BucketID: colDoing, Status: "done"}, {BucketID: colDone, Status: "review"}}
	if done, _ := board.DoneAfterWorkflow(shape); done.ID != colDoing {
		t.Errorf("unmarked, the column showing only done statuses takes the mark: %s", done.ID)
	}
	board.Mapping = nil
	if done, _ := board.DoneAfterWorkflow(shape); done.ID != colDone {
		t.Errorf("else the rightmost column: %s", done.ID)
	}
}

func TestABoardChangePreviewsLevelsColumnsAndMoves(t *testing.T) {
	shape := reviewShape()
	columns := threeColumns()
	mapped := columns
	mapped.Mapping, _ = DefaultMapping(columns.Columns, shape)

	t.Run("a workflow comes to a board", func(t *testing.T) {
		cells := []BoardCell{
			{BucketID: colTodo, Count: 2},                  // open in Offen: initial, stays
			{BucketID: colDoing, Count: 3},                 // open in In Arbeit: initial status, into Offen
			{BucketID: colDone, Completed: true, Count: 4}, // done: first done status, stays
			{Completed: true, Count: 1},                    // done without a column: into Erledigt
		}
		change := PlanBoardChange(columns, mapped, cells, nil, &shape)
		if change.Before != StateModeColumns || change.After != StateModeWorkflow {
			t.Errorf("levels %s → %s", change.Before, change.After)
		}
		if change.Moved != 4 || change.ColumnOnly != 0 {
			t.Errorf("moved %d, column only %d; want 4, 0", change.Moved, change.ColumnOnly)
		}
		if len(change.Unmapped) != 1 || change.Unmapped[0].Status != "review" || change.Unmapped[0].BucketID != colTodo {
			t.Errorf("unmapped %v", change.Unmapped)
		}
	})

	t.Run("a column-only move under the same statuses", func(t *testing.T) {
		cells := []BoardCell{{Status: "doing", BucketID: colTodo, Count: 2}}
		change := PlanBoardChange(mapped, mapped, cells, &shape, &shape)
		if change.Moved != 2 || change.ColumnOnly != 2 {
			t.Errorf("moved %d, column only %d; want 2, 2", change.Moved, change.ColumnOnly)
		}
	})

	t.Run("the workflow goes and the columns stay", func(t *testing.T) {
		cells := []BoardCell{
			{Status: "review", Count: 2},                                      // Ohne Spalte, open: first column
			{Status: "dropped", BucketID: colDone, Completed: true, Count: 1}, // stays in the done column
			{Status: "done", Completed: true, Count: 3},                       // Ohne Spalte, done: done column
			{Status: "doing", BucketID: colDoing, Count: 5},                   // stays
		}
		change := PlanBoardChange(mapped, columns, cells, &shape, nil)
		if change.Before != StateModeWorkflow || change.After != StateModeColumns {
			t.Errorf("levels %s → %s", change.Before, change.After)
		}
		if change.Moved != 5 || change.ColumnOnly != 0 || change.DoneColumn != colDone {
			t.Errorf("moved %d, column only %d, done %s", change.Moved, change.ColumnOnly, change.DoneColumn)
		}
		if len(change.Board.Mapping) != 0 || len(change.Unmapped) != 0 {
			t.Errorf("a board without a workflow kept a mapping: %v %v", change.Board.Mapping, change.Unmapped)
		}
	})

	t.Run("a collection without columns", func(t *testing.T) {
		change := PlanBoardChange(Board{}, Board{}, []BoardCell{{Count: 2}}, nil, &shape)
		if change.Before != StateModeNone || change.After != StateModeWorkflow || change.Moved != 0 {
			t.Errorf("%+v", change)
		}
	})
}

func TestANewColumnStandsInTheWorkflowsOrder(t *testing.T) {
	board := threeColumns()
	for index, key := range []string{"a", "m", "t"} {
		board.Columns[index].OrderKey = key
	}
	board.Mapping = []ColumnStatus{{colTodo, "open"}, {colDoing, "doing"}, {colDone, "done"}, {colDone, "dropped"}}
	shape := reviewShape()
	shape.Statuses = append([]WorkflowStatus{{Key: "backlog", Category: CategoryBacklog}}, shape.Statuses...)
	shape.Statuses = append(shape.Statuses,
		WorkflowStatus{Key: "parked", Category: CategoryTodo},
		WorkflowStatus{Key: "archived", Category: CategoryDone},
	)
	cases := []struct {
		status, after, before string
	}{
		{"backlog", "", "a"},
		{"review", "m", "t"},
		{"parked", "m", "t"},
		{"archived", "t", ""},
	}
	for _, c := range cases {
		t.Run(c.status, func(t *testing.T) {
			after, before := board.PlaceOfStatus(shape, c.status)
			if after != c.after || before != c.before {
				t.Fatalf("PlaceOfStatus(%s) = (%q, %q), want (%q, %q)", c.status, after, before, c.after, c.before)
			}
		})
	}
}
