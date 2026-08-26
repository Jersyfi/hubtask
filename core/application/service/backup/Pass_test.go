// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

func passFor(h *harness, schedules *scheduleStore, runs *runStore, queued *jobs, rules *expander) SchedulePass {
	return SchedulePass{
		Schedules: schedules, Runs: runs, Jobs: queued, Expander: rules,
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: ids{next: runID},
	}
}

// scheduledAt puts one schedule on the shelf, due at a moment.
func scheduledAt(t *testing.T, schedules *scheduleStore, due time.Time, mode domain.Mode, fullRule string) domain.Schedule {
	t.Helper()

	schedule, err := domain.NewSchedule(domain.NewScheduleInput{
		ID: scheduleID, TargetID: targetID, TenantID: tenantID, Scope: domain.ScopeTenant,
		RRULE: "FREQ=DAILY;BYHOUR=3;BYMINUTE=0", TimeZone: "Europe/Berlin",
		Mode: mode, FullRRULE: fullRule, Retention: domain.DefaultRetention(), Now: now,
	})
	if err != nil {
		t.Fatalf("building the schedule: %v", err)
	}
	schedule.NextRunAt = due
	schedules.stored[schedule.ID] = schedule
	schedules.nextRun[schedule.ID] = due
	return schedule
}

func TestAScheduleWhoseMomentHasComeBecomesAJob(t *testing.T) {
	h := newHarness()
	schedules, runs, queued := newSchedules(), newRuns(), &jobs{}
	due := now.Add(-time.Minute)
	scheduledAt(t, schedules, due, domain.ModeIncremental, "")
	tomorrow := now.Add(24 * time.Hour)
	rules := &expander{moments: []time.Time{tomorrow}}

	result, err := passFor(h, schedules, runs, queued, rules).
		Run(t.Context(), persistence.Scope{TenantID: tenantID})
	if err != nil {
		t.Fatalf("the pass: %v", err)
	}

	if result.Started != 1 {
		t.Fatalf("%d runs started", result.Started)
	}
	if len(queued.requests) != 1 || queued.requests[0].Kind != queue.KindBackupRun {
		t.Fatalf("jobs: %+v", queued.requests)
	}
	if queued.requests[0].Payload["schedule_id"] != scheduleID.String() {
		t.Fatalf("the job does not name its schedule: %v", queued.requests[0].Payload)
	}
	// And the schedule has moved on, so the next round does not fire it again.
	if !schedules.nextRun[scheduleID].Equal(tomorrow) {
		t.Fatalf("the schedule is still at %v", schedules.nextRun[scheduleID])
	}
	if !result.NextDue.Equal(tomorrow) {
		t.Fatalf("the pass reports the next moment as %v", result.NextDue)
	}
}

// The first run of an incremental schedule is a full one. Promoting it is right here where it would
// be wrong for a caller who asked by hand: nobody is waiting for an answer, and the alternative is
// a schedule that never produces anything.
func TestTheFirstRunOfAnIncrementalScheduleIsAFullOne(t *testing.T) {
	h := newHarness()
	schedules, runs, queued := newSchedules(), newRuns(), &jobs{}
	scheduledAt(t, schedules, now.Add(-time.Minute), domain.ModeIncremental, "")

	if _, err := passFor(h, schedules, runs, queued, &expander{moments: []time.Time{now.Add(24 * time.Hour)}}).
		Run(t.Context(), persistence.Scope{TenantID: tenantID}); err != nil {
		t.Fatalf("the pass: %v", err)
	}

	if queued.requests[0].Payload["mode"] != string(domain.ModeFull) {
		t.Fatalf("the first run is %v, want FULL", queued.requests[0].Payload["mode"])
	}
}

func TestAnIncrementalScheduleWithAParentStaysIncremental(t *testing.T) {
	h := newHarness()
	schedules, runs, queued := newSchedules(), newRuns(), &jobs{}
	scheduledAt(t, schedules, now.Add(-time.Minute), domain.ModeIncremental, "")
	parent := domain.Run{ID: jobID, TargetID: targetID, Status: domain.RunSucceeded}
	runs.latest = &parent

	if _, err := passFor(h, schedules, runs, queued, &expander{moments: []time.Time{now.Add(24 * time.Hour)}}).
		Run(t.Context(), persistence.Scope{TenantID: tenantID}); err != nil {
		t.Fatalf("the pass: %v", err)
	}

	if queued.requests[0].Payload["mode"] != string(domain.ModeIncremental) {
		t.Fatalf("mode %v", queued.requests[0].Payload["mode"])
	}
	if queued.requests[0].Payload["parent_run_id"] != parent.ID.String() {
		t.Fatalf("parent %v", queued.requests[0].Payload["parent_run_id"])
	}
}

