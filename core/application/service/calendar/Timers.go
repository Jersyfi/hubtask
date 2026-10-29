// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package calendar runs durable timers (domain-model.md §3.7): arming, pausing, resuming and
// stopping them inside the transaction of the use case that decides it, and the pass that fires
// what is due. What an elapsed timer means is its purpose's: an SLA breached, an approval
// escalated, a change's check opened - each registered as a TimerElapsed by the step that builds it.
package calendar

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/calendar"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/calendar"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// TimerElapsed is what one purpose does when its timer fires, inside the firing pass's
// transaction.
type TimerElapsed interface {
	Elapsed(ctx context.Context, actor appshared.ActorContext, timer domain.Timer) error
}

// Clocks is what a use case holds to run timers. Every method runs in the caller's transaction,
// so a timer armed by a write that rolls back was never armed.
type Clocks struct {
	Timers repository.Timers
	// Jobs is where the tenant's next firing is asked for: the write that makes a moment due seeds
	// it, because nothing may enumerate tenants (multi-tenancy.md §2.1).
	Jobs  queue.Queue
	Clock clock.Clock
	IDs   clock.IDGenerator
}

// Arm starts a timer of a purpose for a subject on a calendar.
func (c Clocks) Arm(
	ctx context.Context, tenant shared.ID, purpose domain.Purpose, subject shared.ID, name string,
	cal domain.BusinessCalendar, budget time.Duration,
) (domain.Timer, error) {
	now := c.Clock.Now()
	timer, err := domain.Arm(c.IDs.NewID(), tenant, purpose, subject, name, cal, budget, now)
	if err != nil {
		return domain.Timer{}, err
	}
	if err := c.Timers.Insert(ctx, timer, now); err != nil {
		return domain.Timer{}, err
	}
	return timer, c.schedule(ctx, tenant, timer.DueAt)
}

// Pause stops a live timer's clock.
func (c Clocks) Pause(ctx context.Context, purpose domain.Purpose, subject shared.ID, name string) (domain.Timer, error) {
	return c.change(ctx, purpose, subject, name, domain.Timer.Pause)
}

// Resume runs a paused timer's clock again.
func (c Clocks) Resume(ctx context.Context, purpose domain.Purpose, subject shared.ID, name string) (domain.Timer, error) {
	return c.change(ctx, purpose, subject, name, domain.Timer.Resume)
}

// Stop ends a live timer before it fires.
func (c Clocks) Stop(ctx context.Context, purpose domain.Purpose, subject shared.ID, name string) (domain.Timer, error) {
	return c.change(ctx, purpose, subject, name, domain.Timer.Stop)
}

func (c Clocks) change(
	ctx context.Context, purpose domain.Purpose, subject shared.ID, name string,
	act func(domain.Timer, time.Time) (domain.Timer, error),
) (domain.Timer, error) {
	current, err := c.Timers.FindLive(ctx, purpose, subject, name)
	if err != nil {
		return domain.Timer{}, err
	}
	now := c.Clock.Now()
	next, err := act(current, now)
	if err != nil {
		return domain.Timer{}, err
	}
	if next.State == current.State {
		return current, nil
	}
	if err := c.Timers.Update(ctx, next, current.Version, now); err != nil {
		return domain.Timer{}, err
	}
	next.Version = current.Version + 1
	return next, c.schedule(ctx, next.TenantID, next.DueAt)
}

func (c Clocks) schedule(ctx context.Context, tenant shared.ID, at *time.Time) error {
	if c.Jobs == nil || at == nil {
		return nil
	}
	_, err := c.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindTimerFire, TenantID: tenant, DedupeKey: tenant.String(), RunAt: at.UTC(),
	})
	return err
}

// DefaultTimerBatch is how many timers one pass fires, small enough to keep the transaction short.
const DefaultTimerBatch = 100

// FireTimers fires the tenant's timers whose moment has come.
type FireTimers struct {
	Timers repository.Timers
	Clock  clock.Clock
	// Elapsed is each purpose's handler. A purpose without one fires and is counted as unhandled:
	// the timer's state is still the truth, and the step that builds the purpose reads it.
	Elapsed   map[domain.Purpose]TimerElapsed
	BatchSize int
}

// TimerOutcome is what one pass did and when the tenant needs the next.
type TimerOutcome struct {
	Fired     int
	Unhandled int
	NextAt    *time.Time
}

// FilledBatch reports whether the pass ran into its bound, so the job comes straight back.
func (o TimerOutcome) FilledBatch(batch int) bool { return batch > 0 && o.Fired >= batch }

func (f FireTimers) batch() int {
	if f.BatchSize > 0 {
		return f.BatchSize
	}
	return DefaultTimerBatch
}

// Execute fires what is due for the tenant the actor names.
func (f FireTimers) Execute(ctx context.Context, actor appshared.ActorContext) (TimerOutcome, error) {
	if actor.TenantID.IsZero() {
		return TimerOutcome{}, shared.ErrInternal.WithDetail("timers.fire_without_tenant")
	}
	now := f.Clock.Now()
	due, err := f.Timers.ClaimDue(ctx, now, f.batch())
	if err != nil {
		return TimerOutcome{}, err
	}
	outcome := TimerOutcome{}
	for _, timer := range due {
		fired, err := timer.Fire(now)
		if err != nil {
			return TimerOutcome{}, err
		}
		if err := f.Timers.Update(ctx, fired, timer.Version, now); err != nil {
			return TimerOutcome{}, err
		}
		fired.Version = timer.Version + 1
		outcome.Fired++
		handler, ok := f.Elapsed[fired.Purpose]
		if !ok {
			outcome.Unhandled++
			continue
		}
		if err := handler.Elapsed(ctx, actor, fired); err != nil {
			return TimerOutcome{}, err
		}
	}
	// Read after the pass, so the answer is what is left.
	if outcome.NextAt, err = f.Timers.NextDue(ctx); err != nil {
		return TimerOutcome{}, err
	}
	return outcome, nil
}
