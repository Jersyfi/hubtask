// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package eventbus

import (
	"context"
	"fmt"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	eventbusport "github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// Dispatcher delivers one tenant's pending events (ADR-0007).
//
// It is a job rather than a loop of its own, and one job per tenant rather than one for the
// installation. Both follow from row level security: the events of a tenant are only visible in a
// transaction opened for that tenant (multi-tenancy.md §2.1), so "read the outbox" is not a thing
// a process can do globally. One job per tenant is also what keeps a busy tenant from filling
// every batch while the quiet ones wait.
//
// The job is a poller that never finishes: instead of completing, each round schedules the next
// one. How soon depends on what it found - immediately while there is a backlog, shortly after a
// round that delivered something, and rarely for a tenant that has been quiet. A tenant that
// writes an event while its dispatcher is asleep pulls the wake-up forward, so the usual case does
// not wait for a poll at all (db/queries/Job.sql, EnqueueJob).
//
// Everything it does happens in the transaction the runner opened: the delivery, the record of
// what was delivered, and the completion of the job itself. A process that dies in the middle
// leaves none of the three behind.
type Dispatcher struct {
	// Events is the outbox of the running tenant.
	Events outbox.Pending
	// Consumed is what each subscriber has already seen. It is what makes a repeated delivery
	// harmless, and it is asked before every hand-over (test RT-4).
	Consumed eventbusport.Consumption
	// Subscribers are the consumers, in a fixed order. An empty list is a legitimate state and
	// not a reason to stop: the events are still marked as delivered, because "nobody subscribes"
	// is an answer, and a table that only grows would set off the backlog alert forever.
	Subscribers []eventbusport.Subscriber
	Clock       clock.Clock

	// Batch is how many events one round takes at most.
	Batch int
	// MinInterval is the wait after a round that delivered everything there was, MaxInterval the
	// wait for a tenant that had nothing at all. Between them sits the whole polling policy: the
	// first bounds how long a burst is chased, the second how much a quiet installation pays for
	// having the machinery at all.
	MinInterval time.Duration
	MaxInterval time.Duration

	// Lag reports the age of a delivered event in seconds, which is SLO-4 and the alert on events
	// being stuck (A-05). A hook rather than a metrics dependency, so that this adapter does not
	// have to know the observability one (project-structure.md §2). Nil is allowed.
	Lag func(ctx context.Context, seconds float64)
}

var _ queue.Handler = Dispatcher{}

// Run delivers one round.
func (d Dispatcher) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	if job.TenantID.IsZero() {
		// The transaction is opened for the tenant the job names. Without one there is no outbox
		// to read - a dispatch job without a tenant is a programming error, not an empty round.
		return queue.Result{}, shared.ErrInternal.WithDetail("outbox.dispatch_without_tenant")
	}

	envelopes, err := d.Events.Claim(ctx, d.Batch)
	if err != nil {
		return queue.Result{}, err
	}

	delivered := make([]shared.ID, 0, len(envelopes))
	for _, envelope := range envelopes {
		if err := d.deliver(ctx, envelope); err != nil {
			// The rest of the batch is left alone. Delivering the events after a failure would
			// hand consumers a later change before an earlier one, and per-aggregate order is the
			// one ordering guarantee the outbox gives.
			return queue.Result{}, err
		}
		delivered = append(delivered, envelope.ID)
	}

	now := d.Clock.Now()
	if err := d.Events.MarkDispatched(ctx, delivered, now); err != nil {
		return queue.Result{}, err
	}
	d.reportLag(ctx, envelopes, now)

	remaining, err := d.Events.CountPending(ctx)
	if err != nil {
		return queue.Result{}, err
	}
	return queue.Result{Repeat: true, RepeatAfter: d.nextRound(len(delivered), remaining)}, nil
}

// deliver hands one event to every subscriber that wants it.
func (d Dispatcher) deliver(ctx context.Context, envelope event.Envelope) error {
	for _, subscriber := range d.Subscribers {
		if !subscriber.Wants(envelope.Type) {
			continue
		}

		// The claim comes first, and it is a write rather than a question: two dispatchers asking
		// "has this been consumed" would both be told no. A repeat is not an error - it is the
		// at-least-once guarantee doing what it says (ADR-0007).
		first, err := d.Consumed.Claim(ctx, subscriber.Name(), envelope.ID)
		if err != nil {
			return err
		}
		if !first {
			continue
		}

		if err := subscriber.Deliver(ctx, envelope); err != nil {
			return fmt.Errorf("subscriber %s: %w", subscriber.Name(), err)
		}
	}
	return nil
}

// reportLag records how old each delivered event was. The measure is the age at delivery rather
// than the time the round took: what SLO-4 promises is that a change reaches its consumers within
// thirty seconds, and a fast round on a long backlog is not that.
func (d Dispatcher) reportLag(ctx context.Context, envelopes []event.Envelope, at time.Time) {
	if d.Lag == nil {
		return
	}
	for _, envelope := range envelopes {
		lag := at.Sub(envelope.OccurredAt)
		if lag < 0 {
			// A clock that went backwards between the write and the delivery. Reporting a negative
			// age would put a value in the histogram that cannot happen and would make the
			// percentile lie in the direction of "everything is fine".
			lag = 0
		}
		d.Lag(ctx, lag.Seconds())
	}
}

// nextRound is the adaptive interval ADR-0007 asks for, decided from what this round saw rather
// than from a counter carried between rounds - the job row is the only state a poller has, and it
// says nothing about the round before last.
func (d Dispatcher) nextRound(delivered, remaining int) time.Duration {
	switch {
	case remaining > 0:
		// The batch was not the end of it. Waiting now would be waiting with work in hand.
		return 0
	case delivered > 0:
		// Something happened here a moment ago, and activity comes in bursts.
		return d.MinInterval
	default:
		return d.MaxInterval
	}
}
