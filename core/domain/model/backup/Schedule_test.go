// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	scheduleTenant = shared.MustParseID("0198f0a0-0000-7000-8000-0000000000aa")
	scheduleTarget = shared.MustParseID("0198f0a0-0000-7000-8000-0000000000bb")
	scheduleID     = shared.MustParseID("0198f0a0-0000-7000-8000-0000000000cc")
	scheduleNow    = time.Date(2026, 8, 26, 14, 30, 0, 0, time.UTC)
)

func scheduleInput() domain.NewScheduleInput {
	return domain.NewScheduleInput{
		ID: scheduleID, TargetID: scheduleTarget, TenantID: scheduleTenant,
		Scope: domain.ScopeTenant, RRULE: "FREQ=DAILY;BYHOUR=3;BYMINUTE=0",
		TimeZone: "Europe/Berlin", Mode: domain.ModeIncremental,
		FullRRULE: "FREQ=WEEKLY;BYDAY=SU", IncludeMedia: true, IncludeAudit: true,
		Retention: domain.DefaultRetention(), Now: scheduleNow,
	}
}

func fieldCodes(t *testing.T, err error) []string {
	t.Helper()
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("not a typed error: %v", err)
	}
	var codes []string
	for _, f := range domainErr.Fields {
		codes = append(codes, f.Code)
	}
	return codes
}

func TestAScheduleIsBuiltFromWhatACallerSupplies(t *testing.T) {
	schedule, err := domain.NewSchedule(scheduleInput())
	if err != nil {
		t.Fatalf("building: %v", err)
	}

	switch {
	case schedule.ID != scheduleID || schedule.TargetID != scheduleTarget:
		t.Fatalf("identity: %+v", schedule)
	case !schedule.Enabled || schedule.Version != 1:
		t.Fatalf("enabled=%v version=%d", schedule.Enabled, schedule.Version)
	case !schedule.CreatedAt.Equal(scheduleNow):
		t.Fatalf("created at %v", schedule.CreatedAt)
	case len(schedule.NotifyOn) != 1 || schedule.NotifyOn[0] != domain.NotifyFailure:
		t.Fatalf("a schedule that named no occasion did not get the default: %v", schedule.NotifyOn)
	}
}

func TestAModeThatWasNotNamedIsIncremental(t *testing.T) {
	in := scheduleInput()
	in.Mode = ""

	schedule, err := domain.NewSchedule(in)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if schedule.Mode != domain.ModeIncremental {
		t.Fatalf("mode %q - a schedule is incremental unless it says otherwise", schedule.Mode)
	}
}

// The pairing `0001_init`'s check constraint makes, refused here so that a person configuring a
// backup gets a field error rather than a database error.
func TestAnInstanceScheduleHasNoTenantAndEveryOtherOneDoes(t *testing.T) {
	withTenant := scheduleInput()
	withTenant.Scope = domain.ScopeInstance

	_, err := domain.NewSchedule(withTenant)
	if err == nil {
		t.Fatal("an instance schedule with a tenant was accepted")
	}
	if !slices.Contains(fieldCodes(t, err), domain.CodeScheduleInstanceHasNoTenant) {
		t.Fatalf("codes: %v", fieldCodes(t, err))
	}

	withoutTenant := scheduleInput()
	withoutTenant.TenantID = ""
	if _, err := domain.NewSchedule(withoutTenant); err == nil {
		t.Fatal("a tenant schedule with no tenant was accepted")
	} else if !slices.Contains(fieldCodes(t, err), domain.CodeScheduleTenantRequired) {
		t.Fatalf("codes: %v", fieldCodes(t, err))
	}

	instance := scheduleInput()
	instance.Scope, instance.TenantID = domain.ScopeInstance, ""
	if _, err := domain.NewSchedule(instance); err != nil {
		t.Fatalf("an instance schedule with no tenant was refused: %v", err)
	}
}

