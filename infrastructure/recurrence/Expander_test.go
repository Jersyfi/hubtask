// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package recurrence_test

import (
	"errors"
	"testing"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/recurrence"
	"github.com/Jersyfi/hubtask/infrastructure/recurrence"
)

// The adapter is where the library is, so this is where the library's behaviour is pinned: what it
// accepts, what it refuses, and - the reason ADR-0008 chose one at all - what it does across a
// daylight saving transition.

func at(t *testing.T, zone, value string) time.Time {
	t.Helper()

	location, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatalf("the zone does not load: %v", err)
	}
	moment, err := time.ParseInLocation("2006-01-02T15:04:05", value, location)
	if err != nil {
		t.Fatalf("the moment does not parse: %v", err)
	}
	return moment
}

// R-07's own case, which is why this is a library and not twenty lines of arithmetic: a daily
// series at 09:00 in Berlin stays at 09:00 through the spring transition, which means the instant
// it answers moves by an hour.
func TestADailySeriesHoldsItsLocalTimeAcrossATransition(t *testing.T) {
	start := at(t, "Europe/Berlin", "2026-03-27T09:00:00")

	moments, err := recurrence.New().Occurrences(port.Rule{
		RRULE: "FREQ=DAILY", TimeZone: "Europe/Berlin", Start: start,
	}, start, start.Add(96*time.Hour), 10)
	if err != nil {
		t.Fatalf("expanding: %v", err)
	}
	if len(moments) < 4 {
		t.Fatalf("the series produced %d moments", len(moments))
	}

	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	for _, moment := range moments {
		if local := moment.In(berlin); local.Hour() != 9 || local.Minute() != 0 {
			t.Errorf("an occurrence falls at %s local", local.Format(time.RFC3339))
		}
	}

	// And the proof that the zone did the work: the two sides of the transition are different
	// instants in UTC, an hour apart, for the same local time.
	before, after := moments[0].UTC(), moments[2].UTC()
	if before.Hour() == after.Hour() {
		t.Errorf("the transition changed nothing in UTC: %s and %s", before, after)
	}
}

// The two refusals the port declares, told apart by the sentinel a caller matches on.
func TestARuleThatCannotBeReadIsRefusedByItsReason(t *testing.T) {
	start := at(t, "Europe/Berlin", "2026-09-01T09:00:00")

	for name, test := range map[string]struct {
		rule port.Rule
		want error
	}{
		"text that is not a rule": {
			rule: port.Rule{RRULE: "every other tuesday", TimeZone: "UTC", Start: start},
			want: port.ErrRuleUnreadable,
		},
		"a rule whose parts are out of bounds": {
			rule: port.Rule{RRULE: "FREQ=DAILY;BYHOUR=25", TimeZone: "UTC", Start: start},
			want: port.ErrRuleUnreadable,
		},
		"a zone this installation does not have": {
			rule: port.Rule{RRULE: "FREQ=DAILY", TimeZone: "Mars/Olympus_Mons", Start: start},
			want: port.ErrZoneUnknown,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := recurrence.New().Occurrences(test.rule, start, start.Add(24*time.Hour), 10)
			if !errors.Is(err, test.want) {
				t.Fatalf("refused as %v, want %v", err, test.want)
			}
		})
	}
}

// The end spec is the rule's, not the text's: both spellings reach the expansion through the port's
// own fields, so an entry's stored end and its rule cannot disagree.
func TestTheEndSpecBoundsTheExpansion(t *testing.T) {
	start := at(t, "UTC", "2026-09-01T09:00:00")
	daily := port.Rule{RRULE: "FREQ=DAILY", TimeZone: "UTC", Start: start}

	counted := daily
	counted.Count = 3
	moments, err := recurrence.New().Occurrences(counted, start, start.Add(30*24*time.Hour), 50)
	if err != nil {
		t.Fatalf("expanding: %v", err)
	}
	if len(moments) != 3 {
		t.Errorf("a series of three produced %d moments", len(moments))
	}

	until := daily
	until.Until = start.Add(48 * time.Hour)
	moments, err = recurrence.New().Occurrences(until, start, start.Add(30*24*time.Hour), 50)
	if err != nil {
		t.Fatalf("expanding: %v", err)
	}
	if len(moments) != 3 {
		t.Errorf("a series ending after two days produced %d moments", len(moments))
	}
}

// The limit is the bound that keeps a rule from becoming a denial of service (T-17): a caller asks
// for one more than it will accept and refuses what comes back too long.
func TestTheLimitBoundsWhatComesBack(t *testing.T) {
	start := at(t, "UTC", "2026-09-01T09:00:00")

	moments, err := recurrence.New().Occurrences(port.Rule{
		RRULE: "FREQ=SECONDLY", TimeZone: "UTC", Start: start,
	}, start, start.Add(90*24*time.Hour), 501)
	if err != nil {
		t.Fatalf("expanding: %v", err)
	}
	if len(moments) != 501 {
		t.Fatalf("the expansion answered %d moments, want the limit", len(moments))
	}
}

// A series in a southern zone crosses its transitions the other way round, and the same rule
// holds: the local time is what stays put.
func TestASouthernSeriesHoldsItsLocalTimeToo(t *testing.T) {
	start := at(t, "America/Sao_Paulo", "2026-02-08T09:00:00")

	moments, err := recurrence.New().Occurrences(port.Rule{
		RRULE: "FREQ=WEEKLY;BYDAY=SU", TimeZone: "America/Sao_Paulo", Start: start,
	}, start, start.AddDate(0, 3, 0), 20)
	if err != nil {
		t.Fatalf("expanding: %v", err)
	}
	if len(moments) < 8 {
		t.Fatalf("the series produced %d moments", len(moments))
	}

	saoPaulo, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	for _, moment := range moments {
		local := moment.In(saoPaulo)
		if local.Hour() != 9 || local.Weekday() != time.Sunday {
			t.Errorf("an occurrence falls at %s local", local.Format(time.RFC3339))
		}
	}
}
