// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Boards reads a collection's board and keeps an entry's column agreeing with where it stands
// (domain-model.md §3.5a): without a workflow column and completion move together, under one the
// column follows the status. Every door that sets a column, a completion or a status asks it.
type Boards struct {
	Buckets repository.Buckets
	// Statuses is the column-status mapping; nil reads every status as *Ohne Spalte*'s.
	Statuses repository.BucketStatuses
}

// Wired reports whether the build reads boards; one without couples nothing.
func (b Boards) Wired() bool { return b.Buckets != nil }

// Of answers a collection's live columns and their mapping.
func (b Boards) Of(ctx context.Context, collectionID shared.ID) (domain.Board, error) {
	columns, err := b.Buckets.List(ctx, collectionID)
	if err != nil {
		return domain.Board{}, err
	}
	board := domain.Board{Columns: columns}
	if b.Statuses != nil && len(columns) > 0 {
		if board.Mapping, err = b.Statuses.List(ctx, collectionID); err != nil {
			return domain.Board{}, err
		}
	}
	return board, nil
}

// couple answers after with its column agreeing: under a workflow - the entry carries a status -
// the column showing the status, none for *Ohne Spalte*; without one Board.Coupled.
func (b Boards) couple(ctx context.Context, before, after domain.WorkItem, by shared.ID, at time.Time) (domain.WorkItem, error) {
	if !b.Wired() || after.Type != domain.ItemTask {
		return after, nil
	}
	board, err := b.Of(ctx, after.CollectionID)
	if err != nil {
		return domain.WorkItem{}, err
	}
	if len(board.Columns) == 0 {
		return after, nil
	}
	if after.Status != "" {
		after.BucketID, _ = board.ColumnOfStatus(after.Status)
		return after, nil
	}
	return board.Coupled(before, after, by, at), nil
}

// refuseColumnUnderWorkflow refuses setting a column by hand where a workflow decides it: the
// column follows the status, and a drop takes the step (UC-WRK-05, UC-FLW-06).
func refuseColumnUnderWorkflow(item domain.WorkItem, wanted shared.ID) error {
	if item.Status == "" || wanted == item.BucketID {
		return nil
	}
	return shared.ErrConflict.WithDetail("board.step_required").
		WithFields(shared.FieldError{Path: "/bucket_id", Code: "board.step_required"})
}
