// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

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
