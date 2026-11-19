// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// GoalSetRepository stores the marks of the goal periods a template brought (migration 0440).
type GoalSetRepository struct{}

var _ repository.GoalSets = GoalSetRepository{}

// Mark marks the period; a key the workspace marks already writes nothing.
func (GoalSetRepository) Mark(ctx context.Context, period shared.ID, key string, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	periodKey, err := uuidOf(period)
	if err != nil {
		return false, err
	}
	n, err := queries.InsertConfigGoalSet(ctx, sqlc.InsertConfigGoalSetParams{
		Key: key, PeriodID: periodKey, CreatedAt: timestampOf(at),
	})
	if err != nil {
		return false, sampleFailed("marking a template's goal set", err)
	}
	return n == 1, nil
}

// List answers the workspace's marked periods.
func (GoalSetRepository) List(ctx context.Context) ([]repository.GoalSet, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListConfigGoalSets(ctx)
	if err != nil {
		return nil, sampleFailed("listing a template's goal sets", err)
	}
	out := make([]repository.GoalSet, 0, len(rows))
	for _, row := range rows {
		period, err := idFrom(row.PeriodID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.GoalSet{
			Key: row.Key, PeriodID: period, Name: row.Name,
			StartOn: row.StartOn.Time.UTC(), EndOn: row.EndOn.Time.UTC(),
		})
	}
	return out, nil
}
