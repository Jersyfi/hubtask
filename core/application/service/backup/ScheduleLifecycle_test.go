// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/recurrence"
)

// seeded puts one schedule in the store, the way creating one would have, and answers the
// scheduling the lifecycle use cases run against.
func seeded(t *testing.T, h *harness, schedules *scheduleStore, moments ...time.Time) Scheduling {
	t.Helper()
	scheduling := h.scheduling(schedules, &jobs{}, &expander{moments: moments})

	stored, err := domain.NewSchedule(domain.NewScheduleInput{
		ID: scheduleID, TargetID: targetID, TenantID: tenantID, Scope: domain.ScopeTenant,
		RRULE: "FREQ=DAILY;BYHOUR=3;BYMINUTE=0", TimeZone: "Europe/Berlin",
		Mode: domain.ModeIncremental, IncludeMedia: true, IncludeAudit: true,
		Retention: domain.DefaultRetention(), Now: now,
	})
	if err != nil {
		t.Fatalf("the fixture schedule is not valid: %v", err)
	}
	schedules.stored[scheduleID] = stored
	return scheduling
}

// The read a backup screen opens with, and the permission pair that lets an auditor make it: a
// schedule is what a workspace has decided will leave it every night.
func TestListingSchedulesAcceptsTheAuditorsPermissionToo(t *testing.T) {
	h := newHarness()
	schedules := newSchedules()
	scheduling := seeded(t, h, schedules)

	listed, err := (ListBackupSchedules{Scheduling: scheduling}).Execute(t.Context(), caller())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != scheduleID {
		t.Fatalf("listed %v", listed)
	}

	request := h.authorizer.requests[0]
	if request.Permission != service.PermissionStructure {
		t.Errorf("asked for %q, want STRUCTURE", request.Permission)
	}
	if request.Alternative != service.PermissionReadConfiguration {
		t.Errorf("the alternative is %q, want READ_CONFIGURATION", request.Alternative)
	}
}

// Switching one off keeps everything somebody worked out about hours, zones and generations, and
// owes nothing at all - which is the whole reason a DELETE is not the only answer.
func TestSwitchingAScheduleOffKeepsTheRuleAndClearsTheMoment(t *testing.T) {
	h := newHarness()
	schedules := newSchedules()
	scheduling := seeded(t, h, schedules, now.Add(time.Hour))
	off := false

	changed, err := (UpdateBackupSchedule{Scheduling: scheduling}).Execute(t.Context(), caller(),
		UpdateBackupScheduleCommand{ID: scheduleID, Enabled: &off})
	if err != nil {
		t.Fatalf("switching off: %v", err)
	}
	if changed.Enabled {
		t.Error("the schedule came back switched on")
	}
	if !changed.NextRunAt.IsZero() {
		t.Errorf("a schedule that is off owes %v", changed.NextRunAt)
	}
	if changed.RRULE != "FREQ=DAILY;BYHOUR=3;BYMINUTE=0" || changed.TimeZone != "Europe/Berlin" {
		t.Errorf("the rule did not survive being switched off: %+v", changed)
	}
	if !schedules.nextRun[scheduleID].IsZero() {
		t.Error("the store still owes a moment for a schedule that is off")
	}
}

