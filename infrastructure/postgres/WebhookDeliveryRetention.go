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

// DeleteExpired removes one batch of delivery attempts older than the cutoff, in the tenant the
// transaction is bound to (data-retention.md §3). Which ones lives in the statement: a pending
// attempt a living job still names is kept.
func (r WebhookDeliveryRepository) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	removed, err := queries.DeleteExpiredWebhookDeliveries(ctx, sqlc.DeleteExpiredWebhookDeliveriesParams{
		Cutoff: timestampOf(cutoff), Batch: boundedInt32(batch),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing expired webhook deliveries: %w", err))
	}
	return int(removed), nil
}

// CountExpired reports how many delivery attempts are due, counted no higher than ceiling.
func (r WebhookDeliveryRepository) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	due, err := queries.CountExpiredWebhookDeliveries(ctx, sqlc.CountExpiredWebhookDeliveriesParams{
		Cutoff: timestampOf(cutoff), Ceiling: boundedInt32(ceiling),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting expired webhook deliveries: %w", err))
	}
	return int(due), nil
}
