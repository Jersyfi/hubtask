// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"strconv"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxRateMinor is the highest hourly rate, in a currency's minor unit: ten million an hour.
const MaxRateMinor = 1_000_000_000

// CostRate is what an hour of a person's - or of a role's - time costs from a day on, until a day
// where it ends: a number and a currency, never a price list of Hubtask's own. A person's rate
// comes before their role's.
type CostRate struct {
	ID shared.ID
	// AccountID is the person it prices; zero for a role's rate.
	AccountID shared.ID
	// Role is the role key it prices; empty for a person's rate.
	Role string
	// HourMinor is an hour's cost in the currency's minor unit (cents for EUR).
	HourMinor int64
	// Currency is an ISO 4217 code.
	Currency string
	From     Day
	// To is the last day it holds; nil while it holds on.
	To      *Day
	Version int
}

// HoldsOn reports whether the rate prices a day.
func (r CostRate) HoldsOn(d Day) bool { return r.From <= d && (r.To == nil || d <= *r.To) }

// CheckCostRate holds a rate to one subject, an amount in bounds, a currency code and an ordered
// validity.
func CheckCostRate(r CostRate) error {
	if r.AccountID.IsZero() == (r.Role == "") {
		return invalid("/account_id", "time.rate_subject_required")
	}
	if r.HourMinor < 0 || r.HourMinor > MaxRateMinor {
		return shared.ErrValidation.WithDetail("time.rate_out_of_range").
			WithParams(map[string]string{"maximum": strconv.FormatInt(MaxRateMinor, 10)}).
			WithFields(shared.FieldError{Path: "/hour_minor", Code: "time.rate_out_of_range"})
	}
	if !CurrencyValid(r.Currency) {
		return invalid("/currency", "time.currency_invalid")
	}
	if r.To != nil && *r.To < r.From {
		return invalid("/to", "time.range_reversed")
	}
	return nil
}

// CheckRateOverlap refuses a rate whose validity shares a day with another of the same subject,
// so that a day always has one rate.
func CheckRateOverlap(existing []CostRate, r CostRate) error {
	for _, other := range existing {
		if other.ID == r.ID || other.AccountID != r.AccountID || other.Role != r.Role {
			continue
		}
		startsBeforeEnd := r.To == nil || other.From <= *r.To
		endsAfterStart := other.To == nil || r.From <= *other.To
		if startsBeforeEnd && endsAfterStart {
			return invalid("/from", "time.rate_overlaps")
		}
	}
	return nil
}

// CurrencyValid reports whether a code is three capital letters, as ISO 4217 writes one.
func CurrencyValid(code string) bool {
	if len(code) != 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		if code[i] < 'A' || code[i] > 'Z' {
			return false
		}
	}
	return true
}

// Rates answers a day's rate for a person: their own, else the first of their roles - in the
// order given, the most specific first - that has one.
type Rates struct {
	rates []CostRate
}

// NewRates holds a workspace's rates.
func NewRates(rates []CostRate) Rates { return Rates{rates: rates} }

// For answers the rate that prices a person's day, and whether one does.
func (r Rates) For(account shared.ID, roles []string, d Day) (CostRate, bool) {
	for _, rate := range r.rates {
		if rate.AccountID == account && rate.HoldsOn(d) {
			return rate, true
		}
	}
	for _, role := range roles {
		for _, rate := range r.rates {
			if rate.AccountID.IsZero() && rate.Role == role && rate.HoldsOn(d) {
				return rate, true
			}
		}
	}
	return CostRate{}, false
}

// CostMinor answers what minutes cost at an hourly rate, rounded half away from zero to the minor
// unit; a correction's negative minutes cost a negative amount.
func CostMinor(minutes int, hourMinor int64) int64 {
	product := int64(minutes) * hourMinor
	if product < 0 {
		return -((-product + 30) / 60)
	}
	return (product + 30) / 60
}

// Budget is what a hub or a collection may spend: hours, money in one currency, or both.
type Budget struct {
	// PlanID is the hub or collection.
	PlanID shared.ID
	// Minutes is the time budget; 0 for none.
	Minutes int64
	// AmountMinor is the money budget in the currency's minor unit; 0 for none.
	AmountMinor int64
	// Currency is the money budget's ISO 4217 code; empty without one.
	Currency string
	Version  int
}

