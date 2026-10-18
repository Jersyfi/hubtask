// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// QuotaReachedRepository keeps the marks of limits reached (repository.Reached).
type QuotaReachedRepository struct{}

// NewQuotaReachedRepository builds the repository.
func NewQuotaReachedRepository() QuotaReachedRepository { return QuotaReachedRepository{} }

var _ repository.Reached = QuotaReachedRepository{}

// Mark marks the quota at its period, forgetting its other periods.
func (QuotaReachedRepository) Mark(ctx context.Context, quota, period string, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	standing, err := queries.QuotaReachedMarkedAt(ctx, sqlc.QuotaReachedMarkedAtParams{Quota: quota, Period: period})
	if err != nil {
		return false, reachedFailed(err)
	}
	if standing {
		return false, nil
	}
	if err := queries.ForgetOtherQuotaPeriods(ctx, sqlc.ForgetOtherQuotaPeriodsParams{Quota: quota, Period: period}); err != nil {
		return false, reachedFailed(err)
	}
	marked, err := queries.MarkQuotaReached(ctx, sqlc.MarkQuotaReachedParams{
		Quota: quota, Period: period, ReachedAt: pgtype.Timestamptz{Time: at.UTC(), Valid: true},
	})
	if err != nil {
		return false, reachedFailed(err)
	}
	return marked == 1, nil
}

// Clear removes the quota's mark, reading first so that a create finding none writes nothing.
func (QuotaReachedRepository) Clear(ctx context.Context, quota string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	marked, err := queries.QuotaReachedMarked(ctx, quota)
	if err != nil {
		return reachedFailed(err)
	}
	if !marked {
		return nil
	}
	if err := queries.ClearQuotaReached(ctx, quota); err != nil {
		return reachedFailed(err)
	}
	return nil
}

func reachedFailed(err error) error {
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("marking a limit reached: %w", err))
}
