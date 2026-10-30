// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// Approvals pages every live entry's approvals with their trail and the entry's collection.
func (SnapshotRepository) Approvals(ctx context.Context, after shared.ID, batch int) ([]repository.ApprovalOf, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotApprovals(ctx, sqlc.SnapshotApprovalsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("approvals", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotApprovalsRow) (repository.ApprovalOf, error) {
		approval, err := approvalFromRow(sqlc.Approval{
			ID: row.ID, TenantID: row.TenantID, ItemID: row.ItemID, WorkflowKey: row.WorkflowKey, StepKey: row.StepKey,
			ItemVersion: row.ItemVersion, Levels: row.Levels, DeclineTo: row.DeclineTo, DueHours: row.DueHours,
			CurrentLevel: row.CurrentLevel, State: row.State, RequestedBy: row.RequestedBy, RequestedAt: row.RequestedAt,
			LevelSince: row.LevelSince, DecidedAt: row.DecidedAt, Version: row.Version,
		})
		if err != nil {
			return repository.ApprovalOf{}, err
		}
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return repository.ApprovalOf{}, err
		}
		decisions, err := ServiceDeskApprovals{}.Decisions(ctx, approval.ID)
		if err != nil {
			return repository.ApprovalOf{}, err
		}
		return repository.ApprovalOf{Approval: approval, Decisions: decisions, CollectionID: collection}, nil
	})
}

// EntrySlas pages every live entry's SLA records with the entry's collection.
func (SnapshotRepository) EntrySlas(ctx context.Context, after shared.ID, batch int) ([]repository.EntrySlaOf, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotEntrySlas(ctx, sqlc.SnapshotEntrySlasParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("SLA records", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotEntrySlasRow) (repository.EntrySlaOf, error) {
		record, err := recordFrom(sqlc.EntrySla{
			ID: row.ID, TenantID: row.TenantID, ItemID: row.ItemID, Goal: row.Goal, PolicyKey: row.PolicyKey,
			TargetSeconds: row.TargetSeconds, Standing: row.Standing, ArmedAt: row.ArmedAt, DueAt: row.DueAt,
			AtRiskAt: row.AtRiskAt, MetAt: row.MetAt, BreachedAt: row.BreachedAt, UpdatedAt: row.UpdatedAt,
			Version: row.Version,
		})
		if err != nil {
			return repository.EntrySlaOf{}, err
		}
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return repository.EntrySlaOf{}, err
		}
		return repository.EntrySlaOf{Record: record, CollectionID: collection}, nil
	})
}

// SatisfactionRatings pages every live request's rating with the request's collection.
func (SnapshotRepository) SatisfactionRatings(ctx context.Context, after shared.ID, batch int) ([]repository.RatingOf, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotSatisfactionRatings(ctx, sqlc.SnapshotSatisfactionRatingsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("satisfaction ratings", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotSatisfactionRatingsRow) (repository.RatingOf, error) {
		rating, err := ratingFrom(sqlc.SatisfactionRating{
			ID: row.ID, TenantID: row.TenantID, ItemID: row.ItemID, Score: row.Score, RatedAt: row.RatedAt, Version: row.Version,
		})
		if err != nil {
			return repository.RatingOf{}, err
		}
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return repository.RatingOf{}, err
		}
		return repository.RatingOf{Rating: rating, CollectionID: collection}, nil
	})
}