func TestAContainerScopeNamesItsContainer(t *testing.T) {
	in := scheduleInput()
	in.Scope = domain.ScopeHub

	_, err := domain.NewSchedule(in)
	if err == nil {
		t.Fatal("a hub schedule naming no hub was accepted")
	}
	if !slices.Contains(fieldCodes(t, err), domain.CodeScheduleScopeIDRequired) {
		t.Fatalf("codes: %v", fieldCodes(t, err))
	}
}

func TestWhatMakesAScheduleUnusable(t *testing.T) {
	cases := map[string]func(*domain.NewScheduleInput){
		domain.CodeScheduleTargetRequired:      func(in *domain.NewScheduleInput) { in.TargetID = "" },
		domain.CodeScheduleScopeInvalid:        func(in *domain.NewScheduleInput) { in.Scope = "SOMETIMES" },
		domain.CodeScheduleRuleRequired:        func(in *domain.NewScheduleInput) { in.RRULE = "   " },
		domain.CodeScheduleTimeZoneRequired:    func(in *domain.NewScheduleInput) { in.TimeZone = "" },
		domain.CodeScheduleModeInvalid:         func(in *domain.NewScheduleInput) { in.Mode = "PARTIAL" },
		domain.CodeScheduleNotificationInvalid: func(in *domain.NewScheduleInput) { in.NotifyOn = []domain.Notification{"WHENEVER"} },
	}

	for code, breakIt := range cases {
		t.Run(code, func(t *testing.T) {
			in := scheduleInput()
			breakIt(&in)

			_, err := domain.NewSchedule(in)
			if err == nil {
				t.Fatalf("%s was accepted", code)
			}
			if !slices.Contains(fieldCodes(t, err), code) {
				t.Fatalf("codes: %v", fieldCodes(t, err))
			}
		})
	}
}

// The floor is the one number that may not be zero: a plan with no floor is a plan that can arrive
// at "keep nothing".
func TestARetentionPlanAlwaysKeepsSomething(t *testing.T) {
	in := scheduleInput()
	in.Retention.MinKeep = 0

	_, err := domain.NewSchedule(in)
	if err == nil {
		t.Fatal("a plan with no floor was accepted")
	}
	if !slices.Contains(fieldCodes(t, err), domain.CodeRetentionFloorRequired) {
		t.Fatalf("codes: %v", fieldCodes(t, err))
	}

	negative := scheduleInput()
	negative.Retention.KeepMonthly = -1
	if _, err := domain.NewSchedule(negative); err == nil {
		t.Fatal("a negative generation was accepted")
	} else if !slices.Contains(fieldCodes(t, err), domain.CodeRetentionNegative) {
		t.Fatalf("codes: %v", fieldCodes(t, err))
	}
}

func TestTheDefaultPlanIsTheOneTheSchemaWrites(t *testing.T) {
	plan := domain.DefaultRetention()

	if plan != (domain.Retention{KeepLast: 7, KeepDaily: 14, KeepWeekly: 8, KeepMonthly: 12, KeepYearly: 3, MinKeep: 3}) {
		t.Fatalf("the default plan drifted from 0001_init and the contract: %+v", plan)
	}
}

// The anchor is the creation instant, because a backup schedule has no due date to count from.
func TestTheAnchorIsWhenTheScheduleWasCreated(t *testing.T) {
	schedule, err := domain.NewSchedule(scheduleInput())
	if err != nil {
		t.Fatalf("building: %v", err)
	}

	if !schedule.Anchor().Equal(scheduleNow) {
		t.Fatalf("anchored at %v, want the creation instant %v", schedule.Anchor(), scheduleNow)
	}
}