// Changing the rule recomputes the moment, here rather than on the next read.
func TestChangingTheRuleRecomputesWhenItIsNextOwed(t *testing.T) {
	h := newHarness()
	schedules := newSchedules()
	next := now.Add(9 * time.Hour)
	scheduling := seeded(t, h, schedules, next)
	rule := "FREQ=DAILY;BYHOUR=11;BYMINUTE=0"

	changed, err := (UpdateBackupSchedule{Scheduling: scheduling}).Execute(t.Context(), caller(),
		UpdateBackupScheduleCommand{ID: scheduleID, RRULE: &rule})
	if err != nil {
		t.Fatalf("changing: %v", err)
	}
	if changed.RRULE != rule {
		t.Errorf("rule %q", changed.RRULE)
	}
	if !changed.NextRunAt.Equal(next) {
		t.Errorf("next run %v, want %v", changed.NextRunAt, next)
	}

	// The trail carries the pair. "It ran at three and now it runs at eleven" is the finding, and
	// an entry with only the new value cannot produce it.
	if len(h.audit.entries) != 1 {
		t.Fatalf("%d audit entries", len(h.audit.entries))
	}
	change, held := h.audit.entries[0].Changes["rrule"].(map[string]any)
	if !held {
		t.Fatalf("the entry records %v", h.audit.entries[0].Changes)
	}
	if change["from"] != "FREQ=DAILY;BYHOUR=3;BYMINUTE=0" || change["to"] != rule {
		t.Errorf("the change is %v", change)
	}
}

// A rule this installation cannot read is refused as a field error on the rule, rather than
// stored to fail at three in the morning.
func TestAnUnreadableRuleIsRefusedAtTheChange(t *testing.T) {
	h := newHarness()
	schedules := newSchedules()
	scheduling := h.scheduling(schedules, &jobs{}, &expander{failure: recurrence.ErrRuleUnreadable})
	seededSchedule(t, schedules)
	rule := "NOT A RULE"

	_, err := (UpdateBackupSchedule{Scheduling: scheduling}).Execute(t.Context(), caller(),
		UpdateBackupScheduleCommand{ID: scheduleID, RRULE: &rule})

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error %v is not a domain error", err)
	}
	if domainErr.DetailCode != domain.CodeScheduleRuleUnreadable {
		t.Errorf("detail code %q", domainErr.DetailCode)
	}
	if len(h.audit.entries) != 0 {
		t.Error("a refused change was recorded as one that happened")
	}
}

// A stale version is a conflict rather than a silent overwrite (ADR-0025).
func TestAStaleVersionRefusesTheChange(t *testing.T) {
	h := newHarness()
	schedules := newSchedules()
	scheduling := seeded(t, h, schedules, now.Add(time.Hour))
	rule := "FREQ=WEEKLY"

	_, err := (UpdateBackupSchedule{Scheduling: scheduling}).Execute(t.Context(), caller(),
		UpdateBackupScheduleCommand{ID: scheduleID, RRULE: &rule, ExpectedVersion: 99})

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error %v is not a domain error", err)
	}
	if domainErr.DetailCode != domain.CodeScheduleVersionConflict {
		t.Errorf("detail code %q", domainErr.DetailCode)
	}
	if schedules.stored[scheduleID].RRULE == rule {
		t.Error("the row moved despite the conflict")
	}
}

