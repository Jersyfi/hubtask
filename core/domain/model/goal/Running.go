// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"math/big"
	"slices"
	"strings"
)

// Point is one row of a summary as a widget draws it: its group, its period's first day, its count
// and, for a function over a measure, its value as an exact decimal.
type Point struct {
	Group  string
	Period string
	Count  int64
	Value  *string
}

// Left is the work between a widget's two running totals at one period of one group.
type Left struct {
	Group  string
	Period string
	Value  string
}

// RunningTotals answers each point's running total - its group's amounts summed over the periods up
// to its own, a point without a period being its own -, in the points' order. The amount is the
// value where there is one, else the count; exact, so money keeps its cents.
func RunningTotals(points []Point) []string {
	totals := runningOf(points)
	out := make([]string, len(points))
	for i, p := range points {
		out[i] = totals.text(totals.at(p.Group, p.Period))
	}
	return out
}

// Remaining answers, for every period either series holds, the first series' running total less
// the second's in each group - what is created and not yet resolved, a burndown.
func Remaining(first, second []Point) []Left {
	a, b := runningOf(first), runningOf(second)
	var groups []string
	periods := map[string][]string{}
	for _, p := range slices.Concat(first, second) {
		if _, known := periods[p.Group]; !known {
			groups = append(groups, p.Group)
		}
		if !slices.Contains(periods[p.Group], p.Period) {
			periods[p.Group] = append(periods[p.Group], p.Period)
		}
	}
	scale := max(a.scale, b.scale)
	var out []Left
	for _, g := range groups {
		ps := periods[g]
		slices.Sort(ps)
		for _, p := range ps {
			left := new(big.Rat).Sub(a.at(g, p), b.at(g, p))
			out = append(out, Left{Group: g, Period: p, Value: left.FloatString(scale)})
		}
	}
	return out
}

type running struct {
	// sums holds each group's periods in order with the total up to each.
	sums  map[string][]periodSum
	scale int
}

type periodSum struct {
	period string
	total  *big.Rat
}

func runningOf(points []Point) running {
	r := running{sums: map[string][]periodSum{}}
	amounts := map[string]map[string]*big.Rat{}
	for _, p := range points {
		amount, scale := amountOf(p)
		r.scale = max(r.scale, scale)
		if amounts[p.Group] == nil {
			amounts[p.Group] = map[string]*big.Rat{}
		}
		if sum, ok := amounts[p.Group][p.Period]; ok {
			sum.Add(sum, amount)
		} else {
			amounts[p.Group][p.Period] = amount
		}
	}
	for group, byPeriod := range amounts {
		periods := make([]string, 0, len(byPeriod))
		for period := range byPeriod {
			periods = append(periods, period)
		}
		slices.Sort(periods)
		total := new(big.Rat)
		for _, period := range periods {
			if period == "" {
				// A row without a period is a total of its own, never a step of a run.
				r.sums[group] = append(r.sums[group], periodSum{period: "", total: byPeriod[period]})
				continue
			}
			total = new(big.Rat).Add(total, byPeriod[period])
			r.sums[group] = append(r.sums[group], periodSum{period: period, total: total})
		}
	}
	return r
}

// at answers a group's running total at a period: the last period up to it, zero before the first.
func (r running) at(group, period string) *big.Rat {
	found := new(big.Rat)
	for _, s := range r.sums[group] {
		if s.period == "" {
			if period == "" {
				return s.total
			}
			continue
		}
		if period != "" && s.period > period {
			break
		}
		found = s.total
	}
	return found
}

func (r running) text(v *big.Rat) string { return v.FloatString(r.scale) }

func amountOf(p Point) (*big.Rat, int) {
	if p.Value != nil {
		if v, ok := new(big.Rat).SetString(*p.Value); ok {
			scale := 0
			if dot := strings.IndexByte(*p.Value, '.'); dot >= 0 {
				scale = len(*p.Value) - dot - 1
			}
			return v, scale
		}
	}
	return new(big.Rat).SetInt64(p.Count), 0
}
