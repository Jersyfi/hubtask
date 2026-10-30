// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// The walk's time rows (offline-sync.md §3.1): a person's own records and timer, and the
// workspace's kinds of work and policy, read as the module's own repository reads them.

// OwnTimeRecords pages one person's time records.
func (SnapshotRepository) OwnTimeRecords(ctx context.Context, account, after shared.ID, batch int) ([]domain.Record, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	holder, err := uuidOf(account)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotOwnTimeRecords(ctx, sqlc.SnapshotOwnTimeRecordsParams{
		AccountID: holder, After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("time records", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotOwnTimeRecordsRow) (domain.Record, error) {
		return timeRecordFrom(row.ID, row.AccountID, row.EntryID, row.Corrects, row.WorkType, row.Day, row.StartedAt,
			row.Minutes, row.Note, row.Billable, row.Source, row.CreatedAt, row.UpdatedAt, row.Version)
	})
}

// OwnTimer reads one person's running timer.
func (SnapshotRepository) OwnTimer(ctx context.Context, account shared.ID) (domain.Timer, bool, error) {
	return TimeRepository{}.Timer(ctx, account)
}

// TimeWorkTypes reads the workspace's kinds of work.
func (SnapshotRepository) TimeWorkTypes(ctx context.Context) ([]domain.WorkType, error) {
	return TimeRepository{}.WorkTypes(ctx)
}

// TimePolicy reads the workspace's time policy.
func (SnapshotRepository) TimePolicy(ctx context.Context) (domain.Policy, error) {
	return TimeRepository{}.Policy(ctx)
}
