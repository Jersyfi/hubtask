// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"strconv"

	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The bounds of capacity and absences.
const (
	// MaxAbsenceDays is the longest absence one record holds: a year, as a report's range is.
	MaxAbsenceDays = 366
	// MaxWorkloadWeeks is the longest workload a view lays out.
	MaxWorkloadWeeks = 26
)

// Capacity is the time a person is available to work in each weekday from a day on - a part-time
// person's 4 days of 6 hours, a full-time one's 5 of 8 - until their next capacity begins.
type Capacity struct {
	AccountID shared.ID
	// From is the first day it holds.
	From Day
	// WeekMinutes are the minutes of Monday … Sunday, each 0 to a day.
	WeekMinutes [7]int
	Version     int
}

// CheckCapacity holds each weekday to a day's minutes.
func CheckCapacity(c Capacity) error {
	for _, minutes := range c.WeekMinutes {
		if minutes < 0 || minutes > MaxMinutes {
			return shared.ErrValidation.WithDetail("time.capacity_out_of_range").
				WithParams(map[string]string{"maximum": strconv.Itoa(MaxMinutes)}).
				WithFields(shared.FieldError{Path: "/week_minutes", Code: "time.capacity_out_of_range"})
		}
	}
	return nil
}

// CapacityOn answers the minutes a person's capacities give a day: the one begun last on or before
// it, by weekday. Without one, the person's capacity is not known.
func CapacityOn(capacities []Capacity, d Day) (minutes int, known bool) {
	var held *Capacity
	for i := range capacities {
		if capacities[i].From <= d && (held == nil || capacities[i].From > held.From) {
			held = &capacities[i]
		}
	}
	if held == nil {
		return 0, false
	}
	return held.WeekMinutes[weekday(d)], true
}

// AbsenceKind says why a person is away. There is no "sick": a reason of health is a special
// category of personal data (GDPR Art. 9) that a calendar the team shares must not show - an
// illness is recorded as OTHER, which says only that the person is away.
type AbsenceKind string

const (
	AbsenceLeave    AbsenceKind = "LEAVE"
	AbsenceTraining AbsenceKind = "TRAINING"
	AbsenceOther    AbsenceKind = "OTHER"
)

// Valid reports whether the kind is one of the three.
func (k AbsenceKind) Valid() bool {
	return k == AbsenceLeave || k == AbsenceTraining || k == AbsenceOther
}

// Absence is a span of days, both ends included, a person is away - wholly or for half of each
// day. Public holidays are not absences: they are the business calendar's, recorded once for
// every person and every plan.
type Absence struct {
	ID        shared.ID
	AccountID shared.ID
	From, To  Day
	Kind      AbsenceKind
	// Half says the person is away half of each day of the span.
	Half    bool
	Version int
}

// CheckAbsence holds an absence to a known kind and an ordered span of at most a year.
func CheckAbsence(a Absence) error {
	if !a.Kind.Valid() {
		return invalid("/kind", "time.absence_kind_unknown")
	}
	if a.To < a.From {
		return invalid("/to", "time.range_reversed")
	}
	if int(a.To-a.From)+1 > MaxAbsenceDays {
		return shared.ErrValidation.WithDetail("time.range_too_long").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxAbsenceDays)}).
			WithFields(shared.FieldError{Path: "/to", Code: "time.range_too_long"})
	}
	return nil
}

// Overlaps reports whether two absences of one person share a day; a person is not away twice.
func (a Absence) Overlaps(b Absence) bool {
	return a.AccountID == b.AccountID && a.ID != b.ID && a.From <= b.To && b.From <= a.To
}

// Available answers the minutes a person can work on a day: their capacity, nothing on a day the
// business calendar does not work (a public holiday, a closed weekday), half or nothing on a day
// they are away. Without a capacity it is not known.
func Available(capacities []Capacity, absences []Absence, calendar planning.Workdays, d Day) (minutes int, known bool) {
	minutes, known = CapacityOn(capacities, d)
	if !known {
		return 0, false
	}
	if calendar != nil {
		y, m, day := d.Date()
		if !calendar.WorksOn(y, m, day) {
			return 0, true
		}
	}
	for _, a := range absences {
		if d < a.From || d > a.To {
			continue
		}
		if !a.Half {
			return 0, true
		}
		minutes /= 2
	}
	return minutes, true
}

// Allocation is the work an open entry assigned to a person still asks for, spread over the days
// it is scheduled on.
type Allocation struct {
	EntryID   shared.ID
	AccountID shared.ID
	// Start and Finish are the entry's scheduled days, both included.
	Start, Finish Day
	// Minutes are the entry's remaining estimate.
	Minutes int
}

