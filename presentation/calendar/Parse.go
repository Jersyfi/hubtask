// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// The reading half of RFC 5545, as far as a VTODO a client writes back needs (P-07). Not a
// general parser: it unfolds the lines, walks into the one VTODO, reads the properties the
// product models and names the ones it does not, and skips the components inside (a VALARM is
// the client's own). A document without exactly one VTODO is refused.

// ParsedTodo is what a client's VTODO said.
type ParsedTodo struct {
	UID     string
	Summary string
	// Due is the due moment, or zero for none; DueDate says it was a DATE, and DueZone the
	// TZID it carried, if any. A DATE-TIME with a TZID is converted by the caller, who knows
	// which zone a name resolves to here.
	Due      time.Time
	DueDate  bool
	DueZone  string
	Start    time.Time
	StartSet bool
	// Completed says STATUS:COMPLETED or a COMPLETED stamp was present.
	Completed bool
	// Unsupported names the properties the product cannot keep: a client that wrote one would
	// read the entry back without it, which is the silent ignoring the contract forbids.
	Unsupported []string
}

var errNoTodo = errors.New("calendar: the document holds no VTODO, or more than one")

// unfold joins continuation lines (RFC 5545 §3.1): a CRLF or LF followed by a space or a tab.
func unfold(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\n ", "")
	raw = strings.ReplaceAll(raw, "\n\t", "")
	return strings.Split(raw, "\n")
}

// contentLine splits `NAME;PARAM=V;PARAM=V:VALUE` into its three parts. Parameters are read
// case-insensitively by name; a quoted parameter value keeps its content.
func contentLine(line string) (name string, params map[string]string, value string) {
	params = map[string]string{}
	// The colon that ends the name and parameters is the first one outside quotes.
	inQuotes := false
	split := -1
	for i, r := range line {
		switch {
		case r == '"':
			inQuotes = !inQuotes
		case r == ':' && !inQuotes:
			split = i
		}
		if split >= 0 {
			break
		}
	}
	if split < 0 {
		return strings.ToUpper(strings.TrimSpace(line)), params, ""
	}
	head, value := line[:split], line[split+1:]
	parts := strings.Split(head, ";")
	name = strings.ToUpper(strings.TrimSpace(parts[0]))
	for _, part := range parts[1:] {
		key, v, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		params[strings.ToUpper(strings.TrimSpace(key))] = strings.Trim(v, `"`)
	}
	return name, params, value
}

// unescapeText reverses escapeText.
func unescapeText(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) {
			i++
			switch value[i] {
			case 'n', 'N':
				out.WriteByte('\n')
			default:
				out.WriteByte(value[i])
			}
			continue
		}
		out.WriteByte(value[i])
	}
	return out.String()
}

// keptProperties are the properties the product models, or that say nothing a client would miss.
var keptProperties = map[string]bool{
	"UID": true, "SUMMARY": true, "DUE": true, "DTSTART": true, "STATUS": true, "COMPLETED": true,
	"PERCENT-COMPLETE": true, "DTSTAMP": true, "CREATED": true, "LAST-MODIFIED": true, "SEQUENCE": true,
	"URL": true, "CLASS": true, "TRANSP": true, "ORGANIZER": true,
}

// ParseTodo reads a client's document.
func ParseTodo(raw []byte) (ParsedTodo, error) {
	var todo ParsedTodo
	depth := 0
	found := 0
	inTodo := false
	for _, line := range unfold(string(raw)) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, params, value := contentLine(line)
		switch name {
		case "BEGIN":
			depth++
			if strings.EqualFold(value, "VTODO") && depth == 2 {
				inTodo = true
				found++
			}
			continue
		case "END":
			if strings.EqualFold(value, "VTODO") && depth == 2 {
				inTodo = false
			}
			depth--
			continue
		}
		// Only the todo's own lines: a VALARM inside it is the client's, and the calendar's
		// PRODID is nobody's business here.
		if !inTodo || depth != 2 {
			continue
		}
		switch name {
		case "UID":
			todo.UID = value
		case "SUMMARY":
			todo.Summary = unescapeText(value)
		case "DUE":
			at, isDate, err := parseDateTime(value, params)
			if err != nil {
				return todo, err
			}
			todo.Due, todo.DueDate, todo.DueZone = at, isDate, params["TZID"]
		case "DTSTART":
			at, _, err := parseDateTime(value, params)
			if err != nil {
				return todo, err
			}
			todo.Start, todo.StartSet = at, true
		case "STATUS":
			if strings.EqualFold(value, "COMPLETED") {
				todo.Completed = true
			}
		case "COMPLETED":
			if value != "" {
				todo.Completed = true
			}
		case "PERCENT-COMPLETE":
			if n, err := strconv.Atoi(value); err == nil && n >= 100 {
				todo.Completed = true
			}
		case "DESCRIPTION", "PRIORITY", "CATEGORIES", "LOCATION", "RRULE", "ATTACH", "GEO", "RELATED-TO", "DURATION":
			// A value that says nothing - an empty DESCRIPTION, PRIORITY:0 - is the client's
			// default, not a request to store something.
			if strings.TrimSpace(value) == "" || (name == "PRIORITY" && strings.TrimSpace(value) == "0") {
				continue
			}
			todo.Unsupported = append(todo.Unsupported, name)
		default:
			if strings.HasPrefix(name, "X-") || keptProperties[name] {
				continue
			}
			todo.Unsupported = append(todo.Unsupported, name)
		}
	}
	if found != 1 {
		return todo, errNoTodo
	}
	return todo, nil
}

// parseDateTime reads a DATE or a DATE-TIME value. A floating DATE-TIME (no Z, no TZID) is read
// in UTC, which is the reading RFC 5545 gives it on a server with no local time of its own.
func parseDateTime(value string, params map[string]string) (time.Time, bool, error) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(params["VALUE"], "DATE") || len(value) == 8 {
		at, err := time.Parse("20060102", value)
		if err != nil {
			return time.Time{}, false, err
		}
		return at, true, nil
	}
	if strings.HasSuffix(value, "Z") {
		at, err := time.Parse("20060102T150405Z", value)
		return at, false, err
	}
	at, err := time.Parse("20060102T150405", value)
	if err != nil {
		return time.Time{}, false, err
	}
	if zone := params["TZID"]; zone != "" {
		if loaded, err := time.LoadLocation(zone); err == nil {
			return time.Date(at.Year(), at.Month(), at.Day(), at.Hour(), at.Minute(), at.Second(), 0, loaded), false, nil
		}
	}
	return at, false, nil
}
