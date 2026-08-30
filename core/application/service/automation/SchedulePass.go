// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/recurrence"
)

// ScheduleHorizon is how far ahead the next occurrence of a rule is looked for.
//
// A year, the same answer E-05 gave: a rule that produces nothing in a year produces nothing
// anybody is waiting for, and `FREQ=YEARLY;BYMONTH=2;BYMONTHDAY=29` is the shape that gets closest
// and still lands inside four.
const ScheduleHorizon = 366 * 24 * time.Hour

// scheduleBatch bounds one round. A backlog of missed moments - a worker that was down for a week -
// becomes several rounds rather than a hundred jobs enqueued in one transaction.
const scheduleBatch = 50

// SchedulePass turns the SCHEDULE rules whose moment has come into runs (G-08, decision 5 of
// milestone-0.5.0).
//
// One pass per tenant, and the tenant is the one the caller opens the transaction under. That is
// the whole of the leader-versus-self-seeding question, answered the way E-05 answered it: nothing
// in this system may enumerate tenants (multi-tenancy.md §2.1), so a scheduler cannot create one
// job per tenant even if it wanted to. The write that creates or enables a rule seeds that tenant's
// poller, each round reschedules itself to the next moment the tenant owes, and a tenant that owes
// nothing lets its poller finish - the next write brings it back. A quiet tenant costs nothing.
//
// It fires rules into the engine G-07 built. It evaluates no condition and performs no action: a
// schedule is a *producer*, and the run that follows is the same run an event's would have been.
type SchedulePass struct {
	Schedules  repository.Schedules
	Jobs       queue.Queue
	Expander   recurrence.Expander
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// PassResult is what one round did, and when the next is owed.
type PassResult struct {
	// Started is how many runs this round queued.
	Started int
	// NextDue is the earliest moment this tenant owes anything, and the zero time when it owes
	// nothing.
	NextDue time.Time
}

// Hold takes the row lock on the pass's own job, for the reason D-03's reminders and E-05's backup
// schedules take one: the pass decides when it next runs from the data, and a write committing
// between that read and the reschedule would find the row RUNNING - where the queue's conflict
// clause cannot pull a wake-up forward - and its schedule would wait for a wake-up nobody made.
func (p SchedulePass) Hold(ctx context.Context, job queue.Job) error {
	return p.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		return p.Jobs.Hold(ctx, job)
	})
}

// Run does one round for the tenant the caller opens it under.
func (p SchedulePass) Run(ctx context.Context, scope persistence.Scope) (PassResult, error) {
	now := p.Clock.Now()
	var result PassResult

	err := p.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		due, err := p.Schedules.Due(ctx, now, scheduleBatch)
		if err != nil {
			return err
		}
		for _, rule := range due {
			started, err := p.fire(ctx, rule, scope.TenantID, now)
			if err != nil {
				return err
			}
			if started {
				result.Started++
			}
		}

		result.NextDue, err = p.Schedules.NextDue(ctx)
		return err
	})
	if err != nil {
		return PassResult{}, err
	}
	return result, nil
}

