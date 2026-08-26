// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendar_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/presentation/calendar"
)

// The .ics files this package promises, as files rather than as assertions in prose (D-08).
//
// A golden file for the reason the recurrence expander has them: what this system hands a
// calendar client is read by people who do not read Go, and a change to the rendering shows up as
// a diff of the document rather than as a rewritten test. The files are the artefact.

var update = flag.Bool("update", false, "rewrite the golden files from the current rendering")

func golden(t *testing.T, name string, produced []byte) {
	t.Helper()

	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, produced, 0o600); err != nil {
			t.Fatalf("writing the golden file: %v", err)
		}
		return
	}

	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the golden file: %v", err)
	}
	if !bytes.Equal(expected, produced) {
		t.Errorf("the rendering changed.\n--- want ---\n%s\n--- got ---\n%s",
			visible(expected), visible(produced))
	}
}

// visible makes the line endings legible in a failure. CRLF is part of what is being asserted,
// and a diff that hides it is a diff nobody can act on.
func visible(document []byte) string {
	return strings.ReplaceAll(string(document), "\r\n", "\\r\\n\n")
}

func at(t *testing.T, text string) time.Time {
	t.Helper()
	moment, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatalf("the fixture is not a moment: %v", err)
	}
	return moment
}

// The three shapes the acceptance names, in one document: an all-day entry, a timed one, and two
// occurrences of one series on either side of a daylight saving transition.
//
// The last pair is the point. Berlin puts its clocks back on 25 October 2026, so 09:00 local is
// 07:00Z before it and 08:00Z after - and both entries have to come out at 09:00 in the calendar
// that reads them. The UTC form is what makes that true without a VTIMEZONE component: the file
// states two instants, and the client does the arithmetic it already knows how to do.
func TestTheGoldenCalendar(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("Europe/Berlin is unknown here: %v", err)
	}

	document := calendar.Calendar{
		Name:        "This week",
		GeneratedAt: at(t, "2026-10-20T06:00:00Z"),
		Events: []calendar.Event{
			{
				UID:     "018f2a1b-0000-7000-8000-000000000001@hubtask",
				Summary: "Pack the kitchen",
				// An all-day entry: the day, carried as its own wall clock in UTC.
				Start:        time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC),
				AllDay:       true,
				URL:          "https://hubtask.example.com/items/018f2a1b-0000-7000-8000-000000000001",
				LastModified: at(t, "2026-10-19T14:30:00Z"),
			},
			{
				UID:          "018f2a1b-0000-7000-8000-000000000002@hubtask",
				Summary:      "Call the landlord",
				Start:        at(t, "2026-10-22T14:30:00Z"),
				URL:          "https://hubtask.example.com/items/018f2a1b-0000-7000-8000-000000000002",
				LastModified: at(t, "2026-10-19T14:30:00Z"),
			},
			{
				UID:     "018f2a1b-0000-7000-8000-000000000003@hubtask",
				Summary: "Water the plants",
				// 09:00 in Berlin, the Friday before the clocks go back.
				Start: time.Date(2026, 10, 23, 9, 0, 0, 0, berlin).UTC(),
				URL:   "https://hubtask.example.com/items/018f2a1b-0000-7000-8000-000000000003",
			},
			{
				UID:     "018f2a1b-0000-7000-8000-000000000004@hubtask",
				Summary: "Water the plants",
				// 09:00 in Berlin, the Monday after. One hour later as an instant, the same
				// morning to the person reading it.
				Start: time.Date(2026, 10, 26, 9, 0, 0, 0, berlin).UTC(),
				URL:   "https://hubtask.example.com/items/018f2a1b-0000-7000-8000-000000000004",
			},
		},
	}

	produced := calendar.Render(document)
	golden(t, "week.ics", produced)

	// The transition, stated here as well as in the file, because a golden file that was updated
	// carelessly would otherwise take this promise with it.
	if !strings.Contains(string(produced), "DTSTART:20261023T070000Z") ||
		!strings.Contains(string(produced), "DTSTART:20261026T080000Z") {
		t.Error("the two sides of the transition are not 07:00Z and 08:00Z")
	}
}

// A title is free text, and free text is where a serialiser gets broken: a comma ends a value, a
// backslash escapes the next character, and a newline ends the line. All three travel escaped,
// and a title long enough to pass the fold width is broken with a continuation line.
func TestTitlesAreEscapedAndLongLinesAreFolded(t *testing.T) {
	document := calendar.Calendar{
		Name:        "Everything; all of it",
		GeneratedAt: at(t, "2026-10-20T06:00:00Z"),
		Events: []calendar.Event{
			{
				UID:     "018f2a1b-0000-7000-8000-000000000005@hubtask",
				Summary: "Buy milk, bread; and \\ then\nring Ada",
				Start:   at(t, "2026-10-22T14:30:00Z"),
			},
			{
				UID:     "018f2a1b-0000-7000-8000-000000000006@hubtask",
				Summary: strings.Repeat("a very long title ", 8),
				Start:   at(t, "2026-10-22T15:30:00Z"),
			},
			{
				UID: "018f2a1b-0000-7000-8000-000000000007@hubtask",
				// Multi-byte, so that the fold has to find a rune boundary rather than an octet.
				Summary: strings.Repeat("Grüße an die Nachbarn ", 5),
				Start:   at(t, "2026-10-22T16:30:00Z"),
			},
		},
	}

	produced := calendar.Render(document)
	golden(t, "escaping.ics", produced)

	for _, line := range strings.Split(string(produced), "\r\n") {
		if len(line) > 75 {
			t.Errorf("a line of %d octets was not folded: %q", len(line), line)
		}
	}
	// Unfolded again, the document has to be the text it started as - a fold that cut a
	// multi-byte character in half would leave two invalid bytes behind.
	if unfolded := strings.ReplaceAll(string(produced), "\r\n ", ""); !utf8.ValidString(unfolded) {
		t.Error("the fold broke a character in half")
	}
}

// An empty calendar is still a calendar: a client that subscribed to a view with nothing due in
// it has to get a document rather than an error.
func TestAnEmptyCalendarIsStillADocument(t *testing.T) {
	produced := string(calendar.Render(calendar.Calendar{
		GeneratedAt: at(t, "2026-10-20T06:00:00Z"),
	}))

	if !strings.HasPrefix(produced, "BEGIN:VCALENDAR\r\n") ||
		!strings.HasSuffix(produced, "END:VCALENDAR\r\n") {
		t.Errorf("the document is %q", produced)
	}
	if strings.Contains(produced, "VEVENT") {
		t.Error("an empty calendar carries an event")
	}
	if strings.Contains(produced, "X-WR-CALNAME") {
		t.Error("a calendar with no name carries an empty one")
	}
}