// full_rrule selects among rrule's occurrences rather than producing occurrences of its own, and
// the comparison is by calendar day in the schedule's own zone.
func TestFullEveryPromotesAnOccurrenceRatherThanAddingOne(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("the zone: %v", err)
	}
	schedule, err := domain.NewSchedule(scheduleInput())
	if err != nil {
		t.Fatalf("building: %v", err)
	}

	sunday := time.Date(2026, 8, 30, 3, 0, 0, 0, berlin)
	monday := time.Date(2026, 8, 31, 3, 0, 0, 0, berlin)
	// The full rule names a day and not an hour, which is exactly why it may not be expanded on
	// its own: here it lands at noon, and the run it promotes is the three o'clock one.
	fullDays := []time.Time{time.Date(2026, 8, 30, 12, 0, 0, 0, berlin)}

	if !schedule.IsFullOn(sunday, fullDays, berlin) {
		t.Fatal("the Sunday run was not promoted to a full one")
	}
	if schedule.IsFullOn(monday, fullDays, berlin) {
		t.Fatal("the Monday run was promoted")
	}
}

// A schedule that is FULL outright needs no promotion, and one with no full rule never gets any.
func TestAFullScheduleIsAlwaysFullAndAnIncrementalOneWithoutARuleNeverIs(t *testing.T) {
	utc := time.UTC
	always := scheduleInput()
	always.Mode = domain.ModeFull
	full, err := domain.NewSchedule(always)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if !full.IsFullOn(scheduleNow, nil, utc) {
		t.Fatal("a FULL schedule produced an incremental")
	}

	never := scheduleInput()
	never.FullRRULE = ""
	plain, err := domain.NewSchedule(never)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if plain.IsFullOn(scheduleNow, []time.Time{scheduleNow}, utc) {
		t.Fatal("a schedule with no full rule was promoted anyway")
	}
}

// The day is the day in the schedule's zone. An occurrence at 03:00 in Berlin is the previous day
// in UTC, and reading it in UTC would promote the wrong run.
func TestThePromotionIsReadInTheSchedulesOwnZone(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("the zone: %v", err)
	}
	schedule, err := domain.NewSchedule(scheduleInput())
	if err != nil {
		t.Fatalf("building: %v", err)
	}

	// 01:00 Berlin on Sunday is 23:00 UTC on Saturday.
	occurrence := time.Date(2026, 8, 30, 1, 0, 0, 0, berlin)
	fullDays := []time.Time{time.Date(2026, 8, 30, 12, 0, 0, 0, berlin)}

	if !schedule.IsFullOn(occurrence, fullDays, berlin) {
		t.Fatal("a Sunday run in Berlin was read as a Saturday run")
	}
	if schedule.IsFullOn(occurrence, fullDays, time.UTC) {
		t.Fatal("reading the day in UTC promoted the wrong run - the zone is the schedule's")
	}
}

// The one shape a scheduler may look for by itself.
func TestOnlyAnInstanceScheduleIsInstanceWide(t *testing.T) {
	in := scheduleInput()
	in.Scope, in.TenantID = domain.ScopeInstance, ""
	instance, err := domain.NewSchedule(in)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	tenant, err := domain.NewSchedule(scheduleInput())
	if err != nil {
		t.Fatalf("building: %v", err)
	}

	if !instance.IsInstanceWide() || tenant.IsInstanceWide() {
		t.Fatalf("instance=%v tenant=%v", instance.IsInstanceWide(), tenant.IsInstanceWide())
	}
}

func TestTheScopeEnumIsTheOneTheSchemaAllows(t *testing.T) {
	got := domain.ScheduleScopes()
	want := []domain.ScheduleScope{domain.ScopeInstance, domain.ScopeTenant, domain.ScopeHub, domain.ScopeCollection}

	if !slices.Equal(got, want) {
		t.Fatalf("the scopes drifted from 0001_init's check constraint: %v", got)
	}
	for _, scope := range want {
		if !scope.Valid() {
			t.Fatalf("%s is not valid", scope)
		}
	}
	if domain.ScheduleScope("EVERYTHING").Valid() {
		t.Fatal("a scope nobody defined is valid")
	}
}