// WorkloadCell is one person's week: what they can give and what their entries ask of it.
type WorkloadCell struct {
	AccountID shared.ID
	Week      Day
	Available int
	Allocated int
	// Known is false where the person has no capacity, so nothing is over or under.
	Known bool
}

// Over reports whether the week asks more than the person can give.
func (c WorkloadCell) Over() bool { return c.Known && c.Allocated > c.Available }

// Under reports whether the week leaves at least a fifth of what the person can give unasked:
// room to hand out more work.
func (c WorkloadCell) Under() bool { return c.Known && c.Available > 0 && c.Allocated*5 < c.Available*4 }

// Person is what the workload knows of one person: their capacities and their absences.
type Person struct {
	AccountID  shared.ID
	Capacities []Capacity
	Absences   []Absence
}

// Workload lays out the weeks from the Monday of `from` for each person: the minutes available
// and those their allocations ask. An allocation is spread evenly over the days of its span the
// person can work, from `from` on - a span already past asks of `from`'s week -, and over the span's
// working days where they can work none of them, so that work is never lost from the view.
func Workload(people []Person, calendar planning.Workdays, allocations []Allocation, from Day, weeks int) []WorkloadCell {
	if weeks < 1 {
		weeks = 1
	}
	if weeks > MaxWorkloadWeeks {
		weeks = MaxWorkloadWeeks
	}
	first := WeekOf(from)
	last := first + Day(7*weeks) - 1
	index := map[shared.ID]int{}
	cells := make([]WorkloadCell, 0, len(people)*weeks)
	for p, person := range people {
		index[person.AccountID] = p
		for w := 0; w < weeks; w++ {
			week := first + Day(7*w)
			cell := WorkloadCell{AccountID: person.AccountID, Week: week}
			for d := week; d < week+7; d++ {
				minutes, known := Available(person.Capacities, person.Absences, calendar, d)
				cell.Available += minutes
				cell.Known = cell.Known || known
			}
			cells = append(cells, cell)
		}
	}
	for _, a := range allocations {
		p, ok := index[a.AccountID]
		if !ok || a.Minutes <= 0 {
			continue
		}
		person := people[p]
		start, finish := a.Start, a.Finish
		if finish < start {
			start, finish = finish, start
		}
		if start < from {
			start = from
		}
		if finish < from {
			finish = from
		}
		days := spreadDays(person, calendar, start, finish)
		share, rest := a.Minutes/len(days), a.Minutes%len(days)
		for i, d := range days {
			if d < first || d > last {
				continue
			}
			minutes := share
			if i < rest {
				minutes++
			}
			cells[p*weeks+int(d-first)/7].Allocated += minutes
		}
	}
	return cells
}

// spreadDays answers the days of a span an allocation is spread over: those the person can work,
// else the business calendar's working days, else the span's last day.
func spreadDays(person Person, calendar planning.Workdays, start, finish Day) []Day {
	var available, working []Day
	for d := start; d <= finish; d++ {
		if minutes, _ := Available(person.Capacities, person.Absences, calendar, d); minutes > 0 {
			available = append(available, d)
		}
		y, m, day := d.Date()
		if calendar == nil || calendar.WorksOn(y, m, day) {
			working = append(working, d)
		}
	}
	switch {
	case len(available) > 0:
		return available
	case len(working) > 0:
		return working
	default:
		return []Day{finish}
	}
}

// TeamWeek is a week's sums over every person whose capacity is known - the workload where the
// workspace shows only sums for a team (Visibility AGGREGATED), naming nobody.
type TeamWeek struct {
	Week      Day
	Available int
	Allocated int
	People    int
}

// SumByWeek adds the known cells of each week, in week order.
func SumByWeek(cells []WorkloadCell) []TeamWeek {
	byWeek := map[Day]*TeamWeek{}
	var order []Day
	for _, c := range cells {
		if !c.Known {
			continue
		}
		week, ok := byWeek[c.Week]
		if !ok {
			week = &TeamWeek{Week: c.Week}
			byWeek[c.Week] = week
			order = append(order, c.Week)
		}
		week.Available += c.Available
		week.Allocated += c.Allocated
		week.People++
	}
	out := make([]TeamWeek, 0, len(order))
	for _, w := range order {
		out = append(out, *byWeek[w])
	}
	return out
}

// weekday answers Monday 0 … Sunday 6.
func weekday(d Day) int { return int(d - WeekOf(d)) }