// full_rrule promotes one of the rule's own occurrences rather than adding one. The pass asks for
// the day around the occurrence, in the schedule's own zone.
func TestTheFullRuleTurnsOneOccurrenceIntoAFullRun(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("the zone: %v", err)
	}
	h := newHarness()
	schedules, runs, queued := newSchedules(), newRuns(), &jobs{}
	sunday := time.Date(2026, 8, 30, 3, 0, 0, 0, berlin)
	scheduledAt(t, schedules, sunday, domain.ModeIncremental, "FREQ=WEEKLY;BYDAY=SU")
	runs.latest = &domain.Run{ID: jobID, TargetID: targetID, Status: domain.RunSucceeded}

	// The full rule lands at noon on the same Berlin day - which is exactly why it may not be
	// expanded on its own, and exactly why comparing by day works.
	rules := &expander{moments: []time.Time{
		time.Date(2026, 8, 30, 12, 0, 0, 0, berlin),
		time.Date(2026, 8, 31, 3, 0, 0, 0, berlin),
	}}
	pass := passFor(h, schedules, runs, queued, rules)
	pass.Clock = clock.Fixed(sunday.Add(time.Minute))

	if _, err := pass.Run(t.Context(), persistence.Scope{TenantID: tenantID}); err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if queued.requests[0].Payload["mode"] != string(domain.ModeFull) {
		t.Fatalf("the Sunday run is %v, want FULL", queued.requests[0].Payload["mode"])
	}
}

// A schedule is advanced whether or not the run could be enqueued: one that stayed on a moment it
// could not act on would try again on every round for ever.
func TestAScheduleMovesOnEvenWhenTheQueueRefuses(t *testing.T) {
	h := newHarness()
	schedules, runs := newSchedules(), newRuns()
	queued := &jobs{failure: shared.ErrUnavailable.WithDetail("postgres.query_failed")}
	scheduledAt(t, schedules, now.Add(-time.Minute), domain.ModeFull, "")
	tomorrow := now.Add(24 * time.Hour)

	_, err := passFor(h, schedules, runs, queued, &expander{moments: []time.Time{tomorrow}}).
		Run(t.Context(), persistence.Scope{TenantID: tenantID})
	if err == nil {
		t.Fatal("a queue that refused was not reported")
	}
	if !schedules.nextRun[scheduleID].Equal(tomorrow) {
		t.Fatalf("the schedule is still at %v - it would fire for ever", schedules.nextRun[scheduleID])
	}
}

// A scope that owes nothing lets its poller finish. The next write re-seeds it, which is what keeps
// a quiet tenant costing nothing.
func TestAPassWithNothingDueReportsNoNextMoment(t *testing.T) {
	h := newHarness()

	result, err := passFor(h, newSchedules(), newRuns(), &jobs{}, &expander{}).
		Run(t.Context(), persistence.Scope{TenantID: tenantID})
	if err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if result.Started != 0 || !result.NextDue.IsZero() {
		t.Fatalf("result: %+v", result)
	}
}

// A rule that has run out leaves the schedule with no next moment rather than stuck on the last one.
func TestAScheduleThatHasRunOutStopsRatherThanRepeating(t *testing.T) {
	h := newHarness()
	schedules, runs, queued := newSchedules(), newRuns(), &jobs{}
	scheduledAt(t, schedules, now.Add(-time.Minute), domain.ModeFull, "")

	if _, err := passFor(h, schedules, runs, queued, &expander{}).
		Run(t.Context(), persistence.Scope{TenantID: tenantID}); err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if !schedules.nextRun[scheduleID].IsZero() {
		t.Fatalf("the schedule is still due at %v", schedules.nextRun[scheduleID])
	}
	if len(queued.requests) != 1 {
		t.Fatal("the last occurrence did not fire")
	}
}

// The plan an expiry pass needs, and the only thing it needs.
func TestThePlanOfAScheduleIsReadBack(t *testing.T) {
	h := newHarness()
	schedules := newSchedules()
	scheduledAt(t, schedules, now, domain.ModeIncremental, "")

	plan, zone, err := SchedulePlans{Schedules: schedules, UnitOfWork: h.uow}.
		Plan(t.Context(), tenantID, scheduleID)
	if err != nil {
		t.Fatalf("reading the plan: %v", err)
	}
	if plan != domain.DefaultRetention() {
		t.Fatalf("plan: %+v", plan)
	}
	if zone != "Europe/Berlin" {
		t.Fatalf("zone %q - the generations are counted where the operator is", zone)
	}
}
