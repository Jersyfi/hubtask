// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/recurrence"
)

// schedules is the narrow repository the pass holds, in memory.
type schedules struct {
	rows  map[shared.ID]domain.Rule
	order []shared.ID
	moved map[shared.ID]time.Time
}

func newSchedules(rules ...domain.Rule) *schedules {
	s := &schedules{rows: map[shared.ID]domain.Rule{}, moved: map[shared.ID]time.Time{}}
	for _, rule := range rules {
		s.rows[rule.ID] = rule
		s.order = append(s.order, rule.ID)
	}
	return s
}

func (s *schedules) Due(_ context.Context, at time.Time, limit int) ([]domain.Rule, error) {
	var due []domain.Rule
	for _, id := range s.order {
		rule := s.rows[id]
		if !rule.Enabled || rule.NextRunAt.IsZero() || rule.NextRunAt.After(at) {
			continue
		}
		if len(due) == limit {
			break
		}
		due = append(due, rule)
	}
	return due, nil
}

func (s *schedules) NextDue(context.Context) (time.Time, error) {
	var next time.Time
	for _, rule := range s.rows {
		if !rule.Enabled || rule.NextRunAt.IsZero() {
			continue
		}
		if next.IsZero() || rule.NextRunAt.Before(next) {
			next = rule.NextRunAt
		}
	}
	return next, nil
}

func (s *schedules) SetNextRun(_ context.Context, id shared.ID, at time.Time) error {
	rule := s.rows[id]
	rule.NextRunAt = at
	s.rows[id] = rule
	s.moved[id] = at
	return nil
}

// passQueue is queue.Queue as the pass uses it: two of its methods and no more.
type passQueue struct{ jobs }

func (p *passQueue) Claim(context.Context, queue.Lease) ([]queue.Job, error) { return nil, nil }
func (p *passQueue) Hold(context.Context, queue.Job) error                   { return nil }
func (p *passQueue) Complete(context.Context, queue.Job) error               { return nil }
func (p *passQueue) Repeat(context.Context, queue.Job, time.Time) error      { return nil }
func (p *passQueue) Fail(context.Context, queue.Failure) error               { return nil }
func (p *passQueue) Depth(context.Context) ([]queue.Depth, error)            { return nil, nil }

// expander is the recurrence port, answering a fixed step so that a test states the moments rather
// than the library does. The library's own answers are the golden files' subject
// (infrastructure/recurrence/testdata), including G-08's 03:00 pair across both transitions.
type expander struct {
	step time.Duration
	err  error
}

func (e expander) Occurrences(
	_ recurrence.Rule, after, before time.Time, limit int,
) ([]time.Time, error) {
	if e.err != nil {
		return nil, e.err
	}
	var moments []time.Time
	for at := after.Add(e.step); !at.After(before) && len(moments) < limit; at = at.Add(e.step) {
		moments = append(moments, at)
	}
	return moments, nil
}

func scheduledRule(next time.Time) domain.Rule {
	rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)
	rule.Trigger = domain.Trigger{
		Kind: domain.TriggerSchedule, RRule: "FREQ=DAILY;BYHOUR=3", Timezone: "Europe/Berlin",
	}
	rule.Enabled, rule.CreatedAt, rule.NextRunAt = true, now.Add(-24*time.Hour), next
	return rule
}

func newPass(store *schedules, step time.Duration) (SchedulePass, *passQueue) {
	q := &passQueue{}
	return SchedulePass{
		Schedules: store, Jobs: q, Expander: expander{step: step},
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: manualRunID},
	}, q
}

func tenantScope() persistence.Scope { return persistence.Scope{TenantID: tenant} }

