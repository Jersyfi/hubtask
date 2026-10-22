// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("zone %s: %v", name, err)
	}
	return location
}

// The reader's day and week, edge rows included: a month end, a year end, the two days a year the
// clock changes, a moment that is still yesterday in the reader's zone, each week start.
func TestTheFigureWindowIsTheReadersDayAndWeek(t *testing.T) {
	berlin := zone(t, "Europe/Berlin")
	auckland := zone(t, "Pacific/Auckland")
	cases := []struct {
		name      string
		now       string
		location  *time.Location
		weekStart string
		today     string
		dayEnd    string
		week      string
	}{
		{"a Thursday in Berlin, Monday weeks", "2026-10-22T10:00:00Z", berlin, "MONDAY",
			"2026-10-22", "2026-10-22T22:00:00Z", "2026-10-19"},
		{"Sunday weeks", "2026-10-22T10:00:00Z", berlin, "SUNDAY",
			"2026-10-22", "2026-10-22T22:00:00Z", "2026-10-18"},
		{"Saturday weeks", "2026-10-22T10:00:00Z", berlin, "SATURDAY",
			"2026-10-22", "2026-10-22T22:00:00Z", "2026-10-17"},
		{"an unknown week start is Monday", "2026-10-22T10:00:00Z", berlin, "",
			"2026-10-22", "2026-10-22T22:00:00Z", "2026-10-19"},
		{"23:30 UTC on a Sunday is Monday in Berlin", "2026-10-18T23:30:00Z", berlin, "MONDAY",
			"2026-10-19", "2026-10-19T22:00:00Z", "2026-10-19"},
		{"the autumn change: a day of 25 hours", "2026-10-25T08:00:00Z", berlin, "MONDAY",
			"2026-10-25", "2026-10-25T23:00:00Z", "2026-10-19"},
		{"the spring change: a day of 23 hours", "2026-03-29T08:00:00Z", berlin, "MONDAY",
			"2026-03-29", "2026-03-29T22:00:00Z", "2026-03-23"},
		{"a month end", "2026-10-31T12:00:00Z", berlin, "MONDAY",
			"2026-10-31", "2026-10-31T23:00:00Z", "2026-10-26"},
		{"a year end, the week reaching back into the old year", "2026-12-31T12:00:00Z", berlin, "MONDAY",
			"2026-12-31", "2026-12-31T23:00:00Z", "2026-12-28"},
		{"the new year's first day, a Friday", "2027-01-01T12:00:00Z", berlin, "MONDAY",
			"2027-01-01", "2027-01-01T23:00:00Z", "2026-12-28"},
		{"Auckland is already tomorrow", "2026-10-22T20:00:00Z", auckland, "MONDAY",
			"2026-10-23", "2026-10-23T11:00:00Z", "2026-10-19"},
		{"no zone is UTC", "2026-10-22T23:59:59Z", nil, "MONDAY",
			"2026-10-22", "2026-10-23T00:00:00Z", "2026-10-19"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now, _ := time.Parse(time.RFC3339, tc.now)
			window := NewFigureWindow(now, tc.location, tc.weekStart)
			if window.Today != tc.today {
				t.Errorf("today %s, want %s", window.Today, tc.today)
			}
			if got := window.DayEnd.UTC().Format(time.RFC3339); got != tc.dayEnd {
				t.Errorf("day ends %s, want %s", got, tc.dayEnd)
			}
			if window.WeekStartDate != tc.week {
				t.Errorf("week starts %s, want %s", window.WeekStartDate, tc.week)
			}
			if window.WeekStart.After(now) || !window.DayEnd.After(now) {
				t.Errorf("now %s is outside its own window", now)
			}
		})
	}
}

// What to count follows the structure in force: NONE and FLOW count nothing; DUE reads only the
// levels that carry a due date; FIELD_SUM sums only where the top level carries custom fields.
func TestTheFigureRequestFollowsTheStructureInForce(t *testing.T) {
	id := shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")
	bound := seededBound()
	resolve := func(values StructureValues) EffectiveStructure {
		return ResolveStructure(bound, WorkspaceStructure{}, LayerAt{Layer: StructureLayer{Values: values}})
	}

	for _, figure := range []Figure{FigureNone, FigureFlow} {
		if _, counted := FigureRequestFor(id, resolve(StructureValues{Counts: counting(figure)})); counted {
			t.Errorf("%s is counted", figure)
		}
	}

	activityUndated := map[ItemType]LevelFields{
		ItemTask:        {Capabilities: []Capability{CapabilityCompletion, CapabilityDueDate}},
		ItemWorkPackage: {Capabilities: []Capability{CapabilityCompletion, CapabilityDueDate}},
		ItemActivity:    {Capabilities: []Capability{CapabilityCompletion}},
	}
	dueStructure := resolve(StructureValues{Counts: counting(FigureDue), Fields: activityUndated})
	due, _ := FigureRequestFor(id, dueStructure)
	due = due.Offer(NewOffered(bound, dueStructure))
	if !slices.Equal(due.Offered, []ItemType{ItemTask, ItemWorkPackage, ItemActivity}) ||
		!slices.Equal(due.Dated, []ItemType{ItemTask, ItemWorkPackage}) {
		t.Errorf("due counts %v of %v", due.Dated, due.Offered)
	}

	sum := &Counts{Figure: FigureFieldSum, FieldKey: "amount"}
	withFields, _ := FigureRequestFor(id, resolve(StructureValues{Counts: sum}))
	if withFields.FieldKey != "amount" {
		t.Errorf("the sum's key %q", withFields.FieldKey)
	}
	withoutFields, _ := FigureRequestFor(id, resolve(StructureValues{Counts: sum, Fields: map[ItemType]LevelFields{
		ItemTask: {Capabilities: []Capability{CapabilityCompletion}},
	}}))
	if withoutFields.FieldKey != "" || withoutFields.Kind != FigureFieldSum {
		t.Errorf("a level without custom fields sums %q", withoutFields.FieldKey)
	}

	clipped, _ := FigureRequestFor(id, resolve(StructureValues{
		Counts: counting(FigureProgress), Levels: []Level{{Type: ItemTask}},
	}))
	if clipped.Kind != FigureOpen {
		t.Errorf("progress with one level is counted as %s", clipped.Kind)
	}
}

// The answer orders and deduplicates the days, and an unavailable figure answers the mark alone.
func TestACollectionFigureIsNumbersAndDates(t *testing.T) {
	window := NewFigureWindow(time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC), time.UTC, "MONDAY")
	request := FigureRequest{Kind: FigureRhythm}

	figure := NewCollectionFigure(request, window, FigureCounts{Days: []string{"2026-10-21", "2026-10-19", "2026-10-21"}})
	if !slices.Equal(figure.Counts.Days, []string{"2026-10-19", "2026-10-21"}) || figure.WeekStart != "2026-10-19" ||
		figure.Today != "2026-10-22" {
		t.Errorf("figure %+v", figure)
	}

	failed := UnavailableFigure(request, window)
	if !failed.Unavailable || failed.Kind != FigureRhythm || failed.Counts.Days != nil {
		t.Errorf("unavailable %+v", failed)
	}
	if JoinTypes([]ItemType{ItemTask, ItemActivity}) != "TASK,ACTIVITY" {
		t.Error("the type list")
	}
}
