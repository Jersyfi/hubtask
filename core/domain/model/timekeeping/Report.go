// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Dimension is what a report groups by (UC-TIM-04).
type Dimension string

const (
	ByPerson     Dimension = "PERSON"
	ByEntry      Dimension = "ENTRY"
	ByCollection Dimension = "COLLECTION"
	ByHub        Dimension = "HUB"
	ByWorkType   Dimension = "WORK_TYPE"
	ByDay        Dimension = "DAY"
	ByWeek       Dimension = "WEEK"
	ByMonth      Dimension = "MONTH"
	ByBillable   Dimension = "BILLABLE"
)

// Dimensions are every dimension, in the order a screen offers them.
var Dimensions = []string{
	string(ByPerson), string(ByEntry), string(ByCollection), string(ByHub), string(ByWorkType),
	string(ByDay), string(ByWeek), string(ByMonth), string(ByBillable),
}

// MaxDimensions is how many dimensions one report groups by.
const MaxDimensions = 2

// MinPeoplePerGroup is the fewest people a group of an aggregated-only report sums: a smaller
// group would tell one person's minutes.
const MinPeoplePerGroup = 3

// Line is the time of one person on one day, entry, kind and billable flag: what a report sums.
type Line struct {
	AccountID    shared.ID
	EntryID      shared.ID
	CollectionID shared.ID
	HubID        shared.ID
	WorkType     string
	Day          Day
	Billable     bool
	Minutes      int
	Records      int
}

// Group is one row of a report: its key along each dimension and its sums.
type Group struct {
	Keys            []string
	Minutes         int
	BillableMinutes int
	Records         int
	people          map[shared.ID]bool
}

// Report is a report's rows and what an aggregated-only report withheld.
type Report struct {
	Dimensions []Dimension
	Groups     []Group
	Minutes    int
	// WithheldMinutes are the minutes of the groups too small to show without naming a person.
	WithheldMinutes int
	WithheldGroups  int
}

// CheckDimensions holds a report's grouping to the closed vocabulary, at most MaxDimensions and
// none twice.
func CheckDimensions(dims []Dimension) error {
	if len(dims) > MaxDimensions {
		return shared.ErrValidation.WithDetail("time.too_many_dimensions").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxDimensions)}).
			WithFields(shared.FieldError{Path: "/group_by", Code: "time.too_many_dimensions"})
	}
	for i, d := range dims {
		if !slices.Contains(Dimensions, string(d)) {
			return invalid("/group_by", "time.dimension_unknown")
		}
		if slices.Contains(dims[:i], d) {
			return invalid("/group_by", "time.dimension_twice")
		}
	}
	return nil
}

// Summarise groups the lines along the dimensions. Aggregated, a group holding fewer than
// MinPeoplePerGroup people is withheld - its minutes counted only in the total withheld - unless
// all of it is the reader's own time.
func Summarise(lines []Line, dims []Dimension, aggregated bool, reader shared.ID) Report {
	report := Report{Dimensions: dims}
	index := map[string]int{}
	for _, line := range lines {
		keys := make([]string, len(dims))
		for i, d := range dims {
			keys[i] = keyOf(line, d)
		}
		joined := strings.Join(keys, "\x00")
		at, ok := index[joined]
		if !ok {
			at = len(report.Groups)
			index[joined] = at
			report.Groups = append(report.Groups, Group{Keys: keys, people: map[shared.ID]bool{}})
		}
		g := &report.Groups[at]
		g.Minutes += line.Minutes
		g.Records += line.Records
		if line.Billable {
			g.BillableMinutes += line.Minutes
		}
		g.people[line.AccountID] = true
		report.Minutes += line.Minutes
	}
	if aggregated {
		kept := report.Groups[:0]
		for _, g := range report.Groups {
			if len(g.people) < MinPeoplePerGroup && !(len(g.people) == 1 && g.people[reader]) {
				report.WithheldMinutes += g.Minutes
				report.WithheldGroups++
				continue
			}
			kept = append(kept, g)
		}
		report.Groups = kept
	}
	slices.SortFunc(report.Groups, func(a, b Group) int { return slices.Compare(a.Keys, b.Keys) })
	for i := range report.Groups {
		report.Groups[i].people = nil
	}
	return report
}

func keyOf(line Line, d Dimension) string {
	switch d {
	case ByPerson:
		return line.AccountID.String()
	case ByEntry:
		return idText(line.EntryID)
	case ByCollection:
		return idText(line.CollectionID)
	case ByHub:
		return idText(line.HubID)
	case ByWorkType:
		return line.WorkType
	case ByDay:
		return line.Day.String()
	case ByWeek:
		return WeekOf(line.Day).String()
	case ByMonth:
		return MonthOf(line.Day).String()
	case ByBillable:
		return strconv.FormatBool(line.Billable)
	}
	return ""
}

func idText(id shared.ID) string {
	if id.IsZero() {
		return ""
	}
	return id.String()
}

// Planned is one entry of a plan with its estimate in hours, if it has one.
type Planned struct {
	ID       shared.ID
	ParentID shared.ID
	Hours    *float64
}

// Actual is one entry's estimate against the time recorded on it, on its own and with every entry
// below it in the plan (UC-TIM-04).
type Actual struct {
	ID            shared.ID
	EstimateHours *float64
	Minutes       int
	// RolledHours and RolledMinutes add the entries below it.
	RolledHours   float64
	RolledMinutes int
	// RemainingHours is the rolled estimate less the rolled time, never below zero.
	RemainingHours float64
	// VarianceHours is the rolled time less the rolled estimate: above zero, over the estimate.
	VarianceHours float64
}

// Actuals adds each entry's recorded minutes to its estimate and rolls both up the plan's parents,
// in the order the entries were given.
func Actuals(entries []Planned, minutes map[shared.ID]int) []Actual {
	children := map[shared.ID][]shared.ID{}
	known := map[shared.ID]Planned{}
	for _, e := range entries {
		known[e.ID] = e
	}
	for _, e := range entries {
		if _, ok := known[e.ParentID]; ok && e.ParentID != e.ID {
			children[e.ParentID] = append(children[e.ParentID], e.ID)
		}
	}
	type rolled struct {
		hours   float64
		minutes int
	}
	memo := map[shared.ID]rolled{}
	visiting := map[shared.ID]bool{}
	var roll func(id shared.ID) rolled
	roll = func(id shared.ID) rolled {
		if r, ok := memo[id]; ok {
			return r
		}
		if visiting[id] {
			return rolled{}
		}
		visiting[id] = true
		r := rolled{minutes: minutes[id]}
		if h := known[id].Hours; h != nil {
			r.hours = *h
		}
		for _, child := range children[id] {
			c := roll(child)
			r.hours += c.hours
			r.minutes += c.minutes
		}
		memo[id] = r
		return r
	}
	out := make([]Actual, 0, len(entries))
	for _, e := range entries {
		r := roll(e.ID)
		spent := float64(r.minutes) / 60
		out = append(out, Actual{
			ID: e.ID, EstimateHours: e.Hours, Minutes: minutes[e.ID],
			RolledHours: round2(r.hours), RolledMinutes: r.minutes,
			RemainingHours: round2(math.Max(0, r.hours-spent)), VarianceHours: round2(spent - r.hours),
		})
	}
	return out
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