// The acceptance criterion, minus the DST half the golden files carry: a rule whose moment has come
// produces a run into the same engine an event's run goes to, and the rule moves on.
func TestADueScheduleQueuesARunAndMovesOn(t *testing.T) {
	due := now.Add(-time.Minute)
	store := newSchedules(scheduledRule(due))
	pass, q := newPass(store, 24*time.Hour)

	result, err := pass.Run(context.Background(), tenantScope())
	if err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if result.Started != 1 {
		t.Fatalf("%d runs started, want one", result.Started)
	}
	if len(q.queued) != 1 {
		t.Fatalf("%d jobs queued, want one", len(q.queued))
	}

	job := q.queued[0]
	if job.Kind != queue.KindAutomationRun {
		t.Errorf("job kind %q, want the engine's - a schedule is a producer, not a second engine", job.Kind)
	}
	if got, _ := job.Payload["trigger"].(string); got != string(domain.TriggerSchedule) {
		t.Errorf("payload trigger %q, want SCHEDULE", got)
	}
	// The occurrence rather than the run: two workers that somehow claimed one moment would write
	// the same idempotency keys, and the second would act on nothing.
	want := "schedule:" + due.UTC().Format(time.RFC3339)
	if got, _ := job.Payload["occasion"].(string); got != want {
		t.Errorf("occasion %q, want %q", got, want)
	}

	// Measured from now rather than from the moment that had passed: one catch-up run, and then
	// forward. A rule that missed three nights fires once on Monday morning, not three times.
	if moved := store.moved[ruleID]; !moved.Equal(now.Add(24 * time.Hour)) {
		t.Errorf("moved to %v, want the next occurrence after now", moved)
	}
	if result.NextDue.IsZero() {
		t.Error("the pass answered no next moment, so the poller would finish")
	}
}

// A rule that is not due is not touched, and a tenant that owes nothing lets its poller finish -
// the whole reason a quiet tenant costs nothing.
func TestAPassWithNothingDueQueuesNothing(t *testing.T) {
	store := newSchedules(scheduledRule(now.Add(time.Hour)))
	pass, q := newPass(store, 24*time.Hour)

	result, err := pass.Run(context.Background(), tenantScope())
	if err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if result.Started != 0 || len(q.queued) != 0 {
		t.Errorf("started %d and queued %d for a rule that is not due", result.Started, len(q.queued))
	}
	if !result.NextDue.Equal(now.Add(time.Hour)) {
		t.Errorf("next due %v, want the rule's own moment", result.NextDue)
	}
}

// A rule whose recurrence this build cannot read is advanced to nothing rather than failing the
// pass. It was readable when it was written, so this is a rule the library changed under - and one
// tenant's unreadable rule must not stop every other rule in the same tenant from firing.
func TestAnUnreadableRuleStopsItselfRatherThanThePass(t *testing.T) {
	store := newSchedules(scheduledRule(now.Add(-time.Minute)))
	pass, q := newPass(store, 24*time.Hour)
	pass.Expander = expander{err: recurrence.ErrRuleUnreadable}

	result, err := pass.Run(context.Background(), tenantScope())
	if err != nil {
		t.Fatalf("the pass failed on one bad rule: %v", err)
	}
	if result.Started != 0 || len(q.queued) != 0 {
		t.Errorf("started %d and queued %d for a rule nothing can expand", result.Started, len(q.queued))
	}
	moved, advanced := store.moved[ruleID]
	if !advanced || !moved.IsZero() {
		t.Errorf("moved to %v (set: %v), want no moment at all", moved, advanced)
	}
}

// The batch is what turns a backlog - a worker that was down for a week - into several rounds
// rather than a hundred jobs in one transaction.
func TestOneRoundIsBounded(t *testing.T) {
	var rules []domain.Rule
	for i := range scheduleBatch + 10 {
		rule := scheduledRule(now.Add(-time.Minute))
		rule.ID = shared.ID("01936f2a-7c1e-7000-8000-0000000005" + string(rune('a'+i%26)) + "0")
		rules = append(rules, rule)
	}
	store := newSchedules(rules...)
	pass, q := newPass(store, 24*time.Hour)

	result, err := pass.Run(context.Background(), tenantScope())
	if err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if result.Started != scheduleBatch || len(q.queued) != scheduleBatch {
		t.Errorf("started %d and queued %d, want the batch of %d",
			result.Started, len(q.queued), scheduleBatch)
	}
}

