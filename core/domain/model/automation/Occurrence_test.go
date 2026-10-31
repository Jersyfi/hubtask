// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation_test

import (
	"testing"
	"time"

	automation "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
)

// The arithmetic a RELATIVE_DATE trigger is: an anchor plus a signed offset, or nothing at all.
//
// "Nothing at all" is the half worth stating. A cleared anchor is not an anchor of zero, and the
// distinction is what makes "does not fire for an anchor that was cleared" a property of this
// function rather than of a caller that remembered to check.
func TestAnOccurrenceIsTheAnchorPlusTheOffsetOrNothing(t *testing.T) {
	anchor := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)
	relative := func(offset string) automation.Trigger {
		return automation.Trigger{
			Kind: automation.TriggerRelativeDate, Anchor: automation.AnchorDueDate, Offset: offset,
		}
	}

	cases := map[string]struct {
		trigger automation.Trigger
		anchor  *time.Time
		want    time.Time
		owes    bool
	}{
		"24 hours before": {
			trigger: relative("-PT24H"), anchor: &anchor,
			want: anchor.Add(-24 * time.Hour), owes: true,
		},
		"three days after": {
			trigger: relative("P3D"), anchor: &anchor,
			want: anchor.Add(72 * time.Hour), owes: true,
		},
		"an unsigned offset is forward": {
			trigger: relative("PT90M"), anchor: &anchor,
			want: anchor.Add(90 * time.Minute), owes: true,
		},
		"a cleared anchor": {
			trigger: relative("-PT24H"), anchor: nil,
		},
		"an anchor of the zero time": {
			trigger: relative("-PT24H"), anchor: &time.Time{},
		},
		"another kind of trigger": {
			trigger: automation.Trigger{Kind: automation.TriggerManual}, anchor: &anchor,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			at, owes, err := automation.OccurrenceAt(tc.trigger, tc.anchor)
			if err != nil {
				t.Fatalf("working it out: %v", err)
			}
			if owes != tc.owes {
				t.Fatalf("owes=%v, want %v", owes, tc.owes)
			}
			if owes && !at.Equal(tc.want) {
				t.Errorf("owes at %v, want %v", at, tc.want)
			}
			if !owes && !at.IsZero() {
				t.Errorf("a rule that owes nothing answered %v", at)
			}
		})
	}
}

// Unreachable through the aggregate, which parses the same text when the rule is written. Reported
// rather than swallowed all the same: a rule stored before an offset grammar changed would
// otherwise fire at the anchor itself, which is a run at a moment nobody chose.
func TestAnOffsetThisBuildCannotReadIsAnError(t *testing.T) {
	anchor := time.Date(2026, 8, 30, 9, 0, 0, 0, time.UTC)

	_, owes, err := automation.OccurrenceAt(automation.Trigger{
		Kind: automation.TriggerRelativeDate, Anchor: automation.AnchorDueDate, Offset: "P1M",
	}, &anchor)
	if err == nil {
		t.Fatal("a month-long offset was read as a length of time")
	}
	if owes {
		t.Error("a rule owes a moment it could not work out")
	}
}

// A trigger on a business calendar counts the calendar's working days (configuration.md §6, "close
// resolved incidents after 5 business days"); one without a calendar is the plain arithmetic.
func TestABusinessDayOffsetCountsTheCalendarsWorkingDays(t *testing.T) {
	zone, _ := time.LoadLocation("Europe/Berlin")
	var week calendar.Week
	for _, day := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday} {
		week[day] = []calendar.Span{{From: 8 * 60, To: 18 * 60}}
	}
	office, err := calendar.New(calendar.Spec{Zone: "Europe/Berlin", Week: week, Holidays: []calendar.Date{{Year: 2026, Month: time.December, Day: 25}}})
	if err != nil {
		t.Fatal(err)
	}
	resolved := time.Date(2026, 12, 22, 14, 0, 0, 0, zone) // a Tuesday before Christmas
	trigger := automation.Trigger{
		Kind: automation.TriggerRelativeDate, Anchor: automation.AnchorCompletedAt, Offset: "P5D", Calendar: "office",
	}
	at, owes, err := automation.OccurrenceOn(trigger, &resolved, office)
	if err != nil || !owes {
		t.Fatalf("OccurrenceOn = %v, %v, %v", at, owes, err)
	}
	if want := time.Date(2026, 12, 30, 14, 0, 0, 0, zone); !at.Equal(want) {
		t.Errorf("five business days after %s is %s, want %s", resolved, at.In(zone), want)
	}
	if !at.After(resolved.Add(7 * 24 * time.Hour)) {
		t.Errorf("a holiday week closes earlier than seven calendar days: %s", at)
	}

	calendarDays := trigger
	calendarDays.Calendar = ""
	if at, _, _ := automation.OccurrenceOn(calendarDays, &resolved, office); !at.Equal(resolved.Add(5 * 24 * time.Hour).UTC()) {
		t.Errorf("a trigger without a calendar counts calendar days, got %s", at)
	}
	if _, owes, _ := automation.OccurrenceOn(trigger, nil, office); owes {
		t.Error("a cleared anchor owes nothing on a calendar either")
	}
}
