// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/recurrence"
)

// scheduleStore is the schedules, as a table a test writes by hand.
type scheduleStore struct {
	stored  map[shared.ID]domain.Schedule
	written []domain.Schedule
	nextRun map[shared.ID]time.Time
}

func newSchedules() *scheduleStore {
	return &scheduleStore{stored: map[shared.ID]domain.Schedule{}, nextRun: map[shared.ID]time.Time{}}
}

func (s *scheduleStore) Insert(_ context.Context, schedule domain.Schedule, nextRunAt time.Time) error {
	s.stored[schedule.ID] = schedule
	s.written = append(s.written, schedule)
	s.nextRun[schedule.ID] = nextRunAt
	return nil
}

func (s *scheduleStore) List(context.Context) ([]domain.Schedule, error) {
	out := make([]domain.Schedule, 0, len(s.stored))
	for _, schedule := range s.stored {
		out = append(out, schedule)
	}
	return out, nil
}

func (s *scheduleStore) Find(_ context.Context, id shared.ID) (domain.Schedule, error) {
	schedule, found := s.stored[id]
	if !found {
		return domain.Schedule{}, shared.ErrNotFound.WithDetail(domain.CodeScheduleNotFound)
	}
	return schedule, nil
}

func (s *scheduleStore) Due(_ context.Context, at time.Time, _ int) ([]domain.Schedule, error) {
	var due []domain.Schedule
	for id, schedule := range s.stored {
		if moment, set := s.nextRun[id]; set && !moment.After(at) {
			due = append(due, schedule)
		}
	}
	return due, nil
}

func (s *scheduleStore) NextDue(context.Context) (time.Time, error) {
	var earliest time.Time
	for _, moment := range s.nextRun {
		if earliest.IsZero() || moment.Before(earliest) {
			earliest = moment
		}
	}
	return earliest, nil
}

func (s *scheduleStore) SetNextRun(_ context.Context, id shared.ID, at time.Time) error {
	s.nextRun[id] = at
	return nil
}

var _ repository.Schedules = (*scheduleStore)(nil)

func (h *harness) scheduling(schedules *scheduleStore, queued *jobs, rules *expander) Scheduling {
	return Scheduling{
		Schedules: schedules, Targets: h.targets, Jobs: queued, Expander: rules,
		Authorizer: h.authorizer, Audit: h.audit, UnitOfWork: h.uow,
		Clock: clock.Fixed(now), IDs: ids{next: scheduleID},
	}
}

func scheduleCommand() CreateBackupScheduleCommand {
	return CreateBackupScheduleCommand{
		TargetID: targetID, RRULE: "FREQ=DAILY;BYHOUR=3;BYMINUTE=0", TimeZone: "Europe/Berlin",
		Mode: domain.ModeIncremental, FullRRULE: "FREQ=WEEKLY;BYDAY=SU",
		IncludeMedia: true, IncludeAudit: true, Retention: domain.DefaultRetention(),
	}
}

func TestCreatingAScheduleDecidesWhenItFirstRuns(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	firstRun := now.Add(18 * time.Hour)
	schedules, queued, rules := newSchedules(), &jobs{}, &expander{moments: []time.Time{firstRun}}

	schedule, err := (CreateBackupSchedule{Scheduling: h.scheduling(schedules, queued, rules)}).
		Execute(context.Background(), caller(), scheduleCommand())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	switch {
	case schedule.ID != scheduleID:
		t.Fatalf("identity %s", schedule.ID)
	case !schedule.NextRunAt.Equal(firstRun):
		t.Fatalf("next run %v, want %v", schedule.NextRunAt, firstRun)
	case schedules.nextRun[scheduleID] != firstRun:
		t.Fatalf("the stored moment is %v", schedules.nextRun[scheduleID])
	}

	// The rule is counted from the schedule's creation, because a backup schedule has no due date.
	if len(rules.rules) != 1 || !rules.rules[0].Start.Equal(now) {
		t.Fatalf("the rule was anchored at %v, want the creation instant %v", rules.rules[0].Start, now)
	}
	if rules.rules[0].TimeZone != "Europe/Berlin" {
		t.Fatalf("the rule was read in %q", rules.rules[0].TimeZone)
	}
}

