// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	person   = shared.MustParseID("0192f000-0000-7000-8000-000000000a01")
	lead     = shared.MustParseID("0192f000-0000-7000-8000-000000000a02")
	other    = shared.MustParseID("0192f000-0000-7000-8000-000000000a03")
	third    = shared.MustParseID("0192f000-0000-7000-8000-000000000a04")
	recordID = shared.MustParseID("0192f000-0000-7000-8000-000000000a05")
	entryA   = shared.MustParseID("0192f000-0000-7000-8000-000000000a06")
	entryB   = shared.MustParseID("0192f000-0000-7000-8000-000000000a07")
	entryC   = shared.MustParseID("0192f000-0000-7000-8000-000000000a08")
)

func day(t *testing.T, text string) Day {
	t.Helper()
	d, ok := ParseDay(text)
	if !ok {
		t.Fatalf("day %q", text)
	}
	return d
}

func code(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

// UC-TIM-01 check 1: a record is between a minute and a day, a correction signed and naming
// what it corrects, a note bounded.
func TestARecordIsHeldToItsBounds(t *testing.T) {
	base := Record{ID: recordID, AccountID: person, Minutes: 90, Source: SourceManual}
	cases := []struct {
		name string
		edit func(r *Record)
		want string
	}{
		{"ninety minutes", func(*Record) {}, ""},
		{"zero minutes", func(r *Record) { r.Minutes = 0 }, "time.minutes_out_of_range"},
		{"more than a day", func(r *Record) { r.Minutes = MaxMinutes + 1 }, "time.minutes_out_of_range"},
		{"negative by hand", func(r *Record) { r.Minutes = -30 }, "time.minutes_out_of_range"},
		{"a correction back", func(r *Record) { r.Source, r.Corrects, r.Minutes = SourceCorrection, entryA, -30 }, ""},
		{"a correction of nothing", func(r *Record) { r.Source, r.Minutes = SourceCorrection, -30 }, "time.correction_target_required"},
		{"a correction of zero", func(r *Record) { r.Source, r.Corrects, r.Minutes = SourceCorrection, entryA, 0 }, "time.minutes_out_of_range"},
		{"a long note", func(r *Record) { r.Note = string(make([]rune, MaxNoteLength+1)) }, "time.note_too_long"},
		{"a control character", func(r *Record) { r.Note = "a\x01b" }, "time.note_invalid"},
		{"two lines", func(r *Record) { r.Note = "a\nb" }, ""},
		{"no source", func(r *Record) { r.Source = "" }, "time.source_unknown"},
	}
	for _, c := range cases {
		r := base
		c.edit(&r)
		if got := code(r.Check()); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestASpanRoundsUpToTheStartedMinute(t *testing.T) {
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	if m, err := Span(start, start.Add(61*time.Second)); err != nil || m != 2 {
		t.Fatalf("61 s: %d %v", m, err)
	}
	if _, err := Span(start, start); code(err) != "time.ends_before_start" {
		t.Fatalf("no time: %v", err)
	}
	if _, err := Span(start, start.Add(25*time.Hour)); code(err) != "time.minutes_out_of_range" {
		t.Fatalf("a day and more: %v", err)
	}
}

func TestAWeekStartsOnMonday(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"2026-11-02", "2026-11-02"}, {"2026-11-08", "2026-11-02"}, {"2026-11-04", "2026-11-02"},
		{"1970-01-01", "1969-12-29"}, {"1969-12-28", "1969-12-22"},
	} {
		if got := WeekOf(day(t, c.in)).String(); got != c.want {
			t.Errorf("%s: %s, want %s", c.in, got, c.want)
		}
	}
	if got := MonthOf(day(t, "2026-11-19")).String(); got != "2026-11-01" {
		t.Fatalf("month: %s", got)
	}
}

// UC-TIM-02 checks 3 and 4, UC-TIM-03 checks 1 and 2: a week is handed in with its time, taken
// back before a decision, approved or sent back with a reason by somebody else.
func TestAWeekIsHandedInAndDecided(t *testing.T) {
	now := time.Date(2026, 11, 6, 17, 0, 0, 0, time.UTC)
	sheet := OpenSheet(person, day(t, "2026-11-04"))
	if sheet.WeekStart.String() != "2026-11-02" {
		t.Fatalf("week %s", sheet.WeekStart)
	}
	if _, err := sheet.Submit(0, now); code(err) != "timesheets.empty" {
		t.Fatalf("empty: %v", err)
	}
	submitted, err := sheet.Submit(2400, now)
	if err != nil || submitted.State != SheetSubmitted || submitted.Minutes != 2400 {
		t.Fatalf("submit: %+v %v", submitted, err)
	}
	if _, err := submitted.Submit(2400, now); code(err) != "timesheets.not_open" {
		t.Fatalf("twice: %v", err)
	}
	if back, err := submitted.Withdraw(); err != nil || back.State != SheetOpen {
		t.Fatalf("withdraw: %+v %v", back, err)
	}
	if _, err := submitted.Approve(person, now); code(err) != "timesheets.own_week" {
		t.Fatalf("own: %v", err)
	}
	if _, err := submitted.Reject(lead, "  ", now); code(err) != "timesheets.reason_required" {
		t.Fatalf("no reason: %v", err)
	}
	rejected, err := submitted.Reject(lead, "Tuesday is missing", now)
	if err != nil || rejected.State != SheetRejected || rejected.DecidedBy != lead {
		t.Fatalf("reject: %+v %v", rejected, err)
	}
	if !rejected.State.Writable() {
		t.Fatal("a sent-back week is not writable")
	}
	again, err := rejected.Submit(2460, now)
	if err != nil || again.Reason != "" {
		t.Fatalf("again: %+v %v", again, err)
	}
	approved, err := again.Approve(lead, now)
	if err != nil || approved.State != SheetApproved || approved.State.Writable() {
		t.Fatalf("approve: %+v %v", approved, err)
	}
	if _, err := approved.Withdraw(); code(err) != "timesheets.not_submitted" {
		t.Fatalf("withdraw approved: %v", err)
	}
}

// UC-TIM-03 check 4: the locked period and a handed-in week refuse a change.
func TestTheLockedPeriodAndAHandedInWeekRefuseAChange(t *testing.T) {
	locked := day(t, "2026-10-31")
	policy := Policy{LockedThrough: &locked, Visibility: VisibilityFull}
	if err := policy.MayWrite(day(t, "2026-10-31"), SheetOpen); code(err) != "time.period_locked" {
		t.Fatalf("locked day: %v", err)
	}
	if err := policy.MayWrite(day(t, "2026-11-01"), ""); err != nil {
		t.Fatalf("open day: %v", err)
	}
	for _, s := range []SheetState{SheetSubmitted, SheetApproved} {
		if err := policy.MayWrite(day(t, "2026-11-03"), s); code(err) != "time.week_not_open" {
			t.Fatalf("%s: %v", s, err)
		}
	}
	if err := DefaultPolicy().MayWrite(day(t, "2000-01-01"), SheetRejected); err != nil {
		t.Fatalf("default: %v", err)
	}
}

// UC-TIM-01 check 3: a stopped timer is a record of the day it started, in the person's zone.
func TestATimerStopsIntoARecord(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no zone database")
	}
	start := time.Date(2026, 11, 2, 23, 30, 0, 0, time.UTC) // already the 3rd in Berlin
	timer := Timer{AccountID: person, EntryID: entryA, WorkType: "development", Billable: true, StartedAt: start}
	r, err := timer.Stop(recordID, start.Add(45*time.Minute), berlin)
	if err != nil {
		t.Fatal(err)
	}
	if r.Day.String() != "2026-11-03" || r.Minutes != 45 || r.Source != SourceTimer || !r.Billable {
		t.Fatalf("record %+v", r)
	}
	if _, err := timer.Stop(recordID, start.Add(25*time.Hour), berlin); code(err) != "timers.ran_too_long" {
		t.Fatalf("too long: %v", err)
	}
}