// Removing one is recorded under its own code: "somebody edited the nightly backup" and "somebody
// removed it" are two different findings.
func TestRemovingAScheduleIsItsOwnEntry(t *testing.T) {
	h := newHarness()
	schedules := newSchedules()
	scheduling := seeded(t, h, schedules)

	if err := (DeleteBackupSchedule{Scheduling: scheduling}).
		Execute(t.Context(), caller(), scheduleID); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if len(schedules.removed) != 1 || schedules.removed[0] != scheduleID {
		t.Errorf("removed %v", schedules.removed)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ScheduleRemovedAction {
		t.Errorf("the trail says %v", h.audit.entries)
	}
}

// Removing one that is not there is not a failure: the caller asked for it to be gone and it is.
func TestRemovingAScheduleThatIsNotThereIsNotAnError(t *testing.T) {
	h := newHarness()
	schedules := newSchedules()
	scheduling := h.scheduling(schedules, &jobs{}, &expander{})

	if err := (DeleteBackupSchedule{Scheduling: scheduling}).
		Execute(t.Context(), caller(), scheduleID); err != nil {
		t.Fatalf("removing an absent schedule: %v", err)
	}
	if len(h.audit.entries) != 0 {
		t.Error("removing nothing was recorded as a removal")
	}
}

// seededSchedule is `seeded` without the scheduling, for the tests that build their own.
func seededSchedule(t *testing.T, schedules *scheduleStore) {
	t.Helper()
	stored, err := domain.NewSchedule(domain.NewScheduleInput{
		ID: scheduleID, TargetID: targetID, TenantID: tenantID, Scope: domain.ScopeTenant,
		RRULE: "FREQ=DAILY;BYHOUR=3;BYMINUTE=0", TimeZone: "Europe/Berlin",
		Mode: domain.ModeIncremental, IncludeMedia: true, IncludeAudit: true,
		Retention: domain.DefaultRetention(), Now: now,
	})
	if err != nil {
		t.Fatalf("the fixture schedule is not valid: %v", err)
	}
	schedules.stored[scheduleID] = stored
}

// The three round-trip through the registry, which is what makes them reachable over MCP and
// automation as well as REST - and what applies the descriptor's field validation.
func TestTheScheduleLifecycleRoundTripsThroughTheRegistry(t *testing.T) {
	h := newHarness()
	schedules := newSchedules()
	scheduling := seeded(t, h, schedules, now.Add(time.Hour))

	registry, err := usecase.NewRegistry(nil,
		ListBackupSchedules{Scheduling: scheduling}.Descriptor(),
		UpdateBackupSchedule{Scheduling: scheduling}.Descriptor(),
		DeleteBackupSchedule{Scheduling: scheduling}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}

	listed, err := registry.Invoke(t.Context(), ListBackupSchedulesName, caller(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows, _ := listed["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0].String("id") != scheduleID.String() {
		t.Fatalf("listed %v", listed)
	}

	changed, err := registry.Invoke(t.Context(), UpdateBackupScheduleName, caller(), usecase.Input{
		"schedule_id":   scheduleID.String(),
		"rrule":         "FREQ=DAILY;BYHOUR=11;BYMINUTE=0",
		"timezone":      "UTC",
		"mode":          "FULL",
		"include_media": false,
		"notify_on":     []any{"SUCCESS"},
		"retention":     map[string]any{"min_keep": float64(5)},
	})
	if err != nil {
		t.Fatalf("changing: %v", err)
	}
	if changed.String("rrule") != "FREQ=DAILY;BYHOUR=11;BYMINUTE=0" || changed.String("mode") != "FULL" {
		t.Errorf("the answer is %v", changed)
	}
	if media, _ := changed["include_media"].(bool); media {
		t.Error("include_media did not travel")
	}
	if plan, held := changed["retention"].(map[string]any); !held || plan["min_keep"] != 5 {
		t.Errorf("the plan is %v", changed["retention"])
	}

	if _, err := registry.Invoke(t.Context(), DeleteBackupScheduleName, caller(), usecase.Input{
		"schedule_id": scheduleID.String(),
	}); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if len(schedules.stored) != 0 {
		t.Errorf("the schedule is still there: %v", schedules.stored)
	}
}

// The registry refuses a field the descriptor does not declare - which is where the target and
// the scope are refused, because neither is an input of this use case.
func TestTheScopeCannotBeMovedThroughTheRegistry(t *testing.T) {
	h := newHarness()
	schedules := newSchedules()
	scheduling := seeded(t, h, schedules, now.Add(time.Hour))

	registry, err := usecase.NewRegistry(nil, UpdateBackupSchedule{Scheduling: scheduling}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}

	_, err = registry.Invoke(t.Context(), UpdateBackupScheduleName, caller(), usecase.Input{
		"schedule_id": scheduleID.String(), "target_id": targetID.String(),
	})
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error %v is not a domain error", err)
	}
	if len(domainErr.Fields) != 1 || domainErr.Fields[0].Path != "/target_id" {
		t.Errorf("the refusal points at %v", domainErr.Fields)
	}
}