// The seeding half of decision 5, and the half `multi-tenancy.md` §2.1 makes non-negotiable:
// nothing enumerates tenants, so the write that makes something owed is what starts the poller.
//
// A rule is written switched off, so the write that makes something owed is the *enable* - and the
// moment is recomputed from now rather than fired from wherever the rule was left, because "from
// now on, at three in the morning" is what somebody switching a rule on means.
func TestEnablingAScheduledRuleSeedsItsOwnTenantsPoller(t *testing.T) {
	rule := scheduledRule(time.Time{})
	rule.Enabled = false
	rule.NextRunAt = now.Add(-7 * 24 * time.Hour) // left behind while the rule was off
	h := newHarness(rule)
	store := newSchedules(rule)
	queued := &jobs{}
	h.writer.Schedules = store
	h.writer.Jobs = queued
	h.writer.Expander = expander{step: 24 * time.Hour}
	h.roleOf(writerID, identity.RoleOwner)

	enabled, err := EnableRule{Writer: h.writer}.Execute(context.Background(), writerActor(), ruleID)
	if err != nil {
		t.Fatalf("enabling: %v", err)
	}
	if !enabled.NextRunAt.Equal(now.Add(24 * time.Hour)) {
		t.Errorf("next run at %v, want the first occurrence after now rather than the missed week",
			enabled.NextRunAt)
	}
	if moved := store.moved[ruleID]; !moved.Equal(now.Add(24 * time.Hour)) {
		t.Errorf("stored moment %v, want the recomputed one", moved)
	}

	if len(queued.queued) != 1 {
		t.Fatalf("%d pollers seeded, want one", len(queued.queued))
	}
	job := queued.queued[0]
	if job.Kind != queue.KindAutomationSchedule {
		t.Errorf("job kind %q, want the schedule poller's", job.Kind)
	}
	if job.TenantID != tenant || job.DedupeKey != tenant.String() {
		t.Errorf("job for tenant %q keyed %q - one poller per tenant is the whole shape",
			job.TenantID, job.DedupeKey)
	}
	if !job.RunAt.Equal(now.Add(24 * time.Hour).UTC()) {
		t.Errorf("the poller wakes at %v, want the rule's own moment", job.RunAt)
	}
}

// Switching a rule off seeds nothing, and a rule of another kind seeds nothing either: a poller
// woken for a rule that does not act would wake up to find nothing due.
func TestNothingIsSeededForARuleThatOwesNothing(t *testing.T) {
	for _, name := range []string{"switched off", "not a schedule"} {
		t.Run(name, func(t *testing.T) {
			rule := scheduledRule(time.Time{})
			rule.Enabled = true
			if name == "not a schedule" {
				rule.Trigger = domain.Trigger{Kind: domain.TriggerManual}
			}
			h := newHarness(rule)
			queued := &jobs{}
			h.writer.Schedules, h.writer.Jobs = newSchedules(rule), queued
			h.writer.Expander = expander{step: 24 * time.Hour}
			h.roleOf(writerID, identity.RoleOwner)

			var err error
			if name == "switched off" {
				_, err = DisableRule{Writer: h.writer}.Execute(
					context.Background(), writerActor(), ruleID)
			} else {
				_, err = EnableRule{Writer: h.writer}.Execute(
					context.Background(), writerActor(), ruleID)
			}
			if err != nil {
				t.Fatalf("switching: %v", err)
			}
			if len(queued.queued) != 0 {
				t.Errorf("%d pollers seeded for a rule that owes nothing", len(queued.queued))
			}
		})
	}
}

// A recurrence this installation cannot read is refused to the person who wrote it, with the field
// it is about - not stored to fail at three in the morning (automation.md §2.2).
func TestAnUnreadableRecurrenceIsRefusedAtTheWrite(t *testing.T) {
	h := newHarness()
	h.writer.Expander = expander{err: recurrence.ErrRuleUnreadable}
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleOwner)

	cmd := validCommand()
	cmd.Trigger = domain.Trigger{
		Kind: domain.TriggerSchedule, RRule: "FREQ=NEVER", Timezone: "Europe/Berlin",
	}

	_, err := CreateRule{Writer: h.writer}.Execute(context.Background(), writerActor(), cmd)
	if err == nil {
		t.Fatal("an unreadable recurrence was stored")
	}
	fields := shared.AsError(err).Fields
	if len(fields) != 1 || fields[0].Path != "/trigger/rrule" {
		t.Errorf("fields %v, want the rule's own field named", fields)
	}
	if len(h.store.order) != 0 {
		t.Error("the rule was written anyway")
	}
}

// One catch-up run, not a burst. A worker that was down over a weekend leaves three missed nights
// on a nightly rule; advancing occurrence by occurrence would fire three runs on Monday morning,
// one after another, for a rule whose author asked for "every night at three".
func TestAMissedWeekFiresOnceAndThenGoesForward(t *testing.T) {
	store := newSchedules(scheduledRule(now.Add(-7 * 24 * time.Hour)))
	pass, q := newPass(store, 24*time.Hour)

	result, err := pass.Run(context.Background(), tenantScope())
	if err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if result.Started != 1 || len(q.queued) != 1 {
		t.Fatalf("started %d and queued %d, want exactly one catch-up run",
			result.Started, len(q.queued))
	}
	if moved := store.moved[ruleID]; !moved.After(now) {
		t.Errorf("moved to %v, which is still behind - the next round would fire again", moved)
	}
}
