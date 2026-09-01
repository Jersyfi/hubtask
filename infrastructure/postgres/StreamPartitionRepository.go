// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/streams"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// StreamPartitionRepository is the monthly duty of the partitioned streams (H-09), the audit
// partition repository's shape: two narrow SECURITY DEFINER acts, nothing else.
type StreamPartitionRepository struct{}

func NewStreamPartitionRepository() StreamPartitionRepository { return StreamPartitionRepository{} }

var _ repository.Partitions = StreamPartitionRepository{}

// Ensure creates or repairs one month's partition.
func (StreamPartitionRepository) Ensure(
	ctx context.Context, table string, month time.Time,
) (string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}

	first := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	name, err := queries.EnsureStreamPartition(ctx, sqlc.EnsureStreamPartitionParams{
		Parent: table,
		Month:  pgtype.Date{Time: first, Valid: true},
	})
	if err != nil {
		return "", shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("ensuring the %s partition: %w", table, err))
	}
	return name, nil
}

// DropAged lets go what has wholly aged out.
func (StreamPartitionRepository) DropAged(
	ctx context.Context, table string, defaultDays int,
) ([]repository.Dropped, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.DropAgedStreamPartitions(ctx, sqlc.DropAgedStreamPartitionsParams{
		Parent: table,
		//nolint:gosec // G115: a day count from the closed catalogue, tiny
		DefaultDays: int32(defaultDays),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("dropping aged %s partitions: %w", table, err))
	}

	dropped := make([]repository.Dropped, 0, len(rows))
	for _, row := range rows {
		dropped = append(dropped, repository.Dropped{Name: row.Dropped, Rows: row.RowsRemoved})
	}
	return dropped, nil
}
