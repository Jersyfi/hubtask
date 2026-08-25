// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// ReminderFiring is the queue's way into the first duty this system has that is caused by a stored
// future timestamp rather than by something somebody just did (D-03).
//
// An inbound adapter like every other handler: it translates a job into a call on the application
// layer and the answer into the queue's vocabulary. What is particular to it is the shape of the
// answer - it reschedules itself to the moment the tenant next owes something and finishes when it
// owes nothing, rather than polling forever like the retention sweep. A quiet tenant therefore
// costs one row that is not there, and the next write re-seeds it.
type ReminderFiring struct {
	Firing work.FireReminders
	// Queue is here for one statement: the row lock on this job, taken before anything is read.
	// Without it a write committing between the pass's last read and its reschedule would find
	// the row RUNNING, where Enqueue cannot pull a wake-up forward, and its reminder would wait
	// for a wake-up nobody scheduled (queue.Queue.Hold).
	Queue queue.Queue
	Clock clock.Clock
	// Continuation is the wait after a pass that filled its batch: there is known work left, and
	// the only reason not to do it now is that a batch is where one transaction ends.
	Continuation time.Duration
	// MinimumWait keeps a wake-up that is due now from becoming a spin: a reminder whose moment
	// has just passed is fired by this pass, and the next round is never closer than this.
	MinimumWait time.Duration
}

var _ queue.Handler = ReminderFiring{}

// Run fires what is due for the tenant the job names.
func (f ReminderFiring) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// Every read the pass makes is made for the tenant the job names. Without one there is
		// nothing to fire - a reminder job without a tenant is a programming error, not an empty
		// pass.
		return queue.Result{}, shared.ErrInternal.WithDetail("reminders.fire_without_tenant")
	}
	if err := f.Queue.Hold(ctx, job); err != nil {
		return queue.Result{}, err
	}

	// The system acting for a tenant: a tenant, and no account. That is what tells anyone reading
	// the records afterwards that a message went because a moment arrived rather than because
	// somebody sent it.
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}

	outcome, err := f.Firing.Execute(ctx, actor)
	if err != nil {
		return queue.Result{}, err
	}

	return f.next(outcome), nil
}

// next turns what the pass found into when to come back - or into finishing.
//
// Three answers, in the order they are decided. A batch that filled comes straight back: what is
// left is known and due already. A tenant that owes something later sleeps until then, which is
// the whole point of a stored timestamp - no polling, and the wake-up is the moment itself. A
// tenant that owes nothing finishes: the row goes, and the next write brings it back.
func (f ReminderFiring) next(outcome work.ReminderOutcome) queue.Result {
	if outcome.NextAt == nil {
		return queue.Result{}
	}

	wait := outcome.NextAt.Sub(f.Clock.Now())
	if wait < f.MinimumWait {
		wait = f.MinimumWait
	}
	if outcome.Fired+outcome.Cancelled >= f.Firing.BatchSize && f.Firing.BatchSize > 0 {
		wait = f.Continuation
	}
	return queue.Result{Repeat: true, RepeatAfter: wait}
}