// fire queues one rule's run and moves the rule on to its next moment.
//
// The rule is advanced whether or not the run could be queued, exactly as E-05's schedules are: a
// rule that stayed on a moment it could not act on would try again on every round for ever, and a
// missed occurrence is a missed occurrence rather than a reason to stop.
//
// A rule whose recurrence this installation cannot read is advanced to *no* moment rather than
// failing the pass. It was readable when it was written - the write refuses one that is not - so
// this is a rule whose library changed under it, and one tenant's unreadable rule must not stop
// every other rule in the same tenant from firing.
func (p SchedulePass) fire(
	ctx context.Context, rule domain.Rule, tenantID shared.ID, now time.Time,
) (bool, error) {
	occurrence := rule.NextRunAt

	// The next moment is measured from *now* when the occurrence is already behind, which is the
	// one place this pass deliberately differs from E-05's. A worker that was down over a weekend
	// leaves three missed nights on a nightly rule; advancing occurrence by occurrence would fire
	// three runs on Monday morning, one after the other, for a rule whose author asked for "every
	// night at three". One catch-up run and then forward from now is what a rule means.
	after := occurrence
	if after.Before(now) {
		after = now
	}

	next, err := p.nextAfter(rule, after)
	if err != nil && !errors.Is(err, shared.ErrValidation) {
		return false, err
	}
	if err := p.Schedules.SetNextRun(ctx, rule.ID, next); err != nil {
		return false, err
	}
	if err != nil {
		// Unreadable: advanced to nothing above, and no run queued. The rule stays visible and
		// editable, which is the state its author can act on.
		return false, nil
	}

	runID := p.IDs.NewID()
	// The occasion is the occurrence, not the run. Two workers that somehow claimed the same
	// moment would then write the same idempotency keys and the second would act on nothing -
	// which is what makes a schedule at-most-once in effect while the queue is at-least-once.
	occasion := "schedule:" + occurrence.UTC().Format(time.RFC3339)

	_, err = p.Jobs.Enqueue(ctx, queue.Request{
		Kind:     queue.KindAutomationRun,
		TenantID: tenantID,
		Payload: map[string]any{
			"rule_id":  rule.ID.String(),
			"trigger":  string(domain.TriggerSchedule),
			"run_id":   runID.String(),
			"occasion": occasion,
		},
		DedupeKey: ConsumerName + ":" + rule.ID.String() + ":" + occasion,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

// nextAfter is when the rule fires after this occurrence, and the zero time when it never does.
func (p SchedulePass) nextAfter(rule domain.Rule, occurrence time.Time) (time.Time, error) {
	return NextOccurrence(p.Expander, rule, occurrence)
}

// NextOccurrence answers when a SCHEDULE rule next fires after a moment, and the zero time when it
// never does again.
//
// Shared by the pass and by the writers, because they have to agree: a rule whose next moment the
// write computed one way and the pass another would drift by a day the first time somebody edited
// it. It answers the zero time for a rule that is not a schedule at all, which is what makes the
// writers able to call it unconditionally.
func NextOccurrence(
	expander recurrence.Expander, rule domain.Rule, after time.Time,
) (time.Time, error) {
	if rule.Trigger.Kind != domain.TriggerSchedule || expander == nil {
		return time.Time{}, nil
	}

	moments, err := expander.Occurrences(recurrence.Rule{
		RRULE:    rule.Trigger.RRule,
		TimeZone: rule.Trigger.Timezone,
		Start:    rule.Anchor(),
	}, after, after.Add(ScheduleHorizon), 1)
	if err != nil {
		return time.Time{}, ScheduleRefusal(err)
	}
	if len(moments) == 0 {
		// A rule that is simply over. Stored with no moment rather than refused: the rule may be
		// perfectly good and exhausted, and one an operator can see and edit is better than an
		// error that loses what they typed.
		return time.Time{}, nil
	}
	return moments[0].UTC(), nil
}

// ScheduleRefusal turns the expander's two sentinels into the field errors a client can act on, so
// that an unreadable rule is answered to its author while they are still looking at it rather than
// failing at three in the morning.
func ScheduleRefusal(err error) error {
	switch {
	case errors.Is(err, recurrence.ErrRuleUnreadable):
		return shared.ErrValidation.
			WithDetail("automation.rrule_unreadable").
			WithFields(shared.FieldError{
				Path: "/trigger/rrule", Code: "automation.rrule_unreadable",
			})
	case errors.Is(err, recurrence.ErrZoneUnknown):
		return shared.ErrValidation.
			WithDetail("automation.timezone_unknown").
			WithFields(shared.FieldError{
				Path: "/trigger/timezone", Code: "automation.timezone_unknown",
			})
	}
	return shared.Internalf("automation: expanding a rule's schedule: %w", err)
}
