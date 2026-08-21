// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"time"

	"context"

	"github.com/Jersyfi/hubtask/core/application/service/lifecycle"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RetentionSweep is the queue's way into the retention run: an inbound adapter, like every other
// handler, that translates a job into a call on the application layer and its answer into the
// queue's vocabulary.
//
// It reschedules itself rather than being rescheduled. One row per tenant that comes back forever is
// what a poller looks like once it is a queue (queue.Result.Repeat), and it is the only shape
// available here: nothing in this system may enumerate tenants, so a scheduler cannot create one job
// per tenant even if it wanted to. The first job is created by a deletion, which is also the more
// honest statement of what has to happen - something was deleted, so its cleanup is now due.
type RetentionSweep struct {
	Retention lifecycle.RunRetention
	// Interval is the wait after a pass that reached the end of the trash. It is what a quiet tenant
	// pays for having the machinery at all.
	Interval time.Duration
	// Continuation is the wait after a pass that filled its batch, and is short: there is known work
	// left, and the only reason not to do it now is that a batch is where one transaction ends.
	Continuation time.Duration
}

var _ queue.Handler = RetentionSweep{}

// Run does one pass for the tenant the job names.
func (s RetentionSweep) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// The transaction is opened for the tenant the job names. Without one there is nothing to
		// sweep - a retention job without a tenant is a programming error, not an empty pass.
		return queue.Result{}, shared.ErrInternal.WithDetail("retention.sweep_without_tenant")
	}

	// The system acting for a tenant: a tenant, and no account. The audit trail records exactly
	// that, which is what tells an auditor a removal was the schedule rather than a person.
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}

	outcome, err := s.Retention.Execute(ctx, actor)
	if err != nil {
		return queue.Result{}, err
	}

	// Back at once while there is known work left, and at the long interval once the trash is
	// through. The job is never finished for good: the next thing to expire is always coming, and a
	// row that removed itself would leave the tenant with no sweep until its next deletion.
	after := s.Interval
	if !s.Retention.Exhausted(outcome) {
		after = s.Continuation
	}
	return queue.Result{Repeat: true, RepeatAfter: after}, nil
}
