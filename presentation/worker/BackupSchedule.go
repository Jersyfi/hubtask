// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	service "github.com/Jersyfi/hubtask/core/application/service/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// BackupScheduling is one tenant's wake-up: what does this tenant owe now, and when is the next
// moment it owes anything (E-05, backup-restore.md §5).
//
// The same shape as the reminders' and the recurrence materialisation's, and for the same reason:
// nothing in this system may enumerate tenants, so a scheduler cannot create one job per tenant
// even if it wanted to (multi-tenancy.md §2.1). The write that creates a schedule seeds it, each
// round reschedules itself to the next moment the tenant owes, and a tenant that owes nothing lets
// its poller finish - the next write re-seeds it. A quiet tenant costs nothing.
//
// It is *not* Detached. Everything it does is a write - a job enqueued and a schedule moved on -
// and those belong in the runner's transaction with the job's own completion, so that a process
// that dies half way leaves neither an enqueued run nor a schedule that has been advanced past a
// moment nothing acted on.
type BackupScheduling struct {
	Pass service.SchedulePass
	// Fallback is when to come back if the tenant owes nothing at all. It is not a poll: the pass
	// reschedules itself to the moment it actually owes, and this is only the answer to "there is
	// nothing on the books", where finishing would be correct and coming back once a day is
	// cheaper than being wrong about a schedule that was written by another process.
	Fallback time.Duration
}

var _ queue.Handler = BackupScheduling{}

// Run does one round for the tenant the job names.
func (h BackupScheduling) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// A tenant's poller without a tenant is a programming error, not an empty round. The
		// instance-wide schedules are the leader's duty and never arrive here.
		return queue.Result{}, shared.ErrInternal.WithDetail("backup.schedule_pass_without_tenant")
	}

	// The row lock this pass takes on its own job, for the reason D-03's reminders take one: the
	// pass decides when it next runs from the data, and a write committing between that read and
	// the reschedule would find the row RUNNING - where the queue's conflict clause cannot pull a
	// wake-up forward - and its schedule would wait for a wake-up nobody scheduled.
	if err := h.Pass.Hold(ctx, job); err != nil {
		return queue.Result{}, err
	}

	result, err := h.Pass.Run(ctx, persistence.Scope{TenantID: job.TenantID})
	if err != nil {
		return queue.Result{}, err
	}

	if result.NextDue.IsZero() {
		// Nothing on the books. The poller finishes rather than spinning, and the next schedule
		// somebody writes seeds a new one.
		return queue.Result{}, nil
	}
	return queue.Result{
		Repeat: true, RepeatAfter: waitUntil(h.Pass.Clock.Now(), result.NextDue, h.Fallback),
	}, nil
}

// waitUntil is how long to sleep before the next moment, floored at nothing and capped at the
// fallback: a schedule whose next moment is a year away does not deserve a lease that long, and one
// whose moment has already passed is due now.
func waitUntil(now, next time.Time, fallback time.Duration) time.Duration {
	if fallback <= 0 {
		fallback = 24 * time.Hour
	}
	wait := next.Sub(now)
	switch {
	case wait < 0:
		return 0
	case wait > fallback:
		return fallback
	}
	return wait
}
