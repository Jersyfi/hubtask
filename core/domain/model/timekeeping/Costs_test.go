// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import "testing"

func TestCostRatesAreChecked(t *testing.T) {
	end := day(t, "2026-12-31")
	before := day(t, "2026-01-01")
	cases := []struct {
		name string
		rate CostRate
		want string
	}{
		{"a person's rate", CostRate{AccountID: person, HourMinor: 9500, Currency: "EUR", From: before}, ""},
		{"a role's rate until a day", CostRate{Role: "developer", HourMinor: 8000, Currency: "EUR", From: before, To: &end}, ""},
		{"no subject", CostRate{HourMinor: 1, Currency: "EUR"}, "time.rate_subject_required"},
		{"both subjects", CostRate{AccountID: person, Role: "developer", Currency: "EUR"}, "time.rate_subject_required"},
		{"a negative amount", CostRate{AccountID: person, HourMinor: -1, Currency: "EUR"}, "time.rate_out_of_range"},
		{"too high an amount", CostRate{AccountID: person, HourMinor: MaxRateMinor + 1, Currency: "EUR"}, "time.rate_out_of_range"},
		{"a malformed currency", CostRate{AccountID: person, Currency: "eur"}, "time.currency_invalid"},
		{"an end before the start", CostRate{AccountID: person, Currency: "EUR", From: end, To: &before}, "time.range_reversed"},
	}
	for _, c := range cases {
		if got := code(CheckCostRate(c.rate)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestOneRateHoldsOnEachDay(t *testing.T) {
	june := day(t, "2026-06-30")
	existing := []CostRate{
		{ID: recordID, AccountID: person, Currency: "EUR", From: day(t, "2026-01-01"), To: &june},
		{ID: entryB, Role: "developer", Currency: "EUR", From: day(t, "2026-01-01")},
	}
	cases := []struct {
		name string
		rate CostRate
		want string
	}{
		{"from the day after", CostRate{ID: entryA, AccountID: person, From: day(t, "2026-07-01")}, ""},
		{"on the last day", CostRate{ID: entryA, AccountID: person, From: june}, "time.rate_overlaps"},
		{"before and open-ended", CostRate{ID: entryA, AccountID: person, From: day(t, "2025-01-01")}, "time.rate_overlaps"},
		{"the rate itself changed", CostRate{ID: recordID, AccountID: person, From: day(t, "2026-02-01")}, ""},
		{"another person", CostRate{ID: entryA, AccountID: other, From: day(t, "2026-03-01")}, ""},
		{"the role again", CostRate{ID: entryA, Role: "developer", From: day(t, "2027-01-01")}, "time.rate_overlaps"},
	}
	for _, c := range cases {
		if got := code(CheckRateOverlap(existing, c.rate)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAPersonsRateComesBeforeTheirRole(t *testing.T) {
	june := day(t, "2026-06-30")
	rates := NewRates([]CostRate{
		{AccountID: person, HourMinor: 9500, Currency: "EUR", From: day(t, "2026-01-01"), To: &june},
		{Role: "lead", HourMinor: 12000, Currency: "EUR", From: day(t, "2026-01-01")},
		{Role: "developer", HourMinor: 8000, Currency: "EUR", From: day(t, "2026-01-01")},
	})
	if r, ok := rates.For(person, []string{"developer"}, day(t, "2026-03-01")); !ok || r.HourMinor != 9500 {
		t.Errorf("own rate %+v %v", r, ok)
	}
	if r, ok := rates.For(person, []string{"developer", "lead"}, day(t, "2026-07-01")); !ok || r.HourMinor != 8000 {
		t.Errorf("after the own rate ended %+v %v", r, ok)
	}
	if _, ok := rates.For(other, nil, day(t, "2026-07-01")); ok {
		t.Error("somebody without a rate or a role is priced")
	}
	cases := []struct {
		minutes int
		hour    int64
		want    int64
	}{
		{60, 9500, 9500}, {30, 9500, 4750}, {1, 100, 2}, {1, 29, 0}, {-30, 9500, -4750}, {-1, 100, -2},
	}
	for _, c := range cases {
		if got := CostMinor(c.minutes, c.hour); got != c.want {
			t.Errorf("%d minutes at %d: %d, want %d", c.minutes, c.hour, got, c.want)
		}
	}
}

func TestABudgetBurnsByTimeAndMoney(t *testing.T) {
	if got := code(CheckBudget(Budget{Minutes: 6000, AmountMinor: 1_000_000, Currency: "EUR"})); got != "" {
		t.Errorf("a budget: %q", got)
	}
	if got := code(CheckBudget(Budget{AmountMinor: 100})); got != "time.currency_invalid" {
		t.Errorf("money without a currency: %q", got)
	}
	if got := code(CheckBudget(Budget{Minutes: -1})); got != "time.budget_out_of_range" {
		t.Errorf("negative hours: %q", got)
	}
	rates := NewRates([]CostRate{
		{AccountID: person, HourMinor: 10000, Currency: "EUR", From: day(t, "2026-01-01")},
		{AccountID: other, HourMinor: 9000, Currency: "CHF", From: day(t, "2026-01-01")},
	})
	budget := Budget{Minutes: 300, AmountMinor: 40000, Currency: "EUR"}
	burn := BurnOf(budget, []Priced{
		{AccountID: person, Day: day(t, "2026-11-02"), Minutes: 120},
		{AccountID: other, Day: day(t, "2026-11-03"), Minutes: 60},
		{AccountID: third, Day: day(t, "2026-11-17"), Minutes: 30},
		{AccountID: person, Day: day(t, "2026-11-18"), Minutes: 150},
		{AccountID: person, Day: day(t, "2026-11-19"), Minutes: -30},
	}, rates)
	if burn.Minutes != 330 || burn.AmountMinor != 40000 || burn.UnpricedMinutes != 90 {
		t.Errorf("burn %+v", burn)
	}
	if !burn.OverTime() || burn.OverMoney() {
		t.Errorf("over time %v, over money %v", burn.OverTime(), burn.OverMoney())
	}
	want := []BurnWeek{
		{Week: day(t, "2026-11-02"), Minutes: 180, AmountMinor: 20000},
		{Week: day(t, "2026-11-09"), Minutes: 180, AmountMinor: 20000},
		{Week: day(t, "2026-11-16"), Minutes: 330, AmountMinor: 40000},
	}
	if len(burn.Weeks) != len(want) {
		t.Fatalf("weeks %+v", burn.Weeks)
	}
	for i := range want {
		if burn.Weeks[i] != want[i] {
			t.Errorf("week %d: %+v, want %+v", i, burn.Weeks[i], want[i])
		}
	}
	if empty := BurnOf(budget, nil, rates); empty.Minutes != 0 || len(empty.Weeks) != 0 {
		t.Errorf("no records %+v", empty)
	}
}
