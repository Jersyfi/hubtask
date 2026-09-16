// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar

import (
	"strings"
	"time"
)

// Todo is one entry as a CalDAV calendar of todos shows it (P-06, RFC 5545 §3.6.2).
//
// A VTODO rather than a VEVENT: an entry with a due date is something to do, and a calendar
// client that speaks CalDAV is a client that shows todos - Reminders, Tasks, Thunderbird's
// task pane - with a checkbox it ticks and writes back. As minimal as the feed's event, for
// the same reason (data-protection.md §9): a title, when it is due, whether it is done, and a
// way back into the product. Not the notes, not the assignee, not the comments.
type Todo struct {
	UID     string
	Summary string
	// Due is when it is due, or the zero time for an entry without a date. The convention is
	// Event.Start's: an instant for a timed entry, the day's wall clock in UTC for an all-day one.
	Due    time.Time
	AllDay bool
	// Completed is when it was done, or the zero time while it is open.
	Completed time.Time
	// PercentComplete is the roll-up over the children, 0 to 100, or -1 for an entry that has
	// none - the property is then left out rather than written as zero, which would say "not
	// started" about an entry that is simply a leaf.
	PercentComplete int
	URL             string
	Created         time.Time
	LastModified    time.Time
}

// RenderTodo writes one todo as its own calendar object - what a CalDAV client GETs at the
// member's address, and what a calendar-query REPORT carries per member.
func RenderTodo(todo Todo, generatedAt time.Time) []byte {
	var out strings.Builder
	write(&out, "BEGIN:VCALENDAR")
	write(&out, "VERSION:2.0")
	write(&out, "PRODID:"+productID)
	write(&out, "CALSCALE:GREGORIAN")
	writeTodo(&out, todo, timestamp(generatedAt))
	write(&out, "END:VCALENDAR")
	return []byte(out.String())
}

func writeTodo(out *strings.Builder, todo Todo, stamp string) {
	write(out, "BEGIN:VTODO")
	write(out, "UID:"+escapeText(todo.UID))
	write(out, "DTSTAMP:"+stamp)
	write(out, "SUMMARY:"+escapeText(todo.Summary))
	if !todo.Due.IsZero() {
		if todo.AllDay {
			write(out, "DUE;VALUE=DATE:"+date(todo.Due))
		} else {
			write(out, "DUE:"+timestamp(todo.Due))
		}
	}
	// STATUS and COMPLETED travel together: RFC 5545 §3.8.1.11 gives COMPLETED to a todo that
	// is done, and a client that reads one without the other shows a ticked box with no date or
	// a date with an open box.
	if todo.Completed.IsZero() {
		write(out, "STATUS:NEEDS-ACTION")
	} else {
		write(out, "STATUS:COMPLETED")
		write(out, "COMPLETED:"+timestamp(todo.Completed))
	}
	if todo.PercentComplete >= 0 {
		write(out, "PERCENT-COMPLETE:"+itoa(todo.PercentComplete))
	}
	if todo.URL != "" {
		write(out, "URL:"+escapeText(todo.URL))
	}
	if !todo.Created.IsZero() {
		write(out, "CREATED:"+timestamp(todo.Created))
	}
	if !todo.LastModified.IsZero() {
		write(out, "LAST-MODIFIED:"+timestamp(todo.LastModified))
	}
	write(out, "END:VTODO")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
