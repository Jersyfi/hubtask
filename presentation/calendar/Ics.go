// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package calendar renders RFC 5545 documents (D-08).
//
// It is an inbound adapter's business, not the core's: an .ics file is a wire format, and the
// application layer answers entries rather than files (project-structure.md §3, which names this
// directory). Two callers share it - the public feed route and the view export - which is why it
// is a package rather than a method on either of them.
//
// Nothing here reads a locale. The document carries a title somebody wrote, two timestamps and a
// link, and no sentence this server composed, so the exception i18n-l10n.md §1 grants the feed
// against rule 8 is not needed: there is no display text to render.
package calendar

import (
	"strings"
	"time"
)

// productID identifies the writer, as RFC 5545 §3.7.3 requires. No version: it would say which
// build an installation runs to anybody holding a feed URL (security.md §9).
const productID = "-//Hubtask//Hubtask//EN"

// lineOctets is the fold width of RFC 5545 §3.1 - 75 octets plus the line break. Folding is not
// decoration: a title long enough to exceed it produces a file some clients refuse to read.
const lineOctets = 75

// Event is one entry with a date, as a calendar shows it.
//
// Minimal on purpose (data-protection.md §9): a title, when it is due, and a way back into the
// product. Not the notes, not the assignee, not the comments - a calendar entry is read on
// devices and lock screens nobody in this workspace controls.
type Event struct {
	// UID identifies the entry across fetches, so a client updates rather than duplicates.
	UID string
	// Summary is the entry's title, which is the one piece of user content in the document.
	Summary string
	// Start is when it is due. For a timed entry it is the instant, written in UTC; for an
	// all-day one it is the day, carried as that day's wall clock in UTC - the convention the
	// recurrence expander uses for the same reason, that a date is not an instant.
	Start time.Time
	// AllDay decides which of the two Start is, and therefore whether the entry is written as a
	// DATE or as a UTC DATE-TIME.
	AllDay bool
	// URL is the link back to the entry.
	URL string
	// LastModified is when the entry last changed, which is what lets a client tell a refetch
	// that changed something from one that did not.
	LastModified time.Time
}

// Calendar is the document: what it is called, and what is in it.
type Calendar struct {
	// Name is what a client labels the subscription with - the saved view's name, which is user
	// content and travels as such.
	Name string
	// GeneratedAt stamps every entry (DTSTAMP is required by RFC 5545 §3.8.7.2). One moment for
	// the whole document, because it is one document.
	GeneratedAt time.Time
	Events      []Event
}

// Render writes the document.
//
// Timed entries are written as UTC instants rather than with a TZID. A due date is one moment -
// that is what the column holds - and the UTC form says exactly that moment with no VTIMEZONE
// component to get wrong; the client shows it in whatever zone the person reading is in, which is
// how a daylight saving transition comes out right on both sides of it (i18n-l10n.md §4). An
// all-day entry is a DATE and carries no zone at all, because a day is a day wherever it is read.
//
// Neither carries a DTEND. RFC 5545 §3.6.1 gives a DATE-valued start with no end a duration of
// one day and a DATE-TIME one a duration of nothing, which is precisely what a due date means:
// the day it is due, or the moment it is due.
func Render(document Calendar) []byte {
	var out strings.Builder

	write(&out, "BEGIN:VCALENDAR")
	write(&out, "VERSION:2.0")
	write(&out, "PRODID:"+productID)
	// Gregorian whatever the reader's display calendar. Stated rather than left to the default,
	// because it is the one calendar RFC 5545 defines and i18n-l10n.md §4 says so out loud.
	write(&out, "CALSCALE:GREGORIAN")
	write(&out, "METHOD:PUBLISH")
	if document.Name != "" {
		// Non-standard and near-universal: it is how a subscription gets a name in Apple
		// Calendar, Google Calendar and Thunderbird alike. A client that does not know it
		// ignores it, which is the worst it can do.
		write(&out, "X-WR-CALNAME:"+escapeText(document.Name))
	}

	stamp := timestamp(document.GeneratedAt)
	for _, event := range document.Events {
		write(&out, "BEGIN:VEVENT")
		write(&out, "UID:"+escapeText(event.UID))
		write(&out, "DTSTAMP:"+stamp)
		if event.AllDay {
			write(&out, "DTSTART;VALUE=DATE:"+date(event.Start))
		} else {
			write(&out, "DTSTART:"+timestamp(event.Start))
		}
		write(&out, "SUMMARY:"+escapeText(event.Summary))
		if event.URL != "" {
			write(&out, "URL:"+escapeText(event.URL))
		}
		if !event.LastModified.IsZero() {
			write(&out, "LAST-MODIFIED:"+timestamp(event.LastModified))
		}
		write(&out, "END:VEVENT")
	}
	write(&out, "END:VCALENDAR")

	return []byte(out.String())
}

// write appends one content line, folded and terminated as RFC 5545 §3.1 requires.
func write(out *strings.Builder, line string) {
	out.WriteString(fold(line))
	out.WriteString("\r\n")
}

// fold breaks a long line, continuing it with a single leading space. It counts octets rather
// than runes, which is what the specification says, and never breaks inside a UTF-8 sequence -
// a split multi-byte character would be a file the client cannot decode.
func fold(line string) string {
	if len(line) <= lineOctets {
		return line
	}

	var folded strings.Builder
	remaining := line
	limit := lineOctets
	for len(remaining) > limit {
		cut := limit
		// Back off to a rune boundary. The continuation byte test is the whole of it: bytes of
		// the form 10xxxxxx are never the start of a character.
		for cut > 0 && remaining[cut]&0xC0 == 0x80 {
			cut--
		}
		folded.WriteString(remaining[:cut])
		folded.WriteString("\r\n ")
		remaining = remaining[cut:]
		// A continuation line spends one octet on the leading space.
		limit = lineOctets - 1
	}
	folded.WriteString(remaining)
	return folded.String()
}

// escapeText escapes a TEXT value as RFC 5545 §3.3.11 requires. Titles are free text: a title
// with a comma in it would otherwise end the value and start a second one.
func escapeText(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		";", `\;`,
		",", `\,`,
		"\r\n", `\n`,
		"\n", `\n`,
		"\r", `\n`,
	)
	return replacer.Replace(value)
}

// timestamp writes a UTC DATE-TIME: 20260910T070000Z.
func timestamp(at time.Time) string { return at.UTC().Format("20060102T150405Z") }

// date writes a DATE. The moment is read in UTC because that is how an all-day entry arrives
// here - the day, carried as its own wall clock.
func date(at time.Time) string { return at.UTC().Format("20060102") }
