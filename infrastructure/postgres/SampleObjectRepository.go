// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/config"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// SampleObjectRepository stores the marks of the workspace's sample content (migration 0384).
type SampleObjectRepository struct{}

var _ repository.SampleMarks = SampleObjectRepository{}

// Mark marks the object; a key marked before answers the mark it holds.
func (SampleObjectRepository) Mark(ctx context.Context, mark domain.SampleMark, at time.Time) (domain.SampleMark, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.SampleMark{}, err
	}
	object, err := uuidOf(shared.ID(mark.ObjectID))
	if err != nil {
		return domain.SampleMark{}, err
	}
	row, err := queries.InsertSampleObject(ctx, sqlc.InsertSampleObjectParams{
		Key: mark.Key, Kind: string(mark.Kind), ObjectID: object, Version: mark.Version, CreatedAt: timestampOf(at),
	})
	if err != nil {
		return domain.SampleMark{}, sampleFailed("marking sample content", err)
	}
	return sampleMarkOf(row.Key, row.Kind, row.ObjectID, row.Version)
}

// List answers the workspace's marks by key.
func (SampleObjectRepository) List(ctx context.Context) ([]domain.SampleMark, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListSampleObjects(ctx, repository.MaxSampleMarks)
	if err != nil {
		return nil, sampleFailed("listing sample content", err)
	}
	out := make([]domain.SampleMark, 0, len(rows))
	for _, row := range rows {
		mark, err := sampleMarkOf(row.Key, row.Kind, row.ObjectID, row.Version)
		if err != nil {
			return nil, err
		}
		out = append(out, mark)
	}
	return out, nil
}

// Unmark removes the mark under key.
func (SampleObjectRepository) Unmark(ctx context.Context, key string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	if _, err := queries.DeleteSampleObject(ctx, key); err != nil {
		return sampleFailed("unmarking sample content", err)
	}
	return nil
}

func sampleMarkOf(key, kind string, object pgtype.UUID, version *int64) (domain.SampleMark, error) {
	id, err := idFrom(object)
	if err != nil {
		return domain.SampleMark{}, err
	}
	return domain.SampleMark{Key: key, Kind: domain.SampleKind(kind), ObjectID: id.String(), Version: version}, nil
}
