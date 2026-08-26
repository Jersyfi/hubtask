// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"

	service "github.com/Jersyfi/hubtask/core/application/service/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// PrivacyDeadlines watches one tenant's statutory deadlines (E-10, alert A-19).
//
// The reminder poller's shape, and for the reason that one has it: nothing in this system may
// enumerate tenants, so a scheduler cannot create one of these per tenant. The write that records a
// case seeds it; each pass reschedules itself while the tenant has an open case; a tenant that owes
// nothing finishes, and its next case brings the job back.
type PrivacyDeadlines struct {
	Watch service.WatchDeadlines
	// Queue is here for one statement: the row lock on this job, taken before anything is read.
	// A case recorded between this pass's read and its reschedule would otherwise find the row
	// RUNNING, where Enqueue cannot pull a wake-up forward, and its deadline would wait for a
	// wake-up nobody scheduled.
	Queue queue.Queue
	Clock clock.Clock
}

var _ queue.Handler = PrivacyDeadlines{}

// Run looks at what the tenant owes.
func (d PrivacyDeadlines) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		return queue.Result{}, shared.Internalf("privacy: a deadline job without a tenant")
	}
	if err := d.Queue.Hold(ctx, job); err != nil {
		return queue.Result{}, err
	}

	// The system acting for a tenant: a tenant, and no account. A deadline is watched because a
	// moment is approaching rather than because somebody asked.
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}

	watched, err := d.Watch.Execute(ctx, actor)
	if err != nil {
		return queue.Result{}, err
	}
	if watched.NextAt == nil {
		// Nothing open. The row goes, and the next case brings it back.
		return queue.Result{}, nil
	}
	return queue.Result{Repeat: true, RepeatAfter: watched.NextAt.Sub(d.Clock.Now())}, nil
}
