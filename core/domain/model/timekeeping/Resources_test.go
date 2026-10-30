// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"testing"
	"time"
)

// closedOn is a business calendar that works Monday to Friday except the days it names.
type closedOn map[string]bool

func (c closedOn) WorksOn(y int, m time.Month, d int) bool {
	at := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	if at.Weekday() == time.Saturday || at.Weekday() == time.Sunday {
		return false
	}
	return !c[at.Format(time.DateOnly)]
}

var fullTime = [7]int{480, 480, 480, 480, 480, 0, 0}

func TestCapacityAndAbsencesAreChecked(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"a full-time week", CheckCapacity(Capacity{WeekMinutes: fullTime}), ""},
		{"more than a day", CheckCapacity(Capacity{WeekMinutes: [7]int{1441}}), "time.capacity_out_of_range"},
		{"a negative day", CheckCapacity(Capacity{WeekMinutes: [7]int{-1}}), "time.capacity_out_of_range"},
		{"a week's leave", CheckAbsence(Absence{Kind: AbsenceLeave, From: day(t, "2026-11-02"), To: day(t, "2026-11-06")}), ""},
		{"an unknown kind", CheckAbsence(Absence{Kind: "SICK", From: day(t, "2026-11-02"), To: day(t, "2026-11-02")}), "time.absence_kind_unknown"},
		{"a reversed span", CheckAbsence(Absence{Kind: AbsenceOther, From: day(t, "2026-11-03"), To: day(t, "2026-11-02")}), "time.range_reversed"},
		{"more than a year", CheckAbsence(Absence{Kind: AbsenceOther, From: day(t, "2026-01-01"), To: day(t, "2027-01-02")}), "time.range_too_long"},
	}
	for _, c := range cases {
		if got := code(c.err); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	a := Absence{ID: recordID, AccountID: person, From: day(t, "2026-11-02"), To: day(t, "2026-11-04")}
	if !a.Overlaps(Absence{ID: entryA, AccountID: person, From: day(t, "2026-11-04"), To: day(t, "2026-11-05")}) {
		t.Error("absences sharing a day do not overlap")
	}
	if a.Overlaps(Absence{ID: entryA, AccountID: other, From: day(t, "2026-11-04"), To: day(t, "2026-11-05")}) {
		t.Error("another person's absence overlaps")
	}
}

func TestAvailableTimeFollowsCapacityHolidaysAndAbsences(t *testing.T) {
	capacities := []Capacity{
		{From: day(t, "2026-01-01"), WeekMinutes: fullTime},
		{From: day(t, "2026-11-09"), WeekMinutes: [7]int{360, 360, 360, 360, 0, 0, 0}},
	}
	absences := []Absence{
		{From: day(t, "2026-11-04"), To: day(t, "2026-11-04"), Kind: AbsenceLeave},
		{From: day(t, "2026-11-05"), To: day(t, "2026-11-05"), Kind: AbsenceTraining, Half: true},
	}
	calendar := closedOn{"2026-11-03": true}
	cases := []struct {
		day     string
		minutes int
	}{
		{"2026-11-02", 480}, // a Monday
		{"2026-11-03", 0},   // a holiday of the calendar
		{"2026-11-04", 0},   // on leave
		{"2026-11-05", 240}, // half a day's training
		{"2026-11-07", 0},   // a Saturday
		{"2026-11-09", 360}, // part time from here
		{"2026-11-13", 0},   // a Friday off in part time
	}
	for _, c := range cases {
		minutes, known := Available(capacities, absences, calendar, day(t, c.day))
		if !known || minutes != c.minutes {
			t.Errorf("%s: %d (%v), want %d", c.day, minutes, known, c.minutes)
		}
	}
	if _, known := Available(capacities, nil, calendar, day(t, "2025-12-31")); known {
		t.Error("a day before every capacity is known")
	}
}

func TestWorkloadSpreadsAllocationsAndMarksOverAllocation(t *testing.T) {
	people := []Person{
		{AccountID: person, Capacities: []Capacity{{From: day(t, "2026-01-01"), WeekMinutes: fullTime}},
			Absences: []Absence{{From: day(t, "2026-11-12"), To: day(t, "2026-11-13"), Kind: AbsenceLeave}}},
		{AccountID: other},
	}
	allocations := []Allocation{
		// Work from Monday to Thursday seen from Wednesday: its 40 hours fall on Wednesday and
		// Thursday, in a week of 40 available.
		{EntryID: entryA, AccountID: person, Start: day(t, "2026-11-02"), Finish: day(t, "2026-11-05"), Minutes: 2400},
		// The second week's 30 hours spread over Monday … Wednesday, the days not on leave, against
		// 24 available.
		{EntryID: entryB, AccountID: person, Start: day(t, "2026-11-09"), Finish: day(t, "2026-11-13"), Minutes: 1800},
		// Work whose span is past asks of the view's first day.
		{EntryID: entryC, AccountID: person, Start: day(t, "2026-10-26"), Finish: day(t, "2026-10-28"), Minutes: 60},
		// Somebody without a capacity is laid out, but nothing is over.
		{EntryID: entryA, AccountID: other, Start: day(t, "2026-11-02"), Finish: day(t, "2026-11-02"), Minutes: 600},
	}
	cells := Workload(people, closedOn{}, allocations, day(t, "2026-11-04"), 2)
	if len(cells) != 4 {
		t.Fatalf("%d cells", len(cells))
	}
	first, second := cells[0], cells[1]
	if first.Week != day(t, "2026-11-02") || first.Available != 2400 || first.Allocated != 2460 || !first.Over() {
		t.Errorf("first week %+v", first)
	}
	if second.Available != 1440 || second.Allocated != 1800 || !second.Over() {
		t.Errorf("second week %+v", second)
	}
	if cells[2].Known || cells[2].Over() || cells[2].Allocated != 600 {
		t.Errorf("unknown capacity %+v", cells[2])
	}
	team := SumByWeek(cells)
	if len(team) != 2 || team[0].People != 1 || team[0].Allocated != 2460 {
		t.Errorf("team weeks %+v", team)
	}
	if (WorkloadCell{Known: true, Available: 2400, Allocated: 1000}).Under() != true {
		t.Error("a week less than four fifths full is not under")
	}
}
