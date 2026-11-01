// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	health "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// Backlogs counts the installation's queues for the health report: the due jobs, the dead
// letters, the webhook retries parked on the queue, and the outbox through its door (0345).
// Counts only, in one read-only transaction.
type Backlogs struct {
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Read answers one count of every queue.
func (b Backlogs) Read(ctx context.Context) (health.Backlogs, error) {
	var out health.Backlogs
	err := b.UnitOfWork.WithinReadOnly(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		queries, err := queriesFrom(ctx)
		if err != nil {
			return err
		}
		now := timestampOf(b.Clock.Now())
		depths, err := queries.JobQueueDepth(ctx, now)
		if err != nil {
			return backlogFailed("the queue", err)
		}
		for _, depth := range depths {
			out.JobQueueDepth += depth.Pending
		}
		if out.DeadLetterTotal, err = queries.DeadLetterCount(ctx); err != nil {
			return backlogFailed("the dead letters", err)
		}
		if out.WebhookRetryBacklog, err = queries.ScheduledJobBacklog(ctx, sqlc.ScheduledJobBacklogParams{
			Kind: string(queue.KindWebhookDeliver), Now: now,
		}); err != nil {
			return backlogFailed("the webhook retries", err)
		}
		outbox, err := queries.OutboxBacklog(ctx)
		if err != nil {
			return backlogFailed("the outbox", err)
		}
		out.OutboxPending, out.OutboxLagSeconds = outbox.Pending, outbox.OldestSeconds
		return nil
	})
	return out, err
}

func backlogFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("counting %s for the health report: %w", what, err))
}
