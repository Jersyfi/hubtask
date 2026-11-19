// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// SchemeBoards is what a change of the workflows in force for a board's entry types does to a collection's board
// (domain-model.md §3.5a, UC-FLW-02 check 4): planned for the preview, then the mapping written
// before the statuses move and the entries placed after.
type SchemeBoards struct {
	Columns BucketWriter
	// Types is the registry, which tells the entry types standing on a board; nil takes the types
	// the scheme names.
	Types repository.EntryTypes
}

// Wired reports whether the build reads boards and their mapping.
func (s SchemeBoards) Wired() bool {
	return s.Columns.Boards.Wired() && s.Columns.Boards.Statuses != nil
}

// Plan answers what moving a collection's tasks from the workflow was to will - nil for none -
// does to its board. Given is the mapping the person chose, across boards - a row without a status
// names a column that shows none -: a board one of whose columns it names takes exactly its rows;
// else a board that keeps its statuses keeps its mapping, and the rest work.DefaultMapping.
func (s SchemeBoards) Plan(
	ctx context.Context, collectionID shared.ID, was, will *domain.WorkflowShape, given []domain.ColumnStatus,
) (domain.BoardChange, error) {
	board, err := s.Columns.Boards.Of(ctx, collectionID)
	if err != nil {
		return domain.BoardChange{}, err
	}
	after := domain.Board{Columns: board.Columns}
	if will != nil && len(board.Columns) > 0 {
		mine, named := rowsOn(board, given)
		switch {
		case named:
			if after.Mapping, err = domain.NewBoardMapping(board.Columns, mine, *will); err != nil {
				return domain.BoardChange{}, err
			}
		case keeps(board.Mapping, *will):
			after.Mapping = board.Mapping
		default:
			after.Mapping, _ = domain.DefaultMapping(board.Columns, *will)
		}
	}
	var cells []domain.BoardCell
	if len(board.Columns) > 0 {
		if cells, err = s.Columns.Boards.Statuses.Cells(ctx, collectionID); err != nil {
			return domain.BoardChange{}, err
		}
	}
	return domain.PlanBoardChange(board, after, cells, was, will), nil
}

// rowsOn answers the given rows naming this board's columns, without the empty ones, and whether
// any named one.
func rowsOn(board domain.Board, given []domain.ColumnStatus) ([]domain.ColumnStatus, bool) {
	var mine []domain.ColumnStatus
	named := false
	for _, row := range given {
		if _, on := board.Column(row.BucketID); !on {
			continue
		}
		named = true
		if row.Status != "" {
			mine = append(mine, row)
		}
	}
	return mine, named
}

// keeps reports a mapping every status of which the shape knows.
func keeps(mapping []domain.ColumnStatus, shape domain.WorkflowShape) bool {
	if len(mapping) == 0 {
		return false
	}
	for _, row := range mapping {
		if _, known := shape.Status(row.Status); !known {
			return false
		}
	}
	return true
}

// Map writes the board's mapping after the change, empty where no workflow runs then; before the
// statuses move, so that each moved status lands in the column showing it.
func (s SchemeBoards) Map(ctx context.Context, collectionID shared.ID, change domain.BoardChange) error {
	if len(change.Board.Columns) == 0 {
		return nil
	}
	return s.Columns.Boards.Statuses.Replace(ctx, collectionID, change.Board.Mapping)
}

// Settle places the board's entries once the statuses moved: under a workflow each in the column
// of its status - the moves a status change alone does not make -; without one the done column
// marked, the completed entries in it, the open ones out of it and those without a column in the
// first.
func (s SchemeBoards) Settle(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, change domain.BoardChange,
	will *domain.WorkflowShape, now time.Time,
) error {
	w := s.Columns
	if len(change.Board.Columns) == 0 {
		return nil
	}
	board, err := w.Boards.Of(ctx, collectionID)
	if err != nil {
		return err
	}
	if will != nil {
		for _, status := range will.Statuses {
			column, _ := board.ColumnOfStatus(status.Key)
			placed, err := w.Boards.Statuses.PlaceByStatus(ctx, collectionID, status.Key, column, now)
			if err != nil {
				return err
			}
			if err := w.recordEntries(ctx, actor, collectionID, placed, domain.FieldBucketID, idOrNil(column)); err != nil {
				return err
			}
		}
		return nil
	}
	done := change.DoneColumn
	if current, found := board.DoneColumn(); !found || current.ID != done {
		collection, err := findCollection(ctx, w.Containers, collectionID)
		if err != nil {
			return err
		}
		// The mark alone: the open entries of the new done column move out below rather than
		// being completed, as the preview counted them.
		marked, cleared := true, false
		for _, column := range board.Columns {
			want := &cleared
			if column.ID == done {
				want = &marked
			} else if !column.IsDoneBucket {
				continue
			}
			if err := w.setDone(ctx, actor, column.ID, want, collection); err != nil {
				return err
			}
		}
		if board, err = w.Boards.Of(ctx, collectionID); err != nil {
			return err
		}
	}
	first, _ := board.FirstColumn()
	for _, put := range []struct {
		completed    bool
		from, column shared.ID
	}{{true, "", done}, {false, done, first.ID}} {
		placed, err := w.Boards.Statuses.PlaceByCompletion(ctx, collectionID, put.completed, put.from, put.column, now)
		if err != nil {
			return err
		}
		if err := w.recordEntries(ctx, actor, collectionID, placed, domain.FieldBucketID, idOrNil(put.column)); err != nil {
			return err
		}
	}
	placed, err := w.Boards.Statuses.PlaceUnplaced(ctx, collectionID, false, first.ID, now)
	if err != nil {
		return err
	}
	return w.recordEntries(ctx, actor, collectionID, placed, domain.FieldBucketID, idOrNil(first.ID))
}
