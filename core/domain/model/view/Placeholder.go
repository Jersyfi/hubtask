// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A value a client cannot write down, because only the server knows it: who is asking, and when
// "today" is for them (api-guidelines.md §3, "resolved server-side in the actor's time zone").
//
// The point is that a saved view keeps meaning what it meant. `due_at LTE @end_of_week` is the
// same view next Tuesday; the same view with the date written out is a view that quietly went
// stale, and every client would have had to rewrite it before every request.

// placeholderPrefix is what marks one. A single character, and one that no identifier and no
// RFC 3339 timestamp can begin with - which is why a placeholder is recognised on those two kinds
// of field and on no other: in a title, `@` is somebody's text.
const placeholderPrefix = "@"

// PlaceholderKind is the anchor a placeholder computes from.
type PlaceholderKind string

const (
	// PlaceholderMe is the acting account. The one placeholder that is not a moment.
	PlaceholderMe PlaceholderKind = "@me"

	PlaceholderNow          PlaceholderKind = "@now"
	PlaceholderToday        PlaceholderKind = "@today"
	PlaceholderEndOfDay     PlaceholderKind = "@end_of_day"
	PlaceholderStartOfWeek  PlaceholderKind = "@start_of_week"
	PlaceholderEndOfWeek    PlaceholderKind = "@end_of_week"
	PlaceholderStartOfMonth PlaceholderKind = "@start_of_month"
	PlaceholderEndOfMonth   PlaceholderKind = "@end_of_month"
)

// Placeholder is an unresolved value: an anchor and, optionally, a shift away from it.
type Placeholder struct {
	Kind   PlaceholderKind
	Offset Offset
}

// Offset is the ISO 8601 duration a placeholder may carry, split the way it has to be applied.
//
// Calendar parts and clock parts are kept apart because they are different operations: a month is
// not a number of seconds, and a day is 23 or 25 hours twice a year in most time zones. `@today+P1D`
// means the same wall clock tomorrow, which is what somebody filtering by date expects, and
// `@now+PT24H` means twenty-four hours from now, which is what somebody filtering by elapsed time
// expects. Both are expressible, and neither is silently the other.
type Offset struct {
	Years  int
	Months int
	Days   int
	Clock  time.Duration
	// Negative flips the whole offset. Held separately rather than as negative parts, so that
	// `-P1M15D` cannot be read as "a month back and fifteen days on".
	Negative bool
}

// IsZero reports whether the placeholder shifts nothing.
func (o Offset) IsZero() bool {
	return o.Years == 0 && o.Months == 0 && o.Days == 0 && o.Clock == 0
}

// Resolution is what turns placeholders into values: the clock, the actor, and the zone the actor
// lives in.
//
// It is passed in rather than read here, because the domain reads no clock (arc42 §8.13). The use
// case takes the moment from the Clock port and the zone from the actor, and hands both over.
type Resolution struct {
	Now      time.Time
	Location *time.Location
	ActorID  shared.ID
}

// weekStart is the first day of a week for the week anchors.
//
// Monday, per ISO 8601, for every locale. The capability manifest has a `week_start` per locale
// and nothing answers it yet; when it does, this is the one line that reads it - which is the
// reason the anchor is computed here rather than in three places.
const weekStart = time.Monday

// parsePlaceholder reads `@anchor`, `@anchor+P3D` or `@anchor-P1W`, and refuses an anchor that
// does not fit the field it was written on.
func parsePlaceholder(raw string, kind Kind, path string) (Placeholder, error) {
	anchor, offsetText := raw, ""
	if cut := strings.IndexAny(raw, "+-"); cut > 0 {
		anchor, offsetText = raw[:cut], raw[cut:]
	}

	placeholder := Placeholder{Kind: PlaceholderKind(anchor)}
	if !placeholder.Kind.valid() {
		return Placeholder{}, fieldError(path, "query.placeholder_unknown", map[string]string{
			"placeholder": anchor,
		})
	}
	// `@me` on a date is as wrong as `@today` on an identifier, and both are worth saying plainly:
	// the alternative is a query that parses and then compares an account against a moment.
	//
	// A set of identifiers counts as identifiers. `members CONTAINS @me` is the query
	// api-guidelines.md §3 writes out in its own example, and the kind says "an identifier" rather
	// than "an account" - the same latitude `parent_id` already has, where `@me` parses and then
	// matches nothing.
	if (placeholder.Kind == PlaceholderMe) != (kind == KindID || kind == KindIDSet) {
		return Placeholder{}, fieldError(path, "query.placeholder_not_applicable", map[string]string{
			"placeholder": anchor, "kind": string(kind),
		})
	}

	if offsetText == "" {
		return placeholder, nil
	}
	if placeholder.Kind == PlaceholderMe {
		return Placeholder{}, fieldError(path, "query.placeholder_offset_not_allowed", map[string]string{
			"placeholder": anchor,
		})
	}

	offset, err := parseOffset(offsetText, path)
	if err != nil {
		return Placeholder{}, err
	}
	placeholder.Offset = offset
	return placeholder, nil
}

