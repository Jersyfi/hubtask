// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/calendar"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// TimerFiring is the job of queue.KindTimerFire: one per tenant, firing its due timers and coming
// back at the next due moment - the reminder's shape (ReminderFiring).
type TimerFiring struct {
	Firing       calendar.FireTimers
	Queue        queue.Queue
	Clock        clock.Clock
	Continuation time.Duration
	MinimumWait  time.Duration
}

var _ queue.Handler = TimerFiring{}

func (f TimerFiring) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("timers.fire_without_tenant")
	}
	// The row lock makes a concurrent arm wait for this pass instead of finding the job RUNNING
	// and leaving its earlier moment unscheduled.
	if err := f.Queue.Hold(ctx, job); err != nil {
		return queue.Result{}, err
	}
	outcome, err := f.Firing.Execute(ctx, appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID})
	if err != nil {
		return queue.Result{}, err
	}
	if outcome.NextAt == nil {
		return queue.Result{}, nil
	}
	wait := max(outcome.NextAt.Sub(f.Clock.Now()), f.MinimumWait)
	if outcome.FilledBatch(f.Firing.BatchSize) {
		wait = f.Continuation
	}
	return queue.Result{Repeat: true, RepeatAfter: wait}, nil
}
