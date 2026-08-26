// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package recurrence expands RFC 5545 rules through rrule-go (ADR-0008).
//
// The dependency lives here and nowhere else. ADR-0008 chose it over a home-grown implementation -
// "DST and edge cases are a well-known source of bugs" - and arc42 §11 R-07 names the same risk;
// what this package does is keep that choice reversible: the port is ours, the library is behind
// it, and a second implementation would be a second file rather than a change to the core.
package recurrence

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/teambition/rrule-go"

	port "github.com/Jersyfi/hubtask/core/port/recurrence"
)

// Expander is the adapter. It holds nothing: a rule is expanded from its text every time, because
// the text is what is stored and a cache would be a second copy of it that can disagree.
type Expander struct{}

func New() Expander { return Expander{} }

var _ port.Expander = Expander{}

// Occurrences reads the rule in its own zone and answers the moments it produces.
//
// The expansion runs on the **wall clock** and the zone is applied afterwards, one occurrence at a
// time. That is not a detail: it is what "DST is resolved through the stored time zone, not
// through UTC offsets" means once it is code (arc42 §6.3, i18n-l10n.md §4). A daily series at
// 09:00 is a series of 09:00s, and the instants between them are 23, 24 or 25 hours apart
// depending on what the zone did that night.
//
// Expanding with a zoned start instead produces a failure this project cannot have, and it was
// observed with this library before this shape was chosen: on a night when a zone springs forward
// at midnight (America/Sao_Paulo, 4 November 2018) the same local day comes back twice and the
// following one never appears at all. Expanding on the wall clock and mapping afterwards cannot do
// that - every reading appears exactly once, and the zone decides only which instant it is.
//
// Two readings need a rule of their own, and both are the standard library's rather than this
// adapter's, which is why they are stated here and pinned by the golden files: a local time that
// does not exist (02:30 on a spring-forward night) moves forward by the gap, and one that exists
// twice (02:30 on a fall-back night) is the second of the two.
func (e Expander) Occurrences(
	rule port.Rule, after, before time.Time, limit int,
) ([]time.Time, error) {
	location, err := time.LoadLocation(strings.TrimSpace(rule.TimeZone))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", port.ErrZoneUnknown, rule.TimeZone)
	}

	option, err := rrule.StrToROption(strings.TrimSpace(rule.RRULE))
	if err != nil {
		return nil, errors.Join(port.ErrRuleUnreadable, err)
	}
	// Every moment the expansion sees is a wall clock reading carried in UTC, the start and the
	// window included - a frame with no transitions in it, which is the only frame an RRULE's own
	// arithmetic is defined in.
	option.Dtstart = wallClock(rule.Start, location)
	if !rule.Until.IsZero() {
		option.Until = wallClock(rule.Until, location)
	}
	if rule.Count > 0 {
		option.Count = rule.Count
	}

	expanded, err := rrule.NewRRule(*option)
	if err != nil {
		// The library's own bounds check: a BYHOUR of 25, a BYMONTH of 13. Text that parsed and
		// still cannot be a rule is the same answer as text that did not parse, because it is the
		// same thing to whoever wrote it.
		return nil, errors.Join(port.ErrRuleUnreadable, err)
	}

	// Inclusive on both ends: a caller asking from the entry's due date expects that date to be
	// the first occurrence, and one asking to a horizon expects the horizon's own day to count.
	moments := expanded.Between(wallClock(after, location), wallClock(before, location), true)
	if limit > 0 && len(moments) > limit {
		moments = moments[:limit]
	}

	// Answered in UTC, which is how every instant in this system travels (i18n-l10n.md §7). This
	// is where the zone does its work: a reading becomes the instant it names in that place, on
	// that day.
	utc := make([]time.Time, 0, len(moments))
	for _, moment := range moments {
		utc = append(utc, inZone(moment, location).UTC())
	}
	return utc, nil
}

// wallClock reads an instant in the rule's zone and returns that reading carried in UTC: 09:00 in
// Berlin becomes 09:00Z, which is not the same moment and is not meant to be. It is the frame the
// expansion runs in.
func wallClock(at time.Time, location *time.Location) time.Time {
	local := at.In(location)
	return time.Date(
		local.Year(), local.Month(), local.Day(),
		local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), time.UTC)
}

// inZone is the way back: a reading becomes the moment it names in the zone. The standard library
// decides the two hard cases - a reading that does not exist moves forward by the gap, and one
// that happens twice is the second - and the golden files pin both.
func inZone(reading time.Time, location *time.Location) time.Time {
	return time.Date(
		reading.Year(), reading.Month(), reading.Day(),
		reading.Hour(), reading.Minute(), reading.Second(), reading.Nanosecond(), location)
}
