// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// EndedTimerExpiry removes the timers that fired or were stopped before the cutoff (TIMER).
type EndedTimerExpiry struct{}

// ConnectorRunExpiry removes the connector runs past the period (CONNECTOR_RUN), each connector's
// newest and those with an item still held kept.
type ConnectorRunExpiry struct{}

func sweepFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

func (EndedTimerExpiry) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.DeleteEndedTimers(ctx, sqlc.DeleteEndedTimersParams{Cutoff: timestampOf(cutoff), Batch: boundedInt32(batch)})
	if err != nil {
		return 0, sweepFailed("removing ended timers", err)
	}
	return int(n), nil
}

func (EndedTimerExpiry) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.CountEndedTimers(ctx, sqlc.CountEndedTimersParams{Cutoff: timestampOf(cutoff), Ceiling: boundedInt32(ceiling)})
	if err != nil {
		return 0, sweepFailed("counting ended timers", err)
	}
	return int(n), nil
}

func (ConnectorRunExpiry) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.DeleteExpiredConnectorRuns(ctx, sqlc.DeleteExpiredConnectorRunsParams{Cutoff: timestampOf(cutoff), Batch: boundedInt32(batch)})
	if err != nil {
		return 0, sweepFailed("removing expired connector runs", err)
	}
	return int(n), nil
}

func (ConnectorRunExpiry) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.CountExpiredConnectorRuns(ctx, sqlc.CountExpiredConnectorRunsParams{Cutoff: timestampOf(cutoff), Ceiling: boundedInt32(ceiling)})
	if err != nil {
		return 0, sweepFailed("counting expired connector runs", err)
	}
	return int(n), nil
}
