// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// Sprints pages the workspace's sprints.
func (SnapshotRepository) Sprints(ctx context.Context, after shared.ID, batch int) ([]planning.Sprint, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotSprints(ctx, sqlc.SnapshotSprintsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("sprints", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotSprintsRow) (planning.Sprint, error) {
		return sprintFrom(sqlc.FindSprintRow(row))
	})
}

// SprintEntries pages every live entry's open sprint.
func (SnapshotRepository) SprintEntries(ctx context.Context, after shared.ID, batch int) ([]repository.SprintEntryOf, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotSprintEntries(ctx, sqlc.SnapshotSprintEntriesParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("sprint entries", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotSprintEntriesRow) (repository.SprintEntryOf, error) {
		entry, err := idFrom(row.EntryID)
		if err != nil {
			return repository.SprintEntryOf{}, err
		}
		sprint, err := idFrom(row.SprintID)
		if err != nil {
			return repository.SprintEntryOf{}, err
		}
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return repository.SprintEntryOf{}, err
		}
		return repository.SprintEntryOf{EntryID: entry, SprintID: sprint, CollectionID: collection, Committed: row.Committed}, nil
	})
}

// EntryEstimates pages every live entry's estimate.
func (SnapshotRepository) EntryEstimates(ctx context.Context, after shared.ID, batch int) ([]repository.EstimateOf, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotEntryEstimates(ctx, sqlc.SnapshotEntryEstimatesParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("estimates", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotEntryEstimatesRow) (repository.EstimateOf, error) {
		entry, err := idFrom(row.EntryID)
		if err != nil {
			return repository.EstimateOf{}, err
		}
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return repository.EstimateOf{}, err
		}
		e := planning.Estimate{EntryID: entry, Unit: planning.EstimateUnit(row.Unit), Value: row.Value, Version: int(row.Version)}
		if row.Size != nil {
			e.Size = *row.Size
		}
		return repository.EstimateOf{Estimate: e, CollectionID: collection}, nil
	})
}
