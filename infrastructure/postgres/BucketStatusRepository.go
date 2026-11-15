// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// BucketStatusRepository stores which statuses a board's columns show. The tenant comes from
// the caller's transaction, as everywhere in this package (ADR-0010).
type BucketStatusRepository struct{}

func NewBucketStatusRepository() BucketStatusRepository { return BucketStatusRepository{} }

var _ repository.BucketStatuses = BucketStatusRepository{}

// List returns a board's mapping, left to right.
func (r BucketStatusRepository) List(ctx context.Context, collectionID shared.ID) ([]work.ColumnStatus, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	collection, err := uuidOf(collectionID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListBucketStatuses(ctx, collection)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the board's statuses: %w", err))
	}
	out := make([]work.ColumnStatus, 0, len(rows))
	for _, row := range rows {
		bucket, err := idFrom(row.BucketID)
		if err != nil {
			return nil, err
		}
		out = append(out, work.ColumnStatus{BucketID: bucket, Status: row.Status})
	}
	return out, nil
}

// Replace writes a board's whole mapping in place of the one it had.
func (r BucketStatusRepository) Replace(ctx context.Context, collectionID shared.ID, mapping []work.ColumnStatus) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	collection, err := uuidOf(collectionID)
	if err != nil {
		return err
	}
	if _, err := queries.ClearBucketStatuses(ctx, collection); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing the board's statuses: %w", err))
	}
	for _, row := range mapping {
		bucket, err := uuidOf(row.BucketID)
		if err != nil {
			return err
		}
		written, err := queries.InsertBucketStatus(ctx, sqlc.InsertBucketStatusParams{
			Status: row.Status, BucketID: bucket, CollectionID: collection,
		})
		if err != nil {
			return shared.ErrUnavailable.WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("writing the board's statuses: %w", err))
		}
		// The join found no live column of this board: another board's, another tenant's or
		// a deleted one - which the caller cannot tell apart (multi-tenancy.md §2).
		if written == 0 {
			return shared.ErrNotFound.WithDetail("buckets.not_found").
				WithParams(map[string]string{"bucket_id": row.BucketID.String()})
		}
	}
	return nil
}

// PlaceByStatus puts a collection's top-level tasks in one status into a column.
func (r BucketStatusRepository) PlaceByStatus(
	ctx context.Context, collectionID shared.ID, status string, column shared.ID, at time.Time,
) ([]repository.MovedEntry, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	collection, err := uuidOf(collectionID)
	if err != nil {
		return nil, err
	}
	bucket, err := optionalUUID(column)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PlaceWorkItemsByStatus(ctx, sqlc.PlaceWorkItemsByStatusParams{
		BucketID: bucket, UpdatedAt: timestampOf(at), CollectionID: collection, Status: status,
	})
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("placing entries by status: %w", err))
	}
	return placedEntries(len(rows), func(i int) (pgtype.UUID, int32) { return rows[i].ID, rows[i].Version })
}

// PlaceByCompletion puts a collection's top-level tasks without a status, of one completion, into
// a column.
func (r BucketStatusRepository) PlaceByCompletion(
	ctx context.Context, collectionID shared.ID, completed bool, from, column shared.ID, at time.Time,
) ([]repository.MovedEntry, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	collection, err := uuidOf(collectionID)
	if err != nil {
		return nil, err
	}
	bucket, err := optionalUUID(column)
	if err != nil {
		return nil, err
	}
	source, err := optionalUUID(from)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PlaceWorkItemsByCompletion(ctx, sqlc.PlaceWorkItemsByCompletionParams{
		BucketID: bucket, UpdatedAt: timestampOf(at), CollectionID: collection,
		IsCompleted: completed, FromBucket: source,
	})
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("placing entries by completion: %w", err))
	}
	return placedEntries(len(rows), func(i int) (pgtype.UUID, int32) { return rows[i].ID, rows[i].Version })
}

func placedEntries(n int, row func(int) (pgtype.UUID, int32)) ([]repository.MovedEntry, error) {
	out := make([]repository.MovedEntry, 0, n)
	for i := range n {
		raw, version := row(i)
		id, err := idFrom(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.MovedEntry{ID: id, Version: int(version)})
	}
	return out, nil
}

// Collections answers the collections whose board has a live column.
func (r BucketStatusRepository) Collections(ctx context.Context) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListBoardCollections(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the boards: %w", err))
	}
	out := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// CompleteInColumn completes the open top-level tasks without a status in a column.
func (r BucketStatusRepository) CompleteInColumn(
	ctx context.Context, collectionID, column, by shared.ID, at time.Time,
) ([]repository.MovedEntry, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	ids, err := uuidsOf([]shared.ID{collectionID, column, by})
	if err != nil {
		return nil, err
	}
	rows, err := queries.CompleteWorkItemsInColumn(ctx, sqlc.CompleteWorkItemsInColumnParams{
		At: timestampOf(at), CompletedBy: ids[2], CollectionID: ids[0], BucketID: ids[1],
	})
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("completing the done column's entries: %w", err))
	}
	return placedEntries(len(rows), func(i int) (pgtype.UUID, int32) { return rows[i].ID, rows[i].Version })
}

// PlaceUnplaced puts a collection's top-level tasks without a status and without a column, of one
// completion, into a column.
func (r BucketStatusRepository) PlaceUnplaced(
	ctx context.Context, collectionID shared.ID, completed bool, column shared.ID, at time.Time,
) ([]repository.MovedEntry, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	ids, err := uuidsOf([]shared.ID{collectionID, column})
	if err != nil {
		return nil, err
	}
	rows, err := queries.PlaceUnplacedWorkItems(ctx, sqlc.PlaceUnplacedWorkItemsParams{
		BucketID: ids[1], UpdatedAt: timestampOf(at), CollectionID: ids[0], IsCompleted: completed,
	})
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("placing entries without a column: %w", err))
	}
	return placedEntries(len(rows), func(i int) (pgtype.UUID, int32) { return rows[i].ID, rows[i].Version })
}

// Cells counts a board's top-level tasks by status, column and completion.
func (r BucketStatusRepository) Cells(ctx context.Context, collectionID shared.ID) ([]work.BoardCell, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	collection, err := uuidOf(collectionID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.CountBoardCells(ctx, collection)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the board's entries by column: %w", err))
	}
	out := make([]work.BoardCell, 0, len(rows))
	for _, row := range rows {
		column, err := optionalID(row.BucketID)
		if err != nil {
			return nil, err
		}
		out = append(out, work.BoardCell{Status: row.Status, BucketID: column, Completed: row.IsCompleted, Count: int(row.Count)})
	}
	return out, nil
}

// SetStatusInColumn gives a status to the top-level tasks without one standing in a column, or,
// with no column, to those of one completion standing in none.
func (r BucketStatusRepository) SetStatusInColumn(
	ctx context.Context, collectionID, column shared.ID, completed bool, status string, at time.Time,
) ([]repository.MovedEntry, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	collection, err := uuidOf(collectionID)
	if err != nil {
		return nil, err
	}
	bucket, err := optionalUUID(column)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SetWorkItemStatusInColumn(ctx, sqlc.SetWorkItemStatusInColumnParams{
		Status: status, UpdatedAt: timestampOf(at), CollectionID: collection, BucketID: bucket, IsCompleted: completed,
	})
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("giving a column's entries a status: %w", err))
	}
	return placedEntries(len(rows), func(i int) (pgtype.UUID, int32) { return rows[i].ID, rows[i].Version })
}
