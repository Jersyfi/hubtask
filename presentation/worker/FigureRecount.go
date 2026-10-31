// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// FigureRecount is the queue's way into a workspace's recount of its kept figures (migration 0292).
// It reschedules itself, for MediaReconciliation's reason: nothing may enumerate tenants, so the
// first count a read keeps schedules it, and it never finishes for good - the next read keeps more.
// It runs in the runner's transaction; a pass is safe to run twice.
type FigureRecount struct {
	Recount work.RecountFigures
	// Interval is the wait after a pass that verified less than a batch.
	Interval time.Duration
	// Continuation is the wait after a full batch: there may be more to verify.
	Continuation time.Duration
}

var _ queue.Handler = FigureRecount{}

// Run does one pass for the tenant the job names.
func (r FigureRecount) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.ErrInternal.WithDetail("figure.recount_without_tenant")
	}
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}
	outcome, err := r.Recount.Execute(ctx, actor)
	if err != nil {
		return queue.Result{}, err
	}
	after := r.Interval
	if outcome.Full {
		after = r.Continuation
	}
	return queue.Result{Repeat: true, RepeatAfter: after}, nil
}
