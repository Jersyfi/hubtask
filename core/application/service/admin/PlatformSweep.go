// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

const (
	// platformSweepBatch bounds one statement; platformSweepRounds bounds one pass, so a backlog
	// after a long outage is worked off over several passes rather than in one transaction.
	platformSweepBatch  = 1000
	platformSweepRounds = 50
	// PlatformSweepInterval is how often the sweep comes back. Daily would do for a 30-day window;
	// hourly keeps each pass small.
	PlatformSweepInterval = time.Hour
	platformSweepKey      = "installation"
	// platformSweepMargin is how much longer than the window an event is kept. A cursor's moment
	// and an event's are both read before the event's number is drawn, so near the window's edge a
	// later number can carry an earlier moment; sweeping a day after the 410 begins means no event a
	// still-valid cursor has not read is gone.
	platformSweepMargin = 24 * time.Hour
)

// SweepPlatformEvents removes the installation's events past their 30 days (data-catalog.md). An
// installation job: it runs under the system scope, which reaches the policy-free table and no
// workspace's rows.
type SweepPlatformEvents struct {
	Events     adminrepo.PlatformEvents
	Jobs       JobQueue
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	Tenancy    env.TenancyMode
}

// Seed asks for the sweep once a worker starts. Single mode keeps no events and seeds nothing.
func (h SweepPlatformEvents) Seed(ctx context.Context) error {
	if h.Tenancy != env.TenancyMulti {
		return nil
	}
	return h.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		_, err := h.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindPlatformSweep, DedupeKey: platformSweepKey, RunAt: h.Clock.Now().UTC(),
		})
		return err
	})
}

// Run removes one pass's worth of expired events inside the job's transaction, and answers how
// many.
func (h SweepPlatformEvents) Run(ctx context.Context) (int, error) {
	cutoff := h.Clock.Now().Add(-PlatformEventsRetention - platformSweepMargin)
	total := 0
	for range platformSweepRounds {
		removed, err := h.Events.Sweep(ctx, cutoff, platformSweepBatch)
		if err != nil {
			return total, err
		}
		total += removed
		if removed < platformSweepBatch {
			break
		}
	}
	return total, nil
}
