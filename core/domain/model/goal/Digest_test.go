// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"testing"
	"time"
)

func TestADigestIsCheckedByItsCadence(t *testing.T) {
	cases := []struct {
		name string
		in   Digest
		want string
	}{
		{"daily", Digest{Cadence: CadenceDaily, Hour: 7, TimeZone: "Europe/Berlin"}, ""},
		{"weekly on a Monday", Digest{Cadence: CadenceWeekly, Weekday: 1, Hour: 8}, ""},
		{"weekly without a day", Digest{Cadence: CadenceWeekly, Hour: 8}, "dashboards.digest_weekday_invalid"},
		{"an hour past the day", Digest{Cadence: CadenceDaily, Hour: 24}, "dashboards.digest_hour_invalid"},
		{"no cadence", Digest{Hour: 8}, "dashboards.digest_cadence_invalid"},
		{"an unknown zone", Digest{Cadence: CadenceDaily, TimeZone: "Mars/Olympus"}, "dashboards.digest_zone_invalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := CheckDigest(c.in)
			if code(err) != c.want {
				t.Fatalf("got %q, want %q", code(err), c.want)
			}
		})
	}
}

func TestADigestIsOwedAtItsHourInItsZone(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	// Friday 30 October 2026, 09:30 in Berlin.
	now := time.Date(2026, 10, 30, 9, 30, 0, 0, berlin)
	cases := []struct {
		name string
		d    Digest
		want time.Time
	}{
		{"daily, the hour gone today", Digest{Cadence: CadenceDaily, Hour: 8, TimeZone: "Europe/Berlin"},
			time.Date(2026, 10, 31, 8, 0, 0, 0, berlin)},
		{"daily, the hour still to come", Digest{Cadence: CadenceDaily, Hour: 17, TimeZone: "Europe/Berlin"},
			time.Date(2026, 10, 30, 17, 0, 0, 0, berlin)},
		{"weekly on Monday, across the change to winter time", Digest{Cadence: CadenceWeekly, Weekday: 1, Hour: 8, TimeZone: "Europe/Berlin"},
			time.Date(2026, 11, 2, 8, 0, 0, 0, berlin)},
		{"weekly on Friday, today's hour gone", Digest{Cadence: CadenceWeekly, Weekday: 5, Hour: 9, TimeZone: "Europe/Berlin"},
			time.Date(2026, 11, 6, 9, 0, 0, 0, berlin)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.d.Next(now); !got.Equal(c.want) {
				t.Fatalf("got %v, want %v", got, c.want.UTC())
			}
		})
	}
}

func TestANumberWidgetsTotalIsExact(t *testing.T) {
	cases := []struct {
		name string
		rows []map[string]any
		want string
		has  bool
	}{
		{"counts", []map[string]any{{"count": 3}, {"count": int64(4)}}, "7", true},
		{"money values", []map[string]any{{"value": "0.10", "count": 1}, {"value": "0.20", "count": 1}}, "0.30", true},
		{"a value beside a count", []map[string]any{{"value": "1.5"}, {"count": 2}}, "3.5", true},
		{"nothing", nil, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, has := Total(c.rows)
			if got != c.want || has != c.has {
				t.Fatalf("got %q %v, want %q %v", got, has, c.want, c.has)
			}
		})
	}
}
