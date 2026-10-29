// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// officeHours is Monday to Friday, 09:00-12:00 and 13:00-17:00 in Berlin, with 25 December off.
func officeHours(t *testing.T) BusinessCalendar {
	t.Helper()
	day := []Span{{From: 9 * 60, To: 12 * 60}, {From: 13 * 60, To: 17 * 60}}
	var week Week
	for _, weekday := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday} {
		week[weekday] = day
	}
	cal, err := New(Spec{Zone: "Europe/Berlin", Week: week, Holidays: []Date{{2026, time.December, 25}}})
	if err != nil {
		t.Fatalf("office hours: %v", err)
	}
	return cal
}

func berlin(t *testing.T, value string) time.Time {
	t.Helper()
	zone, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.ParseInLocation("2006-01-02 15:04", value, zone)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestAddCountsOnlyWorkingTime(t *testing.T) {
	cal := officeHours(t)
	for _, tc := range []struct {
		name  string
		start string
		work  time.Duration
		want  string
	}{
		{"inside a span", "2026-10-28 09:30", time.Hour, "2026-10-28 10:30"},
		{"across lunch", "2026-10-28 11:30", time.Hour, "2026-10-28 13:30"},
		{"to the end of a span exactly", "2026-10-28 16:00", time.Hour, "2026-10-28 17:00"},
		{"overnight", "2026-10-28 16:00", 2 * time.Hour, "2026-10-29 10:00"},
		{"before opening", "2026-10-28 07:00", 30 * time.Minute, "2026-10-28 09:30"},
		{"over the weekend", "2026-10-30 16:00", 2 * time.Hour, "2026-11-02 10:00"},
		{"started on a Saturday", "2026-10-31 10:00", time.Hour, "2026-11-02 10:00"},
		{"over a holiday", "2026-12-24 16:00", 2 * time.Hour, "2026-12-28 10:00"},
		{"a whole working week", "2026-11-02 09:00", 35 * time.Hour, "2026-11-06 17:00"},
		{"no work", "2026-10-31 10:00", 0, "2026-10-31 10:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := cal.Add(berlin(t, tc.start), tc.work)
			if want := berlin(t, tc.want); !got.Equal(want) {
				t.Errorf("Add(%s, %s) = %s, want %s", tc.start, tc.work, got.In(want.Location()), want)
			}
		})
	}
}

func TestBetweenIsAddsInverse(t *testing.T) {
	cal := officeHours(t)
	for _, tc := range []struct {
		from, to string
		want     time.Duration
	}{
		{"2026-10-28 09:30", "2026-10-28 10:30", time.Hour},
		{"2026-10-28 11:30", "2026-10-28 13:30", time.Hour},
		{"2026-10-30 16:00", "2026-11-02 10:00", 2 * time.Hour},
		{"2026-12-24 16:00", "2026-12-28 10:00", 2 * time.Hour},
		{"2026-10-31 08:00", "2026-11-01 20:00", 0},
		{"2026-10-28 12:00", "2026-10-28 09:00", 0},
	} {
		if got := cal.Between(berlin(t, tc.from), berlin(t, tc.to)); got != tc.want {
			t.Errorf("Between(%s, %s) = %s, want %s", tc.from, tc.to, got, tc.want)
		}
	}
}

// The night the clocks go back (25 October 2026 in Berlin) has 25 hours; a continuous calendar
// in UTC and an all-day one in Berlin both count real elapsed time.
func TestAClockChangeCountsRealTime(t *testing.T) {
	var week Week
	for day := range week {
		week[day] = []Span{{From: 0, To: MinutesPerDay}}
	}
	allDay, err := New(Spec{Zone: "Europe/Berlin", Week: week})
	if err != nil {
		t.Fatal(err)
	}
	from, to := berlin(t, "2026-10-25 00:00"), berlin(t, "2026-10-26 00:00")
	if got := allDay.Between(from, to); got != 25*time.Hour {
		t.Errorf("the long night counts %s, want 25h", got)
	}
	if got := Continuous().Between(from, to); got != 25*time.Hour {
		t.Errorf("the continuous calendar counts %s, want 25h", got)
	}
}

func TestNewRefusesACalendarThatCannotCount(t *testing.T) {
	good := []Span{{From: 540, To: 1020}}
	for _, tc := range []struct {
		name   string
		spec   Spec
		detail string
	}{
		{"unknown zone", Spec{Zone: "Mars/Olympus", Week: Week{1: good}}, "calendar.zone_unknown"},
		{"no zone", Spec{Week: Week{1: good}}, "calendar.zone_unknown"},
		{"no working time", Spec{Zone: "UTC"}, "calendar.no_working_time"},
		{"empty span", Spec{Zone: "UTC", Week: Week{1: {{From: 600, To: 600}}}}, "calendar.span_invalid"},
		{"past midnight", Spec{Zone: "UTC", Week: Week{1: {{From: 600, To: 1441}}}}, "calendar.span_invalid"},
		{"overlapping", Spec{Zone: "UTC", Week: Week{1: {{From: 540, To: 720}, {From: 700, To: 800}}}}, "calendar.span_invalid"},
		{"impossible holiday", Spec{Zone: "UTC", Week: Week{1: good}, Holidays: []Date{{2026, time.February, 30}}}, "calendar.holiday_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.spec)
			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != tc.detail {
				t.Errorf("New = %v, want %s", err, tc.detail)
			}
		})
	}
}