func TestAWorkTypeIsHeldToItsBounds(t *testing.T) {
	if err := CheckWorkType(WorkType{Key: "development", Name: "Development"}); err != nil {
		t.Fatal(err)
	}
	if code(CheckWorkType(WorkType{Key: "Dev", Name: "x"})) != "work_types.key_invalid" {
		t.Fatal("an upper-case key passed")
	}
	if code(CheckWorkType(WorkType{Key: "dev", Name: " "})) != "work_types.name_required" {
		t.Fatal("a blank name passed")
	}
}

// UC-TIM-04 checks 1 and 3: a report sums by up to two dimensions; aggregated, it withholds a
// group smaller than three people unless it is the reader's own time.
func TestAReportSumsAndWithholdsSmallGroups(t *testing.T) {
	d := day(t, "2026-11-03")
	lines := []Line{
		{AccountID: person, EntryID: entryA, WorkType: "dev", Day: d, Billable: true, Minutes: 60, Records: 1},
		{AccountID: lead, EntryID: entryA, WorkType: "dev", Day: d, Minutes: 30, Records: 1},
		{AccountID: other, EntryID: entryA, WorkType: "meeting", Day: d + 1, Minutes: 15, Records: 1},
		{AccountID: other, EntryID: entryB, WorkType: "dev", Day: d, Minutes: 120, Records: 2},
	}
	if err := CheckDimensions([]Dimension{ByEntry, ByEntry}); code(err) != "time.dimension_twice" {
		t.Fatalf("twice: %v", err)
	}
	if err := CheckDimensions([]Dimension{ByEntry, ByDay, ByHub}); code(err) != "time.too_many_dimensions" {
		t.Fatalf("three: %v", err)
	}
	full := Summarise(lines, []Dimension{ByEntry}, false, person)
	if len(full.Groups) != 2 || full.Groups[0].Minutes+full.Groups[1].Minutes != 225 || full.Minutes != 225 {
		t.Fatalf("full %+v", full)
	}
	var a Group
	for _, g := range full.Groups {
		if g.Keys[0] == entryA.String() {
			a = g
		}
	}
	if a.Minutes != 105 || a.BillableMinutes != 60 || a.Records != 3 {
		t.Fatalf("entry A %+v", a)
	}
	aggregated := Summarise(lines, []Dimension{ByEntry}, true, person)
	if len(aggregated.Groups) != 1 || aggregated.Groups[0].Keys[0] != entryA.String() ||
		aggregated.WithheldMinutes != 120 || aggregated.WithheldGroups != 1 {
		t.Fatalf("aggregated %+v", aggregated)
	}
	own := Summarise(lines[:1], []Dimension{ByDay}, true, person)
	if len(own.Groups) != 1 || own.WithheldGroups != 0 {
		t.Fatalf("own %+v", own)
	}
	week := Summarise(lines, []Dimension{ByWeek, ByBillable}, false, person)
	if len(week.Groups) != 2 || week.Groups[0].Keys[0] != "2026-11-02" {
		t.Fatalf("week %+v", week)
	}
}

// UC-TIM-04 check 2: an entry's estimate against its time, rolled up its parents.
func TestActualsRollUpThePlan(t *testing.T) {
	eight, two := 8.0, 2.0
	entries := []Planned{
		{ID: entryA, Hours: &eight},
		{ID: entryB, ParentID: entryA, Hours: &two},
		{ID: entryC, ParentID: entryB},
	}
	got := Actuals(entries, map[shared.ID]int{entryA: 60, entryB: 90, entryC: 300})
	if got[0].RolledHours != 10 || got[0].RolledMinutes != 450 || got[0].RemainingHours != 2.5 || got[0].VarianceHours != -2.5 {
		t.Fatalf("A %+v", got[0])
	}
	if got[1].RolledMinutes != 390 || got[1].RemainingHours != 0 || got[1].VarianceHours != 4.5 {
		t.Fatalf("B %+v", got[1])
	}
	if got[2].EstimateHours != nil || got[2].Minutes != 300 {
		t.Fatalf("C %+v", got[2])
	}
	_ = third
}
