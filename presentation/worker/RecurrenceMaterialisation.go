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

// RecurrenceMaterialisation is the queue's way into what a series owes (D-05): an inbound adapter
// like every other handler, translating a job into a call on the application layer and its answer
// into the queue's vocabulary.
//
// It holds its own job row for the pass, for the reason the reminder's firing does: a pass that
// decides its own next wake-up must not lose a write that arrives while it is running
// (queue.Queue.Hold).
type RecurrenceMaterialisation struct {
	Materialisation work.MaterializeOccurrences
	Queue           queue.Queue
	Clock           clock.Clock
	// Continuation is the wait after a pass whose batch filled: there is known work left, and the
	// only reason not to do it now is that a batch is where one transaction ends.
	Continuation time.Duration
	// MinimumWait keeps a wake-up that is due now from becoming a spin.
	MinimumWait time.Duration
}

var _ queue.Handler = RecurrenceMaterialisation{}

// Run materialises what the tenant the job names owes.
func (m RecurrenceMaterialisation) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// Every read the pass makes is made for the tenant the job names. Without one there is
		// nothing to materialise - a series job without a tenant is a programming error, not an
		// empty pass.
		return queue.Result{}, shared.ErrInternal.
			WithDetail("recurrence.materialize_without_tenant")
	}
	if err := m.Queue.Hold(ctx, job); err != nil {
		return queue.Result{}, err
	}

	// The system acting for a tenant: a tenant, and no account. That is what tells anyone reading
	// an occurrence's history afterwards that it arrived because of a rule rather than because
	// somebody typed it.
	actor := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: job.TenantID}

	outcome, err := m.Materialisation.Execute(ctx, actor)
	if err != nil {
		return queue.Result{}, err
	}
	return m.next(outcome), nil
}

// next turns what the pass found into when to come back - or into finishing.
//
// A batch that filled comes straight back: there are more series than one transaction should hold.
// A tenant whose windows reach something later sleeps until then, which is the moment the horizon
// touches the next occurrence rather than the occurrence itself. A tenant that owes nothing
// finishes: for ON_COMPLETION that is the normal state, and the completion that starts the next
// one seeds a fresh job.
func (m RecurrenceMaterialisation) next(outcome work.MaterializationOutcome) queue.Result {
	if outcome.Considered >= m.Materialisation.RuleBatch && m.Materialisation.RuleBatch > 0 {
		return queue.Result{Repeat: true, RepeatAfter: m.Continuation}
	}
	if outcome.NextAt == nil {
		return queue.Result{}
	}

	wait := outcome.NextAt.Sub(m.Clock.Now())
	if wait < m.MinimumWait {
		wait = m.MinimumWait
	}
	return queue.Result{Repeat: true, RepeatAfter: wait}
}