func (k PlaceholderKind) valid() bool {
	switch k {
	case PlaceholderMe, PlaceholderNow, PlaceholderToday, PlaceholderEndOfDay,
		PlaceholderStartOfWeek, PlaceholderEndOfWeek, PlaceholderStartOfMonth, PlaceholderEndOfMonth:
		return true
	}
	return false
}

// maxOffsetUnits bounds one component of an offset. A shift of a million years is not a query
// anybody means, and it is a way of pushing a timestamp past what the column can hold.
const maxOffsetUnits = 1000

// parseOffset reads the ISO 8601 duration subset a placeholder may carry: `PnYnMnWnD` and
// `TnHnMnS`, signed.
//
// A subset and not the whole of ISO 8601: fractional components exist in the standard and mean
// nothing useful here, and refusing them keeps the parser small enough to read in one sitting.
func parseOffset(raw, path string) (Offset, error) {
	malformed := func() (Offset, error) {
		return Offset{}, fieldError(path, "query.offset_malformed", nil)
	}

	offset := Offset{}
	switch {
	case strings.HasPrefix(raw, "-"):
		offset.Negative = true
		raw = raw[1:]
	case strings.HasPrefix(raw, "+"):
		raw = raw[1:]
	}
	if !strings.HasPrefix(raw, "P") || raw == "P" {
		return malformed()
	}
	raw = raw[1:]

	date, clock, hasClock := strings.Cut(raw, "T")
	if hasClock && clock == "" {
		return malformed()
	}

	for _, part := range []struct {
		text        string
		designators string
	}{{date, "YMWD"}, {clock, "HMS"}} {
		if err := eachComponent(part.text, part.designators, func(amount int, designator byte) {
			switch designator {
			case 'Y':
				offset.Years += amount
			case 'W':
				offset.Days += amount * 7
			case 'D':
				offset.Days += amount
			case 'H':
				offset.Clock += time.Duration(amount) * time.Hour
			case 'S':
				offset.Clock += time.Duration(amount) * time.Second
			case 'M':
				// The one designator that means two things. Before the T it is a month, after it a
				// minute - which is the whole reason the two halves are parsed with different
				// alphabets rather than in one pass.
				if part.designators == "YMWD" {
					offset.Months += amount
				} else {
					offset.Clock += time.Duration(amount) * time.Minute
				}
			}
		}); err != nil {
			return malformed()
		}
	}

	if offset.IsZero() {
		return malformed()
	}
	return offset, nil
}

// eachComponent walks `3D`, `1Y6M` and the like, refusing anything that is not a run of digits
// followed by one of the designators this half of the duration allows.
func eachComponent(text, designators string, apply func(amount int, designator byte)) error {
	for len(text) > 0 {
		digits := 0
		for digits < len(text) && text[digits] >= '0' && text[digits] <= '9' {
			digits++
		}
		if digits == 0 || digits >= len(text) {
			return errMalformedOffset
		}
		amount, err := strconv.Atoi(text[:digits])
		if err != nil || amount > maxOffsetUnits {
			return errMalformedOffset
		}

		designator := text[digits]
		if strings.IndexByte(designators, designator) < 0 {
			return errMalformedOffset
		}
		apply(amount, designator)
		text = text[digits+1:]
	}
	return nil
}

// errMalformedOffset never reaches a client: parseOffset turns every failure into the one field
// error, because "the duration is not one" is the whole of what a client can act on.
var errMalformedOffset = shared.ErrValidation.WithDetail("query.offset_malformed")

