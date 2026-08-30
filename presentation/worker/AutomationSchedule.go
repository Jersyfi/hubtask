// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// AutomationScheduling is one tenant's wake-up for its SCHEDULE rules (G-08).
//
// The same shape as E-05's backup scheduling and D-03's reminders, and for the same reason: nothing
// in this system may enumerate tenants, so a scheduler cannot create one job per tenant even if it
// wanted to (multi-tenancy.md §2.1). The write that creates or enables a scheduled rule seeds this
// job for its own tenant, each round reschedules itself to the moment the tenant next owes
// something, and a tenant that owes nothing lets its poller finish - the next write re-seeds it.
//
// It is *not* Detached. Everything it does is a write - runs queued and rules moved on - and those
// belong in the runner's transaction with the job's own completion, so that a process that dies
// halfway leaves neither a queued run nor a rule advanced past a moment nothing acted on.
type AutomationScheduling struct {
	Pass automation.SchedulePass
	// Fallback is when to come back if the tenant owes nothing at all. It is not a poll: the pass
	// reschedules itself to the moment it actually owes, and this is only the cap on a lease - a
	// rule whose next moment is a year away does not deserve one that long.
	Fallback time.Duration
}

var _ queue.Handler = AutomationScheduling{}

// Run does one round for the tenant the job names.
func (h AutomationScheduling) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// Every read the pass makes is made for the tenant the job names. Without one there is
		// nothing to fire - and there is no instance-wide automation rule for a leader to own,
		// because `automation_rule.tenant_id` is NOT NULL.
		return queue.Result{}, shared.ErrInternal.WithDetail("automation.schedule_pass_without_tenant")
	}

	// The row lock on the pass's own job, before anything is read. Without it a write committing
	// between the pass's last read and its reschedule would find the row RUNNING - where the
	// queue's conflict clause cannot pull a wake-up forward - and its rule would wait for a
	// wake-up nobody made.
	if err := h.Pass.Hold(ctx, job); err != nil {
		return queue.Result{}, err
	}

	result, err := h.Pass.Run(ctx, persistence.Scope{TenantID: job.TenantID})
	if err != nil {
		return queue.Result{}, err
	}

	if result.NextDue.IsZero() {
		// Nothing on the books. The poller finishes rather than spinning, and the next scheduled
		// rule somebody writes seeds a new one.
		return queue.Result{}, nil
	}
	return queue.Result{
		Repeat: true, RepeatAfter: waitUntil(h.Pass.Clock.Now(), result.NextDue, h.Fallback),
	}, nil
}
