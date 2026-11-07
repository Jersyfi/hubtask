// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar

import (
	"strconv"
	"strings"
	"time"
)

// A zone a client defines in its own document (RFC 5545 §3.6.5): Outlook and others name a zone
// the time zone database does not know ("W. Europe Standard Time", "Customized Time Zone") and send
// its VTIMEZONE along. What is read is the subset such clients write: observances with a DTSTART,
// both offsets and at most a yearly RRULE by month and weekday. Anything else - an RDATE, another
// rule - leaves the zone unreadable, and a due in it is refused as before rather than guessed.

// observance is one STANDARD or DAYLIGHT block: from its onset on, the zone is `to` seconds east.
type observance struct {
	start    time.Time // the onset's wall clock, as written
	from, to int
	ruled    bool
	month    time.Month
	nth      int // the nth weekday of the month; negative counts from its end
	weekday  time.Weekday
	until    time.Time
}

// definedZone is a client's VTIMEZONE.
type definedZone struct {
	observances []observance
	unreadable  bool
}

// definedZones collects the document's VTIMEZONE blocks by TZID.
func definedZones(lines []string) map[string]definedZone {
	zones := map[string]definedZone{}
	var (
		tzid    string
		zone    *definedZone
		current *observance
		set     map[string]bool
	)
	for _, line := range lines {
		name, params, value := contentLine(line)
		switch {
		case name == "BEGIN" && strings.EqualFold(value, "VTIMEZONE"):
			tzid, zone = "", &definedZone{}
		case zone == nil:
			continue
		case name == "END" && strings.EqualFold(value, "VTIMEZONE"):
			if tzid != "" {
				zones[tzid] = *zone
			}
			zone = nil
		case name == "BEGIN" && (strings.EqualFold(value, "STANDARD") || strings.EqualFold(value, "DAYLIGHT")):
			current, set = &observance{}, map[string]bool{}
		case name == "END" && current != nil:
			if !set["DTSTART"] || !set["TZOFFSETFROM"] || !set["TZOFFSETTO"] {
				zone.unreadable = true
			}
			zone.observances = append(zone.observances, *current)
			current = nil
		case name == "TZID" && current == nil:
			tzid = value
		case current == nil:
			continue
		default:
			set[name] = true
			if !current.read(name, params, value) {
				zone.unreadable = true
			}
		}
	}
	return zones
}

// read takes one property of an observance; false is one this reader cannot follow.
func (o *observance) read(name string, params map[string]string, value string) bool {
	var ok bool
	switch name {
	case "DTSTART":
		at, err := time.Parse("20060102T150405", strings.TrimSpace(value))
		o.start, ok = at, err == nil && params["TZID"] == ""
	case "TZOFFSETFROM":
		o.from, ok = offsetOf(value)
	case "TZOFFSETTO":
		o.to, ok = offsetOf(value)
	case "RRULE":
		ok = o.rule(value)
	case "RDATE":
		ok = false
	default:
		ok = true
	}
	return ok
}

// rule reads FREQ=YEARLY;BYMONTH=m;BYDAY=nWD[;UNTIL=…], the one rule zone definitions use.
func (o *observance) rule(value string) bool {
	parts := map[string]string{}
	for _, part := range strings.Split(strings.TrimSpace(value), ";") {
		key, v, _ := strings.Cut(part, "=")
		parts[strings.ToUpper(key)] = strings.ToUpper(v)
	}
	month, err := strconv.Atoi(parts["BYMONTH"])
	day := parts["BYDAY"]
	if parts["FREQ"] != "YEARLY" || err != nil || month < 1 || month > 12 || len(day) < 3 {
		return false
	}
	weekday, known := weekdays[day[len(day)-2:]]
	nth := 1
	if prefix := day[:len(day)-2]; prefix != "" {
		if nth, err = strconv.Atoi(prefix); err != nil || nth == 0 || nth < -5 || nth > 5 {
			return false
		}
	}
	for key := range parts {
		switch key {
		case "FREQ", "BYMONTH", "BYDAY", "UNTIL", "INTERVAL", "WKST":
		default:
			return false
		}
	}
	if interval, given := parts["INTERVAL"]; given && interval != "1" {
		return false
	}
	if until := parts["UNTIL"]; until != "" {
		at, err := time.Parse("20060102T150405Z", until)
		if err != nil {
			return false
		}
		o.until = at
	}
	o.ruled, o.month, o.nth, o.weekday = true, time.Month(month), nth, weekday
	return known
}

var weekdays = map[string]time.Weekday{
	"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday,
	"TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday,
}

// offsetOf reads ±HHMM or ±HHMMSS as seconds east.
func offsetOf(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if (len(value) != 5 && len(value) != 7) || (value[0] != '+' && value[0] != '-') {
		return 0, false
	}
	digits := value[1:] + "00"
	hours, err1 := strconv.Atoi(digits[0:2])
	minutes, err2 := strconv.Atoi(digits[2:4])
	seconds, err3 := strconv.Atoi(digits[4:6])
	if err1 != nil || err2 != nil || err3 != nil || hours > 23 || minutes > 59 || seconds > 59 {
		return 0, false
	}
	offset := hours*3600 + minutes*60 + seconds
	if value[0] == '-' {
		offset = -offset
	}
	return offset, true
}

// onsets are the wall clocks the observance begins at in the year (in the offset before it).
func (o observance) onset(year int) (time.Time, bool) {
	if !o.ruled {
		return o.start, o.start.Year() == year
	}
	if year < o.start.Year() {
		return time.Time{}, false
	}
	var day time.Time
	if o.nth > 0 {
		first := time.Date(year, o.month, 1, 0, 0, 0, 0, time.UTC)
		day = first.AddDate(0, 0, (int(o.weekday)-int(first.Weekday())+7)%7+7*(o.nth-1))
	} else {
		last := time.Date(year, o.month+1, 0, 0, 0, 0, 0, time.UTC)
		day = last.AddDate(0, 0, -((int(last.Weekday())-int(o.weekday)+7)%7)-7*(-o.nth-1))
	}
	if day.Month() != o.month {
		return time.Time{}, false
	}
	at := time.Date(year, o.month, day.Day(), o.start.Hour(), o.start.Minute(), o.start.Second(), 0, time.UTC)
	if !o.until.IsZero() && at.Add(-time.Duration(o.from)*time.Second).After(o.until) {
		return time.Time{}, false
	}
	return at, true
}

// offsetAt answers the offset of a wall clock: that of the observance begun last before it, else
// the offset the first observance changes from.
func (z definedZone) offsetAt(wall time.Time) (int, bool) {
	if z.unreadable || len(z.observances) == 0 {
		return 0, false
	}
	var (
		latest time.Time
		offset = z.observances[0].from
		found  bool
	)
	for _, o := range z.observances {
		for _, year := range []int{wall.Year() - 1, wall.Year()} {
			at, ok := o.onset(year)
			if ok && !at.After(wall) && (!found || at.After(latest)) {
				latest, offset, found = at, o.to, true
			}
		}
	}
	return offset, true
}