// Resolve computes the value a placeholder stands for.
func (p Placeholder) Resolve(at Resolution, path string) (Value, error) {
	if p.Kind == PlaceholderMe {
		if at.ActorID.IsZero() {
			// The automation engine and the retention job act as the system, which is nobody.
			// A rule filtering on `@me` is a rule whose author expected a person to be running it,
			// and answering it with "no rows" would hide that.
			return Value{}, fieldError(path, "query.placeholder_needs_actor", map[string]string{
				"placeholder": string(p.Kind),
			})
		}
		return Value{Kind: KindID, ID: at.ActorID}, nil
	}

	location := at.Location
	if location == nil {
		// UTC rather than the machine's zone. A server whose answer depended on the operating
		// system's configuration would answer differently in two replicas of the same deployment.
		location = time.UTC
	}
	moment, inclusiveEnd := p.anchor(at.Now.In(location), location)
	if !p.Offset.IsZero() {
		moment = p.Offset.apply(moment)
	}
	if inclusiveEnd {
		// The microsecond comes off after the shift, never before it. Taken first, `@end_of_month`
		// would be the 31st at 23:59:59.999999, and a month added to the 31st of a month with 30
		// days lands in the month after next - so `@end_of_month+P1M` would answer with a date
		// nobody asked about. Shifting the boundary and stepping back from it is exact for every
		// period length.
		moment = moment.Add(-resolution)
	}
	return Value{Kind: KindTimestamp, Time: moment.UTC()}, nil
}

// anchor is the moment the placeholder names, in the actor's zone, and whether that moment is the
// exclusive end of a period.
//
// An end is reported as the beginning of the *next* period. `LTE @end_of_month` has to mean the
// last instant of this one, so the caller steps one stored resolution back - and does it after any
// offset, which is what the second return value is for.
func (p Placeholder) anchor(now time.Time, location *time.Location) (time.Time, bool) {
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)

	switch p.Kind {
	case PlaceholderToday:
		return startOfDay, false
	case PlaceholderEndOfDay:
		return startOfDay.AddDate(0, 0, 1), true
	case PlaceholderStartOfWeek:
		return startOfDay.AddDate(0, 0, -daysSinceWeekStart(startOfDay)), false
	case PlaceholderEndOfWeek:
		return startOfDay.AddDate(0, 0, 7-daysSinceWeekStart(startOfDay)), true
	case PlaceholderStartOfMonth:
		return startOfMonth, false
	case PlaceholderEndOfMonth:
		return startOfMonth.AddDate(0, 1, 0), true
	}
	return now, false
}

// resolution is what one stored timestamp can distinguish: `timestamptz` keeps microseconds.
const resolution = time.Microsecond

func daysSinceWeekStart(day time.Time) int {
	return (int(day.Weekday()) - int(weekStart) + 7) % 7
}

// apply shifts a moment by the offset: the calendar parts first, then the clock.
//
// In that order deliberately. AddDate keeps the wall clock across a daylight saving boundary,
// which is what makes `@today+P1D` land on midnight tomorrow rather than at eleven at night; a
// clock offset added afterwards then means exactly what it says.
func (o Offset) apply(moment time.Time) time.Time {
	sign := 1
	if o.Negative {
		sign = -1
	}
	shifted := moment.AddDate(sign*o.Years, sign*o.Months, sign*o.Days)
	return shifted.Add(time.Duration(sign) * o.Clock)
}

// Resolve replaces every placeholder in the tree with the value it stands for.
//
// A new tree rather than a mutation: the parsed filter is what a saved view stores, and a resolve
// that rewrote it in place would turn "everything due this week" into "everything due in the week
// somebody first opened the view" the moment the two were kept together.
func (n Node) Resolve(at Resolution, path string) (Node, error) {
	if !n.IsLeaf() {
		resolved := Node{Op: n.Op, Nodes: make([]Node, 0, len(n.Nodes))}
		for index, child := range n.Nodes {
			value, err := child.Resolve(at, path+"/nodes/"+strconv.Itoa(index))
			if err != nil {
				return Node{}, err
			}
			resolved.Nodes = append(resolved.Nodes, value)
		}
		return resolved, nil
	}

	resolved := Node{Op: n.Op, Field: n.Field, Values: make([]Value, 0, len(n.Values))}
	for index, value := range n.Values {
		if value.IsPlaceholder() {
			computed, err := value.Placeholder.Resolve(at, path+"/value/"+strconv.Itoa(index))
			if err != nil {
				return Node{}, err
			}
			value = computed
		}
		resolved.Values = append(resolved.Values, value)
	}
	return resolved, nil
}
