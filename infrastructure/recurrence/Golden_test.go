// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package recurrence_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/recurrence"
	"github.com/Jersyfi/hubtask/infrastructure/recurrence"
)

// The DST proof arc42 §11 R-07 asks for and §10.2 QS-07 names, as files rather than as assertions
// in prose (D-05).
//
// Files, for two reasons. A table in Go is read by whoever is changing the code; a file of local
// times and instants is read by whoever is asking what this system promises about a night when the
// clocks change - and the answer has to be legible without Go. And a change to the expansion shows
// up in a diff of expectations rather than in a rewritten test, which is the whole point of a
// golden file: the behaviour is the artefact, not the assertion.
//
// Every case states both sides of every occurrence: the local reading, which is what a person sees
// and what must not move, and the instant, which is what the server stores and what does move. A
// case where those two agree everywhere would be a case that proves nothing.

// goldenCase is one file.
type goldenCase struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	RRULE       string `json:"rrule"`
	TimeZone    string `json:"time_zone"`
	// Start is the wall clock reading the series counts from, in the case's own zone.
	Start string `json:"start"`
	// WindowDays is how far the expansion is asked to look.
	WindowDays  int    `json:"window_days"`
	Count       int    `json:"count"`
	Until       string `json:"until"`
	Occurrences []struct {
		Local string `json:"local"`
		UTC   string `json:"utc"`
	} `json:"occurrences"`
}

func TestTheGoldenCasesHoldAcrossEveryTransitionInThem(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden files under testdata: %v", err)
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			golden := readGolden(t, file)
			location, err := time.LoadLocation(golden.TimeZone)
			if err != nil {
				t.Fatalf("the zone does not load: %v", err)
			}

			start, err := time.ParseInLocation("2006-01-02T15:04:05", golden.Start, location)
			if err != nil {
				t.Fatalf("the start does not parse: %v", err)
			}
			rule := port.Rule{
				RRULE: golden.RRULE, TimeZone: golden.TimeZone, Start: start, Count: golden.Count,
			}
			if golden.Until != "" {
				until, err := time.ParseInLocation("2006-01-02T15:04:05", golden.Until, location)
				if err != nil {
					t.Fatalf("the end does not parse: %v", err)
				}
				rule.Until = until
			}

			moments, err := recurrence.New().Occurrences(
				rule, start, start.AddDate(0, 0, golden.WindowDays), len(golden.Occurrences)+5)
			if err != nil {
				t.Fatalf("expanding: %v", err)
			}

			if len(moments) != len(golden.Occurrences) {
				t.Fatalf("the expansion produced %d occurrences, the file expects %d:\n%s",
					len(moments), len(golden.Occurrences), render(moments, location))
			}
			for index, moment := range moments {
				want := golden.Occurrences[index]
				if got := moment.UTC().Format(time.RFC3339); got != want.UTC {
					t.Errorf("occurrence %d is %s, the file expects %s", index, got, want.UTC)
				}
				if got := moment.In(location).Format(time.RFC3339); got != want.Local {
					t.Errorf("occurrence %d reads as %s locally, the file expects %s",
						index, got, want.Local)
				}
			}
		})
	}
}

// render prints what an expansion produced in the shape a golden file carries, so that a failing
// case can be read and - once somebody has decided the new behaviour is right - copied.
func render(moments []time.Time, location *time.Location) string {
	rendered := ""
	for _, moment := range moments {
		rendered += "  " + moment.In(location).Format(time.RFC3339) +
			"  " + moment.UTC().Format(time.RFC3339) + "\n"
	}
	return rendered
}

func readGolden(t *testing.T, file string) goldenCase {
	t.Helper()

	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	var golden goldenCase
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	if golden.Description == "" {
		// A golden file whose expectations nobody can explain is a file that gets updated to
		// whatever the code now does.
		t.Fatalf("%s says what it expects and not why", file)
	}
	return golden
}