// MaxBudgetMinor bounds a money budget: a hundred billion in the major unit.
const MaxBudgetMinor = 10_000_000_000_000

// CheckBudget holds a budget to non-negative bounds and a currency where it has money.
func CheckBudget(b Budget) error {
	if b.Minutes < 0 || b.Minutes > int64(MaxBudgetMinutes) {
		return shared.ErrValidation.WithDetail("time.budget_out_of_range").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxBudgetMinutes)}).
			WithFields(shared.FieldError{Path: "/minutes", Code: "time.budget_out_of_range"})
	}
	if b.AmountMinor < 0 || b.AmountMinor > MaxBudgetMinor {
		return shared.ErrValidation.WithDetail("time.budget_out_of_range").
			WithParams(map[string]string{"maximum": strconv.FormatInt(MaxBudgetMinor, 10)}).
			WithFields(shared.FieldError{Path: "/amount_minor", Code: "time.budget_out_of_range"})
	}
	if (b.AmountMinor > 0 || b.Currency != "") && !CurrencyValid(b.Currency) {
		return invalid("/currency", "time.currency_invalid")
	}
	return nil
}

// MaxBudgetMinutes bounds a time budget: a million hours.
const MaxBudgetMinutes = 60_000_000

// Priced is one record as the burn reads it: whose time, which day, how many minutes.
type Priced struct {
	AccountID shared.ID
	Day       Day
	Minutes   int
	// Roles are the person's role keys where the time was spent, the most specific first.
	Roles []string
}

// BurnWeek is the burn up to and including a week: the cumulative minutes and cost.
type BurnWeek struct {
	Week        Day
	Minutes     int64
	AmountMinor int64
}

// Burn is a budget's consumption: the time recorded, what it cost in the budget's currency, the
// minutes no rate in that currency prices, and the weeks the burn chart draws.
type Burn struct {
	Budget          Budget
	Minutes         int64
	AmountMinor     int64
	UnpricedMinutes int64
	Weeks           []BurnWeek
}

// OverTime reports whether the time recorded passed the time budget.
func (b Burn) OverTime() bool { return b.Budget.Minutes > 0 && b.Minutes > b.Budget.Minutes }

// OverMoney reports whether the cost passed the money budget.
func (b Burn) OverMoney() bool { return b.Budget.AmountMinor > 0 && b.AmountMinor > b.Budget.AmountMinor }

// BurnOf sums records against a budget: every minute counts against the time budget, and a
// minute costs what the day's rate says where that rate is in the budget's currency - a rate in
// another is not converted (there is no exchange-rate table) and its minutes are unpriced. The
// weeks run from the first record's week to the last's, cumulative.
func BurnOf(budget Budget, records []Priced, rates Rates) Burn {
	burn := Burn{Budget: budget}
	if len(records) == 0 {
		return burn
	}
	byWeek := map[Day]*BurnWeek{}
	first, last := WeekOf(records[0].Day), WeekOf(records[0].Day)
	for _, r := range records {
		week := WeekOf(r.Day)
		if week < first {
			first = week
		}
		if week > last {
			last = week
		}
		w, ok := byWeek[week]
		if !ok {
			w = &BurnWeek{Week: week}
			byWeek[week] = w
		}
		burn.Minutes += int64(r.Minutes)
		w.Minutes += int64(r.Minutes)
		rate, ok := rates.For(r.AccountID, r.Roles, r.Day)
		if !ok || budget.Currency == "" || rate.Currency != budget.Currency {
			burn.UnpricedMinutes += int64(r.Minutes)
			continue
		}
		cost := CostMinor(r.Minutes, rate.HourMinor)
		burn.AmountMinor += cost
		w.AmountMinor += cost
	}
	var minutes, amount int64
	for week := first; week <= last; week += 7 {
		if w, ok := byWeek[week]; ok {
			minutes += w.Minutes
			amount += w.AmountMinor
		}
		burn.Weeks = append(burn.Weeks, BurnWeek{Week: week, Minutes: minutes, AmountMinor: amount})
	}
	return burn
}
