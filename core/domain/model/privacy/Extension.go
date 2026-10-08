// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"fmt"
	"time"
)

// ExtensionReason is why a deadline was extended. Art. 12(3) names two, and an extension that
// names neither is not one the law allows (data-protection.md §4.1).
type ExtensionReason string

const (
	ReasonComplexity       ExtensionReason = "COMPLEXITY"
	ReasonNumberOfRequests ExtensionReason = "NUMBER_OF_REQUESTS"
)

// ExtensionReasons returns both.
func ExtensionReasons() []ExtensionReason {
	return []ExtensionReason{ReasonComplexity, ReasonNumberOfRequests}
}

func (r ExtensionReason) Valid() bool {
	return r == ReasonComplexity || r == ReasonNumberOfRequests
}

// extensionMonths is the longest period Art. 12(3) allows in all, counted from receipt: the month
// a case gets plus the two further months an extension may add.
const extensionMonths = 3

// Day is a calendar day with no zone and no time of day.
//
// The extension speaks in days on purpose: the law names a date, a person is told a date, and a
// time of day would be precision nobody stated. A day becomes an instant in exactly one place,
// End, and only with the workspace's zone.
type Day struct {
	Year  int
	Month time.Month
	Day   int
}

// DayLayout is the one spelling a day travels in.
const DayLayout = "2006-01-02"

// ParseDay reads YYYY-MM-DD and refuses anything else, an impossible date included.
func ParseDay(raw string) (Day, error) {
	at, err := time.Parse(DayLayout, raw)
	if err != nil {
		return Day{}, err
	}
	return DayOf(at, time.UTC), nil
}

// DayOf is the calendar day an instant falls on in a zone.
func DayOf(at time.Time, zone *time.Location) Day {
	year, month, day := at.In(zone).Date()
	return Day{Year: year, Month: month, Day: day}
}

func (d Day) IsZero() bool { return d == Day{} }

func (d Day) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// Before reports whether d is an earlier day than other.
func (d Day) Before(other Day) bool { return d.compare(other) < 0 }

// After reports whether d is a later day than other.
func (d Day) After(other Day) bool { return d.compare(other) > 0 }

func (d Day) compare(other Day) int {
	switch {
	case d.Year != other.Year:
		return d.Year - other.Year
	case d.Month != other.Month:
		return int(d.Month) - int(other.Month)
	default:
		return d.Day - other.Day
	}
}

// End is the last second of the day in a zone: what a deadline named as a day means. A day of a
// daylight saving change is still the day the calendar shows, so the hour is built from the
// fields rather than added to midnight.
func (d Day) End(zone *time.Location) time.Time {
	return time.Date(d.Year, d.Month, d.Day, 23, 59, 59, 0, zone)
}

// Midnight is the day as the instant 00:00 UTC, which is how a zoneless date column stores it.
func (d Day) Midnight() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// addMonths is the same day of the month so many months later, or the last day of that month when
// it has no such day (Reg. 1182/71 Art. 3(2)(c)): 30 November and three months is 28 February,
// not 2 March - which is what time.AddDate answers, because it normalises the overflow forward.
func (d Day) addMonths(months int) Day {
	first := time.Date(d.Year, d.Month+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	day := d.Day
	if day > last {
		day = last
	}
	return Day{Year: first.Year(), Month: first.Month(), Day: day}
}

// ExtendInput is one extension as the controller states it.
type ExtendInput struct {
	// DueOn is the new deadline as a day; the case answers until the end of it in Zone.
	DueOn  Day
	Reason ExtensionReason
	// InformedOn is the day the controller told the person. Hubtask records it and writes to
	// nobody (data-protection.md §4.1).
	InformedOn Day
	// Zone is the workspace's own. The bounds are calendar days there, not in UTC: a month that
	// ends at midnight in Berlin does not end an hour later because the server counts in UTC.
	Zone *time.Location
	Now  time.Time
}

// Extended reports a case whose deadline was extended. The original deadline is the marker,
// because it is what an extension cannot leave out.
func (r Request) Extended() bool { return !r.OriginalDueAt.IsZero() }

// LatestExtension is the last day an extension may name: the day of receipt and three months, in
// the workspace's zone (data-protection.md §4.1).
func (r Request) LatestExtension(zone *time.Location) Day {
	return DayOf(r.ReceivedAt, zone).addMonths(extensionMonths)
}

// ExtendableUntil answers the last day an extension may name, and false when no extension could
// succeed now: the case is closed, already extended, past its deadline, or its deadline already
// lies at or beyond the bound. A screen that offers the extension only where this answers true
// never offers one the server will refuse (P-05).
func (r Request) ExtendableUntil(now time.Time, zone *time.Location) (Day, bool) {
	if r.Status.Closed() || r.Extended() || !now.Before(r.DueAt) {
		return Day{}, false
	}
	latest := r.LatestExtension(zone)
	if !latest.End(zone).After(r.DueAt) {
		return Day{}, false
	}
	return latest, true
}

// Extend moves the deadline once (Art. 12(3)).
//
// The deadline in force takes the new date and the original is kept beside it, so that every
// reader of the deadline - the register's order, the watch, the overdue count - reads the
// extended one without knowing an extension exists.
func (r Request) Extend(in ExtendInput) (Request, error) {
	zone := in.Zone
	if zone == nil {
		zone = time.UTC
	}
	switch {
	case r.Status.Closed():
		return Request{}, conflict(CodeRequestClosed, "/status")
	case r.Extended():
		return Request{}, conflict(CodeAlreadyExtended, "/due_on")
	case !in.Now.Before(r.DueAt):
		// The law allows the extension within the first month; once the deadline has passed
		// the case is late, and moving the date afterwards would hide that.
		return Request{}, conflict(CodeExtensionAfterDeadline, "/due_on")
	}
	if !in.Reason.Valid() {
		return Request{}, invalid(CodeExtensionReasonInvalid, "/reason")
	}

	latest := r.LatestExtension(zone)
	if in.DueOn.After(latest) {
		return Request{}, invalidWith(CodeExtensionTooLong, "/due_on",
			map[string]string{"latest": latest.String()})
	}
	due := in.DueOn.End(zone)
	if !due.After(r.DueAt) {
		return Request{}, invalid(CodeExtensionNotLater, "/due_on")
	}

	received, today := DayOf(r.ReceivedAt, zone), DayOf(in.Now, zone)
	if in.InformedOn.Before(received) || in.InformedOn.After(today) {
		return Request{}, invalidWith(CodeInformedOnOutOfRange, "/informed_on",
			map[string]string{"from": received.String(), "to": today.String()})
	}

	extended := r
	extended.OriginalDueAt = r.DueAt
	extended.DueAt = due
	extended.ExtensionReason = in.Reason
	extended.InformedOn = in.InformedOn
	return extended, nil
}
