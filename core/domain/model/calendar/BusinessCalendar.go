// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package calendar is business time (F-10): a calendar of working hours, holidays and a time
// zone, the arithmetic that counts working time on it, and the durable timer that runs on it -
// the clock under an SLA, an approval's deadline and a change's effectiveness check
// (domain-model.md, business calendars and timers).
package calendar

import (
	"slices"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MinutesPerDay bounds a span: a working span is minutes from the local midnight.
const MinutesPerDay = 24 * 60

// MaxHolidays bounds a calendar's holiday list, so that a walk over working time always ends.
const MaxHolidays = 1000

// Span is one stretch of working time in a day, in minutes from local midnight, From before To.
type Span struct {
	From int
	To   int
}

// Week is the working spans of each weekday, indexed by time.Weekday (Sunday first).
type Week [7][]Span

// Date is a civil date in the calendar's zone: a holiday is a whole local day.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// Spec is a calendar as data - what a timer keeps of the calendar it was armed on, so that a
// later change to the calendar moves no clock already running.
type Spec struct {
	Zone     string
	Week     Week
	Holidays []Date
}

// BusinessCalendar counts working time. Built only by New or Continuous, so every value holds a
// loaded zone and at least one working minute a week.
type BusinessCalendar struct {
	spec     Spec
	location *time.Location
	holidays map[Date]bool
}

// New checks a calendar: a known zone, ordered and disjoint spans inside the day, at least one
// working minute a week, and at most MaxHolidays holidays.
func New(spec Spec) (BusinessCalendar, error) {
	location, err := time.LoadLocation(spec.Zone)
	if err != nil || spec.Zone == "" {
		return BusinessCalendar{}, shared.ErrValidation.WithDetail("calendar.zone_unknown")
	}
	working := false
	for day := range spec.Week {
		previous := 0
		for i, span := range spec.Week[day] {
			if span.From < 0 || span.To > MinutesPerDay || span.From >= span.To ||
				(i > 0 && span.From < previous) {
				return BusinessCalendar{}, shared.ErrValidation.WithDetail("calendar.span_invalid")
			}
			previous = span.To
			working = true
		}
	}
	if !working {
		return BusinessCalendar{}, shared.ErrValidation.WithDetail("calendar.no_working_time")
	}
	if len(spec.Holidays) > MaxHolidays {
		return BusinessCalendar{}, shared.ErrValidation.WithDetail("calendar.too_many_holidays")
	}
	holidays := make(map[Date]bool, len(spec.Holidays))
	for _, date := range spec.Holidays {
		normal := time.Date(date.Year, date.Month, date.Day, 0, 0, 0, 0, time.UTC)
		if normal.Year() != date.Year || normal.Month() != date.Month || normal.Day() != date.Day {
			return BusinessCalendar{}, shared.ErrValidation.WithDetail("calendar.holiday_invalid")
		}
		holidays[date] = true
	}
	copied := Spec{Zone: spec.Zone, Holidays: slices.Clone(spec.Holidays)}
	for day := range spec.Week {
		copied.Week[day] = slices.Clone(spec.Week[day])
	}
	return BusinessCalendar{spec: copied, location: location, holidays: holidays}, nil
}

// Continuous is the calendar of a clock that never stops: every minute of every day, in UTC.
func Continuous() BusinessCalendar {
	var week Week
	for day := range week {
		week[day] = []Span{{From: 0, To: MinutesPerDay}}
	}
	calendar, _ := New(Spec{Zone: "UTC", Week: week})
	return calendar
}

// Spec answers the calendar as data.
func (c BusinessCalendar) Spec() Spec { return c.spec }

// IsZero is a calendar nobody built.
func (c BusinessCalendar) IsZero() bool { return c.location == nil }

// spansOn answers the working stretches of one local day as instants, in order; none on a holiday.
// A span the zone's clock change cuts away (02:00-03:00 on the spring night) is left out.
func (c BusinessCalendar) spansOn(year int, month time.Month, day int) [][2]time.Time {
	date := time.Date(year, month, day, 12, 0, 0, 0, c.location)
	if c.holidays[Date{Year: date.Year(), Month: date.Month(), Day: date.Day()}] {
		return nil
	}
	var out [][2]time.Time
	for _, span := range c.spec.Week[date.Weekday()] {
		from := time.Date(year, month, day, 0, span.From, 0, 0, c.location)
		to := time.Date(year, month, day, 0, span.To, 0, 0, c.location)
		if to.After(from) {
			out = append(out, [2]time.Time{from, to})
		}
	}
	return out
}

// Zone answers the calendar's loaded zone: the place its days are local to.
func (c BusinessCalendar) Zone() *time.Location {
	if c.location == nil {
		return time.UTC
	}
	return c.location
}

// WorksOn reports whether a local day holds working time - the day a plan counts (M2).
func (c BusinessCalendar) WorksOn(year int, month time.Month, day int) bool {
	return len(c.spansOn(year, month, day)) > 0
}

// Add answers the instant at which work of the given length, started at start, is done in working
// time. A start outside working time begins at the next working minute; no work is done at start.
func (c BusinessCalendar) Add(start time.Time, work time.Duration) time.Time {
	if work <= 0 {
		return start
	}
	remaining := work
	local := start.In(c.location)
	year, month, day := local.Date()
	// Every week holds working time and the holidays are bounded, so the walk ends; the bound only
	// keeps a defect from spinning.
	for range 400 * 366 {
		for _, span := range c.spansOn(year, month, day) {
			if !span[1].After(start) {
				continue
			}
			from := span[0]
			if start.After(from) {
				from = start
			}
			available := span[1].Sub(from)
			if remaining <= available {
				return from.Add(remaining)
			}
			remaining -= available
		}
		next := time.Date(year, month, day+1, 12, 0, 0, 0, c.location)
		year, month, day = next.Date()
	}
	return start.Add(remaining)
}

// Between answers the working time in [from, to); none when to is not after from.
func (c BusinessCalendar) Between(from, to time.Time) time.Duration {
	if !to.After(from) {
		return 0
	}
	var total time.Duration
	local := from.In(c.location)
	year, month, day := local.Date()
	for {
		midnight := time.Date(year, month, day, 0, 0, 0, 0, c.location)
		if !midnight.Before(to) {
			return total
		}
		for _, span := range c.spansOn(year, month, day) {
			start, end := span[0], span[1]
			if start.Before(from) {
				start = from
			}
			if end.After(to) {
				end = to
			}
			if end.After(start) {
				total += end.Sub(start)
			}
		}
		next := time.Date(year, month, day+1, 12, 0, 0, 0, c.location)
		year, month, day = next.Date()
	}
}