// The write that creates the work seeds the job for its own tenant: nothing in this system may
// enumerate tenants, so a scheduler cannot create one job per tenant even if it wanted to.
func TestCreatingAScheduleWakesThisTenantsPoller(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	firstRun := now.Add(18 * time.Hour)
	queued := &jobs{}

	if _, err := (CreateBackupSchedule{Scheduling: h.scheduling(newSchedules(), queued, &expander{moments: []time.Time{firstRun}})}).
		Execute(context.Background(), caller(), scheduleCommand()); err != nil {
		t.Fatalf("creating: %v", err)
	}

	if len(queued.requests) != 1 {
		t.Fatalf("%d jobs enqueued", len(queued.requests))
	}
	request := queued.requests[0]
	switch {
	case request.Kind != queue.KindBackupSchedule:
		t.Fatalf("kind %s", request.Kind)
	case request.TenantID != tenantID:
		t.Fatalf("tenant %s", request.TenantID)
	case request.DedupeKey != tenantID.String():
		t.Fatalf("dedupe key %q - one poller per tenant", request.DedupeKey)
	case !request.RunAt.Equal(firstRun):
		t.Fatalf("the wake-up is at %v", request.RunAt)
	}
}

// The owner's right, the same line creating a target needs: a schedule is the decision that the
// data will leave every night without anybody being asked again.
func TestCreatingAScheduleAsksForTheOwnersRight(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)

	if _, err := (CreateBackupSchedule{Scheduling: h.scheduling(newSchedules(), &jobs{}, &expander{moments: []time.Time{now.Add(time.Hour)}})}).
		Execute(context.Background(), caller(), scheduleCommand()); err != nil {
		t.Fatalf("creating: %v", err)
	}

	if h.authorizer.requests[0].Permission != domainservice.PermissionDeleteContainer {
		t.Fatalf("permission %s", h.authorizer.requests[0].Permission)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ScheduleChangedAction {
		t.Fatalf("audit: %+v", h.audit.entries)
	}
}

// A rule this installation cannot read is a field error on the rule, now - rather than a job that
// fails at three in the morning.
func TestARuleThisInstallationCannotReadIsRefusedAtCreation(t *testing.T) {
	for name, failure := range map[string]error{
		domain.CodeScheduleRuleUnreadable: recurrence.ErrRuleUnreadable,
		domain.CodeScheduleZoneUnknown:    recurrence.ErrZoneUnknown,
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness()
			enabledTarget(t, h)
			schedules := newSchedules()

			_, err := (CreateBackupSchedule{Scheduling: h.scheduling(schedules, &jobs{}, &expander{failure: failure})}).
				Execute(context.Background(), caller(), scheduleCommand())
			if err == nil {
				t.Fatal("an unreadable rule was accepted")
			}
			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != name {
				t.Fatalf("detail code: %v", err)
			}
			if len(schedules.written) != 0 {
				t.Fatal("the schedule was written anyway")
			}
		})
	}
}

// A rule that produces nothing in a year is stored with no next moment rather than refused: it may
// be perfectly good and simply exhausted, and a schedule an operator can see and edit is better
// than an error that loses what they typed.
func TestAScheduleWithNoNextMomentIsStoredWithNone(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	schedules, queued := newSchedules(), &jobs{}

	schedule, err := (CreateBackupSchedule{Scheduling: h.scheduling(schedules, queued, &expander{})}).
		Execute(context.Background(), caller(), scheduleCommand())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if !schedule.NextRunAt.IsZero() {
		t.Fatalf("next run %v", schedule.NextRunAt)
	}
	if len(queued.requests) != 0 {
		t.Fatal("a poller was woken for a schedule that owes nothing")
	}
}

func TestAScheduleForATargetNobodyHasIsNotFound(t *testing.T) {
	h := newHarness()

	_, err := (CreateBackupSchedule{Scheduling: h.scheduling(newSchedules(), &jobs{}, &expander{moments: []time.Time{now.Add(time.Hour)}})}).
		Execute(context.Background(), caller(), scheduleCommand())
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("creating against a target nobody has: %v", err)
	}
}

func TestARetentionPlanWithNoFloorIsRefused(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	cmd := scheduleCommand()
	cmd.Retention.MinKeep = 0

	_, err := (CreateBackupSchedule{Scheduling: h.scheduling(newSchedules(), &jobs{}, &expander{moments: []time.Time{now.Add(time.Hour)}})}).
		Execute(context.Background(), caller(), cmd)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a plan with no floor: %v", err)
	}
}

// A caller that names only min_keep means "the usual plan with a higher floor" rather than "keep
// nothing but the floor".
func TestAPartialRetentionPlanKeepsTheDefaults(t *testing.T) {
	plan := retentionFrom(map[string]any{"min_keep": float64(5)}, domain.DefaultRetention())

	if plan.MinKeep != 5 {
		t.Fatalf("min_keep %d", plan.MinKeep)
	}
	if plan.KeepDaily != domain.DefaultRetention().KeepDaily {
		t.Fatalf("keep_daily %d - the rest of the plan was lost", plan.KeepDaily)
	}
}

