// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// dateLayout is how a figure spells a date: the reader's calendar day, ISO 8601.
const dateLayout = "2006-01-02"

// FigureWindow is the reader's day and week a figure is counted in (UC-WRK-29, D4 of UX-24's
// record): "due by today" and "this week" are the reader's, as `@today` and `@start_of_week` are
// (api-guidelines.md §3). Built from the Clock port's moment and the actor's zone and week start;
// the domain reads no clock.
type FigureWindow struct {
	// Now is the moment of counting.
	Now time.Time
	// Location is the reader's zone.
	Location *time.Location
	// Today is the reader's calendar date.
	Today string
	// DayEnd is the first instant of the reader's tomorrow: a timed due date before it is due by
	// today. Computed from the calendar, so a day of 23 or 25 hours ends where the clock says.
	DayEnd time.Time
	// WeekStart is the first instant of the reader's week, WeekStartDate its date.
	WeekStart     time.Time
	WeekStartDate string
}

// FlowWindow is how far back the flow figure looks: a week up to the moment of counting, rolling
// rather than the reader's calendar week, so that "a week" means the same on a Monday as on a
// Friday (UC-WRK-31 check 4).
const FlowWindow = 7 * 24 * time.Hour

// FlowSince is the first instant the flow figure counts a completion from.
func (w FigureWindow) FlowSince() time.Time { return w.Now.Add(-FlowWindow) }

// weekStarts is the account's vocabulary for a week's first day.
var weekStarts = map[string]time.Weekday{
	"MONDAY": time.Monday, "SUNDAY": time.Sunday, "SATURDAY": time.Saturday,
}

// NewFigureWindow answers the reader's day and week at a moment. A nil location is UTC; an empty or
// unknown week start is Monday, as the query anchors read it.
func NewFigureWindow(now time.Time, location *time.Location, weekStart string) FigureWindow {
	if location == nil {
		location = time.UTC
	}
	first, known := weekStarts[weekStart]
	if !known {
		first = time.Monday
	}
	local := now.In(location)
	year, month, day := local.Date()
	dayStart := time.Date(year, month, day, 0, 0, 0, 0, location)
	back := (int(local.Weekday()) - int(first) + 7) % 7
	weekBegins := time.Date(year, month, day-back, 0, 0, 0, 0, location)
	return FigureWindow{
		Now:           now,
		Location:      location,
		Today:         dayStart.Format(dateLayout),
		DayEnd:        time.Date(year, month, day+1, 0, 0, 0, 0, location),
		WeekStart:     weekBegins,
		WeekStartDate: weekBegins.Format(dateLayout),
	}
}

// FigureRequest is what one collection's figure is counted from: the kind in force and what the
// collection's structure offers. Built from the structure in force, never from a request.
type FigureRequest struct {
	CollectionID shared.ID
	Kind         Figure
	// Offered are the types whose entries are not hidden (Offered.Counted, set by Offer). An
	// entry of another type, or under an entry of another type, is hidden and counts nowhere
	// (domain-model.md §2, "Narrowing hides").
	Offered []ItemType
	// Dated are the offered types whose fields carry a due date: only their due dates count.
	Dated []ItemType
	// FieldKey is the number field FIELD_SUM sums, empty where the top level carries no custom
	// fields - the sum is then answered as unknown.
	FieldKey string
}

// FigureRequestFor answers what to count for a collection, or false where what counts is NONE.
func FigureRequestFor(collectionID shared.ID, effective EffectiveStructure) (FigureRequest, bool) {
	counts := effective.Values.Counts
	if counts == nil {
		return FigureRequest{}, false
	}
	switch counts.Figure {
	case FigureOpen, FigureFieldSum, FigureDue, FigureRhythm, FigureProgress, FigureFlow:
	default:
		return FigureRequest{}, false
	}
	request := FigureRequest{CollectionID: collectionID, Kind: counts.Figure}
	for _, level := range effective.Values.Levels {
		if slices.Contains(effective.Values.Fields[level.Type].Capabilities, CapabilityDueDate) {
			request.Dated = append(request.Dated, level.Type)
		}
	}
	top := ItemTask
	if counts.Figure == FigureFieldSum &&
		slices.Contains(effective.Values.Fields[top].Capabilities, CapabilityCustomFields) {
		request.FieldKey = counts.FieldKey
	}
	return request, true
}

// Offer sets the types the request counts from what the collection offers: the shared predicate,
// so that a figure leaves out exactly the entries a read marks hidden.
func (r FigureRequest) Offer(offered Offered) FigureRequest {
	r.Offered = offered.Counted()
	return r
}

// FigureCounts is what the store counted for one collection; every number is over the visible,
// untrashed entries.
type FigureCounts struct {
	// Open are the open, unarchived entries directly under the collection.
	Open int
	// Sum is the decimal sum of the field over those, nil where the field is no live number field.
	Sum *string
	// Summed is how many of them carry a value.
	Summed int
	// Due are the open, unarchived entries of a dated level due by the end of the reader's day.
	Due int
	// Done and Total are the completed and all leaves, archived ones included.
	Done, Total int
	// Days are the reader's dates this week with a completion.
	Days []string
	// Completed are the visible top-level entries completed since FlowSince, archived ones
	// included; AverageSeconds is their average time from creation to completion - from the
	// first column to the last, since a flow's entries start in its first column and its last
	// completes them - nil where none was completed.
	Completed      int
	AverageSeconds *float64
	// OverLimit are the board's columns holding more open, visible entries than their limit.
	OverLimit int
}

// CollectionFigure is a collection's figure as answered: numbers and dates, never words (P-12).
type CollectionFigure struct {
	Kind  Figure
	Today string
	// Unavailable is set where the count could not be made; nothing else is answered then.
	Unavailable bool
	Counts      FigureCounts
	WeekStart   string
}

// NewCollectionFigure assembles the answer for one collection.
func NewCollectionFigure(request FigureRequest, window FigureWindow, counts FigureCounts) CollectionFigure {
	days := slices.Clone(counts.Days)
	slices.Sort(days)
	counts.Days = slices.Compact(days)
	return CollectionFigure{Kind: request.Kind, Today: window.Today, Counts: counts, WeekStart: window.WeekStartDate}
}

// UnavailableFigure is the answer where the count failed: the kind, the day, and the mark.
func UnavailableFigure(request FigureRequest, window FigureWindow) CollectionFigure {
	return CollectionFigure{Kind: request.Kind, Today: window.Today, Unavailable: true}
}

// JoinTypes is a type list as one comma-separated parameter of the figure statement.
func JoinTypes(types []ItemType) string {
	names := make([]string, 0, len(types))
	for _, itemType := range types {
		names = append(names, string(itemType))
	}
	return strings.Join(names, ",")
}
