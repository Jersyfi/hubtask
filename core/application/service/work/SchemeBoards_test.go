// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// What a change of the workflow in force does to a board (UC-FLW-02 check 4, domain-model.md
// §3.5a): the preview's mapping - chosen, kept or proposed -, the moves it counts, and the
// placing once the statuses moved.

func boardShape() domain.WorkflowShape {
	return domain.WorkflowShape{
		Statuses: []domain.WorkflowStatus{
			{Key: "open", Name: "Offen", Category: domain.CategoryTodo},
			{Key: "doing", Name: "Doing", Category: domain.CategoryInProgress},
			{Key: "done", Name: "Fertig", Category: domain.CategoryDone},
		},
		Initial: "open",
		Steps:   []domain.WorkflowStep{{Key: "go", To: "doing"}, {Key: "end", To: "done"}},
	}
}

func schemeBoards(rows []domain.ColumnStatus, cells []domain.BoardCell) (SchemeBoards, *writerHarness, *mapping) {
	h := newWriterHarness()
	threeColumnBoard(h.buckets)
	board := &mapping{rows: rows, cells: cells}
	writer := h.update.Writer
	writer.Boards = Boards{Buckets: h.buckets, Statuses: board}
	return SchemeBoards{Columns: writer}, h, board
}

func TestASchemeChangePlansTheBoard(t *testing.T) {
	shape := boardShape()
	t.Run("the proposal", func(t *testing.T) {
		boards, _, _ := schemeBoards(nil, []domain.BoardCell{{BucketID: doingColumn, Count: 2}})
		change, err := boards.Plan(t.Context(), collectionID, nil, &shape, nil)
		if err != nil {
			t.Fatal(err)
		}
		if change.Before != domain.StateModeColumns || change.After != domain.StateModeWorkflow {
			t.Errorf("levels %s → %s", change.Before, change.After)
		}
		if !slices.Contains(change.Board.Mapping, domain.ColumnStatus{BucketID: todoColumn, Status: "open"}) || change.Moved != 2 {
			t.Errorf("mapping %v, moved %d", change.Board.Mapping, change.Moved)
		}
	})
	t.Run("the columns chosen", func(t *testing.T) {
		boards, _, _ := schemeBoards(nil, nil)
		chosen := []domain.ColumnStatus{{BucketID: doingColumn, Status: "open"}}
		change, err := boards.Plan(t.Context(), collectionID, nil, &shape, chosen)
		if err != nil || !slices.Equal(change.Board.Mapping, chosen) {
			t.Fatalf("mapping %v: %v", change.Board.Mapping, err)
		}
		if len(change.Unmapped) != 2 {
			t.Errorf("unmapped %v, want doing and done", change.Unmapped)
		}
		_, err = boards.Plan(t.Context(), collectionID, nil, &shape, []domain.ColumnStatus{{BucketID: doingColumn, Status: "gone"}})
		if !errors.Is(err, shared.ErrValidation) {
			t.Errorf("a status the workflow does not know: %v", err)
		}
	})
	t.Run("a board that keeps its statuses keeps its mapping", func(t *testing.T) {
		kept := []domain.ColumnStatus{{BucketID: doingColumn, Status: "open"}}
		boards, _, _ := schemeBoards(kept, nil)
		change, err := boards.Plan(t.Context(), collectionID, &shape, &shape, nil)
		if err != nil || !slices.Equal(change.Board.Mapping, kept) {
			t.Errorf("mapping %v: %v", change.Board.Mapping, err)
		}
	})
}

func TestASchemeChangeSettlesTheBoard(t *testing.T) {
	shape := boardShape()
	t.Run("under the workflow every status into its column", func(t *testing.T) {
		rows := []domain.ColumnStatus{{BucketID: todoColumn, Status: "open"}, {BucketID: doneColumn, Status: "done"}}
		boards, _, board := schemeBoards(rows, nil)
		change, _ := boards.Plan(t.Context(), collectionID, nil, &shape, rows)
		if err := boards.Settle(t.Context(), actor(), collectionID, change, &shape, now); err != nil {
			t.Fatal(err)
		}
		want := []placedByStatus{{status: "open", column: todoColumn}, {status: "doing"}, {status: "done", column: doneColumn}}
		if !slices.Equal(board.placed, want) {
			t.Errorf("placed %+v, want %+v", board.placed, want)
		}
	})
	t.Run("the workflow goes, the columns and completion stay", func(t *testing.T) {
		rows := []domain.ColumnStatus{{BucketID: todoColumn, Status: "open"}, {BucketID: doingColumn, Status: "done"}}
		boards, h, board := schemeBoards(rows, nil)
		stored := h.buckets.stored[doneColumn]
		stored.IsDoneBucket = false
		h.buckets.stored[doneColumn] = stored
		change, err := boards.Plan(t.Context(), collectionID, &shape, nil, nil)
		if err != nil || change.After != domain.StateModeColumns || change.DoneColumn != doingColumn {
			t.Fatalf("change %+v: %v", change, err)
		}
		if err := boards.Map(t.Context(), collectionID, change); err != nil || len(board.rows) != 0 {
			t.Fatalf("the mapping stays %v: %v", board.rows, err)
		}
		if err := boards.Settle(t.Context(), actor(), collectionID, change, nil, now); err != nil {
			t.Fatal(err)
		}
		if !h.buckets.stored[doingColumn].IsDoneBucket {
			t.Error("the column showing only done statuses was not marked done")
		}
		want := []placedByStatus{
			{completed: true, column: doingColumn},
			{from: doingColumn, column: todoColumn},
			{from: unplaced, column: todoColumn},
		}
		if !slices.Equal(board.placed, want) {
			t.Errorf("placed %+v, want %+v", board.placed, want)
		}
		if len(board.completedIn) != 0 {
			t.Errorf("the removal completed entries in %v", board.completedIn)
		}
	})
}