// The schedule's other two channels, through the descriptor: the retention plan and the scope come
// in as a nested document, and both come back out as one.
func TestTheScheduleDescriptorCarriesTheSameWorkAsExecute(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	firstRun := now.Add(18 * time.Hour)
	schedules := newSchedules()
	scheduling := h.scheduling(schedules, &jobs{}, &expander{moments: []time.Time{firstRun}})

	out, err := (CreateBackupSchedule{Scheduling: scheduling}).invoke(context.Background(), caller(),
		map[string]any{
			"target_id":  targetID.String(),
			"rrule":      "FREQ=DAILY;BYHOUR=3;BYMINUTE=0",
			"timezone":   "Europe/Berlin",
			"scope":      "TENANT",
			"mode":       "INCREMENTAL",
			"full_rrule": "FREQ=WEEKLY;BYDAY=SU",
			"notify_on":  []any{"FAILURE", "SUCCESS"},
			"retention":  map[string]any{"min_keep": float64(5)},
		})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}

	switch {
	case out.String("id") != scheduleID.String():
		t.Fatalf("id %q", out.String("id"))
	case out.String("rrule") != "FREQ=DAILY;BYHOUR=3;BYMINUTE=0":
		t.Fatalf("rrule %q", out.String("rrule"))
	case out.String("full_rrule") != "FREQ=WEEKLY;BYDAY=SU":
		t.Fatalf("full_rrule %q", out.String("full_rrule"))
	}

	plan, present := out["retention"].(map[string]any)
	if !present {
		t.Fatalf("no retention plan came back: %+v", out)
	}
	if plan["min_keep"] != 5 {
		t.Fatalf("min_keep %v", plan["min_keep"])
	}
	if plan["keep_daily"] != domain.DefaultRetention().KeepDaily {
		t.Fatalf("keep_daily %v - the rest of the plan was lost", plan["keep_daily"])
	}
	occasions, present := out["notify_on"].([]string)
	if !present || len(occasions) != 2 {
		t.Fatalf("notify_on: %v", out["notify_on"])
	}
	if _, named := out["next_run_at"]; !named {
		t.Fatalf("the answer does not say when it first runs: %+v", out)
	}

	scope, present := out["scope"].(map[string]any)
	if !present || scope["kind"] != string(domain.ScopeTenant) {
		t.Fatalf("scope: %v", out["scope"])
	}
	if scope["id"] != nil {
		t.Fatalf("a tenant schedule named a container: %v", scope["id"])
	}
}

// A container schedule names its container, and the answer carries it back.
func TestAContainerScheduleCarriesItsContainerBothWays(t *testing.T) {
	h := newHarness()
	enabledTarget(t, h)
	scheduling := h.scheduling(newSchedules(), &jobs{}, &expander{moments: []time.Time{now.Add(time.Hour)}})

	out, err := (CreateBackupSchedule{Scheduling: scheduling}).invoke(context.Background(), caller(),
		map[string]any{
			"target_id": targetID.String(), "rrule": "FREQ=DAILY", "timezone": "UTC",
			"scope": "HUB", "scope_id": targetID.String(),
		})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	scope, _ := out["scope"].(map[string]any)
	if scope["kind"] != string(domain.ScopeHub) || scope["id"] != targetID.String() {
		t.Fatalf("scope: %v", scope)
	}
}

// The descriptor is what the other two channels render, so it has to say enough to act on.
func TestTheScheduleDescriptorIsComplete(t *testing.T) {
	h := newHarness()
	descriptor := (CreateBackupSchedule{Scheduling: h.scheduling(newSchedules(), &jobs{}, &expander{})}).Descriptor()

	switch {
	case descriptor.Name != CreateBackupScheduleName:
		t.Fatalf("name %q", descriptor.Name)
	case descriptor.TokenScope != backupManage:
		t.Fatalf("token scope %q", descriptor.TokenScope)
	case !descriptor.Audit.Required || descriptor.Audit.Action != ScheduleChangedAction:
		t.Fatalf("audit: %+v", descriptor.Audit)
	case len(descriptor.Input) < 8:
		t.Fatalf("%d input fields - the plan and the rules are the whole of it", len(descriptor.Input))
	}
	for _, field := range descriptor.Input {
		if field.Description == "" {
			t.Errorf("the field %s has no description", field.Name)
		}
	}
}
