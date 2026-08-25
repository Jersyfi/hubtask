// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package recurrence is the port for reading an RFC 5545 recurrence rule and turning it into the
// moments it means (ADR-0008, arc42 §11 R-07).
//
// It exists because the expansion is a library's job and the domain may have no libraries
// (ADR-0001, rule 1): "RRULE plus time zones plus DST is error-prone" is the risk R-07 names, and
// the answer it names is a proven implementation rather than our own. What the domain and the
// application see is this interface - a rule goes in, moments come out - so the library is one
// adapter and not a type spread through the core.
//
// Every moment that comes out is an instant. The zone is in the rule rather than in the answer,
// because that is where daylight saving is resolved: a weekly series at 09:00 in Europe/Berlin is
// 07:00 UTC in winter and 08:00 in summer, and an expander that answered in offsets would have to
// be told which one.
package recurrence

import (
	"errors"
	"time"
)

// Rule is a series as the expander takes it: what repeats, where it is read, and what it counts
// from.
type Rule struct {
	// RRULE is the rule text, without a DTSTART and without an end - both of those are fields of
	// their own here, because the entry's due date is the start and the end spec is the end
	// (domain-model.md §3.5).
	RRULE string
	// TimeZone is the IANA name the rule is read in. Required: a rule without one cannot resolve
	// a transition, which is the whole reason this port exists.
	TimeZone string
	// Start is the moment the series counts from - the entry's due date.
	Start time.Time
	// Until is when the series stops, and the zero time for a series that does not.
	Until time.Time
	// Count is how many occurrences the series produces at most, and zero for no limit.
	Count int
}

// The reasons a rule cannot be expanded. Sentinels rather than message codes: which field a
// refusal points at and what a client is told are the application's answers, and an adapter that
// produced them would be writing the API's error surface (ADR-0011).
var (
	// ErrRuleUnreadable is text that is not a recurrence rule, or one whose parts are out of
	// bounds - a BYHOUR of 25, a frequency nobody defined.
	ErrRuleUnreadable = errors.New("the recurrence rule cannot be read")
	// ErrZoneUnknown is a time zone this installation's tzdata does not have.
	ErrZoneUnknown = errors.New("the time zone is not one this installation knows")
)

// Expander answers what a rule means.
type Expander interface {
	// Occurrences returns the moments the rule produces in [after, before], in order, and at most
	// limit of them.
	//
	// The limit is not a page: it is the bound that keeps a rule from becoming a denial of service
	// (security.md T-17). A caller that wants to know whether a rule is too dense asks for one
	// more than it will accept and looks at what it gets back.
	Occurrences(rule Rule, after, before time.Time, limit int) ([]time.Time, error)
}
