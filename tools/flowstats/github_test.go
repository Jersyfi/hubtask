// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/tools/losttests"
)

func at(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return t
}

// The answer of the runs API for one day, reduced to the fields read and to one run of each kind.
const runsAnswer = `{"total_count": 8, "workflow_runs": [
 {"id": 1, "event": "pull_request", "run_attempt": 2, "status": "completed", "conclusion": "failure",
  "created_at": "2026-11-15T18:20:21Z", "run_started_at": "2026-11-15T18:41:44Z", "updated_at": "2026-11-15T18:58:49Z"},
 {"id": 2, "event": "pull_request", "run_attempt": 1, "status": "completed", "conclusion": "skipped",
  "created_at": "2026-11-15T18:20:11Z", "run_started_at": "2026-11-15T18:20:11Z", "updated_at": "2026-11-15T18:20:12Z"},
 {"id": 3, "event": "pull_request", "run_attempt": 1, "status": "completed", "conclusion": "cancelled",
  "created_at": "2026-11-16T14:44:56Z", "run_started_at": "2026-11-16T14:44:56Z", "updated_at": "2026-11-16T15:09:04Z"},
 {"id": 4, "event": "push", "run_attempt": 2, "status": "completed", "conclusion": "success",
  "created_at": "2026-11-16T15:38:55Z", "run_started_at": "2026-11-18T13:20:26Z", "updated_at": "2026-11-18T13:41:45Z"},
 {"id": 5, "event": "pull_request", "run_attempt": 1, "status": "completed", "conclusion": "timed_out",
  "created_at": "2026-11-16T10:00:00Z", "run_started_at": "2026-11-16T10:00:00Z", "updated_at": "2026-11-16T10:30:00Z"},
 {"id": 6, "event": "push", "run_attempt": 1, "status": "completed", "conclusion": "startup_failure",
  "created_at": "2026-11-16T11:00:00Z", "run_started_at": "2026-11-16T11:00:00Z", "updated_at": "2026-11-16T11:00:05Z"},
 {"id": 7, "event": "workflow_dispatch", "run_attempt": 1, "status": "completed", "conclusion": "success",
  "created_at": "2026-11-16T12:00:00Z", "run_started_at": "2026-11-16T12:00:00Z", "updated_at": "2026-11-16T12:20:00Z"},
 {"id": 8, "event": "pull_request", "run_attempt": 1, "status": "in_progress", "conclusion": null,
  "created_at": "2026-11-16T13:00:00Z", "run_started_at": "2026-11-16T13:00:00Z", "updated_at": "2026-11-16T13:05:00Z"}
]}`

func TestParseRuns(t *testing.T) {
	runs, drafts, err := parseRuns([]byte(runsAnswer))
	if err != nil {
		t.Fatal(err)
	}
	if len(drafts) != 1 || drafts[0].id != 2 {
		t.Errorf("drafts %+v, want the skipped run alone", drafts)
	}
	type read struct {
		id          int64
		pullRequest bool
		failed      bool
		wall        time.Duration
	}
	want := []read{
		// The wait between the first attempt and the second is not the run's time.
		{1, true, true, 17*time.Minute + 5*time.Second},
		{4, false, false, 21*time.Minute + 19*time.Second},
		{5, true, true, 30 * time.Minute},
		{6, false, true, 5 * time.Second},
	}
	if len(runs) != len(want) {
		t.Fatalf("runs %+v, want %d of them", runs, len(want))
	}
	for i, w := range want {
		if got := (read{runs[i].id, runs[i].pullRequest, runs[i].failed, runs[i].wall}); got != w {
			t.Errorf("run %d: %+v, want %+v", i, got, w)
		}
	}
}

func TestRunMeasures(t *testing.T) {
	runs := []run{
		{pullRequest: true, wall: 10 * time.Minute},
		{pullRequest: true, wall: 30 * time.Minute, failed: true},
		{pullRequest: true, wall: 50 * time.Minute},
		{attempt: 1}, {attempt: 2}, {attempt: 3}, {attempt: 1},
	}
	for name, c := range map[string]struct {
		got  value
		n    float64
		text string
	}{
		"median":          {prMedian(runs), 30, "30.0 (n = 3 runs)"},
		"failed":          {prFailed(runs), 100.0 / 3, "33 (n = 3 runs)"},
		"second attempts": {secondAttempts(runs), 50, "50 (n = 4 runs)"},
	} {
		if !c.got.numeric || c.got.n != c.n || c.got.String() != c.text {
			t.Errorf("%s: %v %q, want %v %q", name, c.got.n, c.got, c.n, c.text)
		}
	}
	for name, v := range map[string]value{"median": prMedian(nil), "failed": prFailed(nil), "second attempts": secondAttempts(nil)} {
		if v.why == "" {
			t.Errorf("%s of no runs reads %q", name, v)
		}
	}
}

func TestRedHours(t *testing.T) {
	runs := []run{
		{created: at("2026-11-13 20:00"), updated: at("2026-11-13 22:00"), failed: true}, // red before the window starts
		{created: at("2026-11-14 01:00"), updated: at("2026-11-14 03:00")},               // green three hours into it
		{created: at("2026-11-15 00:00"), updated: at("2026-11-15 01:00"), failed: true},
		{created: at("2026-11-15 00:30"), updated: at("2026-11-15 01:30"), failed: true}, // still the same red
		{created: at("2026-11-15 02:00"), updated: at("2026-11-15 02:30")},
		{pullRequest: true, created: at("2026-11-16 00:00"), updated: at("2026-11-16 09:00"), failed: true},
		{created: at("2026-11-20 20:00"), updated: at("2026-11-20 23:00"), failed: true}, // red to the window's end
	}
	if got := redHours(runs, week, at("2026-11-25 00:00")); got.n != 3+1.5+1 || got.String() != "5.5 (n = 5 `main` runs)" {
		t.Errorf("red for %v hours, printed %q; want 5.5 beside the window's five runs on main", got.n, got)
	}
	// A window that has not ended is red until now, not until its end.
	if got := redHours(runs, week, at("2026-11-20 23:30")); got.n != 3+1.5+0.5 {
		t.Errorf("red for %.2f hours, want 5", got.n)
	}
	// The days after a red run on which nothing ran are red.
	idle := []run{{created: at("2026-11-14 10:00"), updated: at("2026-11-14 12:00"), failed: true}}
	if got := redHours(idle, week, at("2026-11-25 00:00")); got.n != 12+6*24 || got.String() != "156.0 (n = 1 `main` runs)" {
		t.Errorf("red for %v hours, printed %q; want 156 beside the one run", got.n, got)
	}
}

func TestJobStats(t *testing.T) {
	stats := jobStats([]job{
		{name: "unit", minutes: 4}, {name: "unit", minutes: 6, failed: true}, {name: "unit", minutes: 8},
		{name: "node", minutes: 12},
	})
	if len(stats) != 2 || stats[0] != (jobStat{name: "node", runs: 1, median: 12}) || stats[1] != (jobStat{name: "unit", runs: 3, failed: 1, median: 6}) {
		t.Errorf("stats %+v", stats)
	}
}

func TestPackagesOf(t *testing.T) {
	got := packagesOf([]string{
		"core/application/items/create.go",
		"apps/webapp/src/lib/board/Column.svelte",
		"apps/webapp/src/lib/board/cards/Card.svelte", // deeper than an area: still the area
		"apps/webapp/src/lib/motion.ts",
		"apps/webapp/e2e/board.test.mjs",
		"apps/webapp/package.json",
		"packages/sync-engine/src/storage/idb.ts",
		"packages/sdk/go/client.go",
		"docs/architecture/ci-cd.md",
		"Makefile",
	})
	want := []string{
		"core/application/items", "apps/webapp/src/lib/board", "apps/webapp/src/lib", "apps/webapp/e2e", "apps/webapp",
		"packages/sync-engine/src/storage", "packages/sdk/go",
	}
	if len(got) != len(want) {
		t.Errorf("packages %v, want %v", got, want)
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("packages %v lack %s", got, name)
		}
	}
}

func TestFixAfterFeat(t *testing.T) {
	feat := pullRequest{kind: "feat", merged: at("2026-11-10 12:00"), files: []string{"core/application/items/create.go", "apps/webapp/src/lib/board/Column.svelte"}}
	prs := []pullRequest{
		feat,
		{kind: "fix", merged: at("2026-11-15 12:00"), files: []string{"core/application/items/create_test.go"}},      // the same Go package
		{kind: "fix", merged: at("2026-11-16 12:00"), files: []string{"apps/webapp/src/lib/board/drag.ts"}},          // the same area of the client
		{kind: "fix", merged: at("2026-11-16 12:30"), files: []string{"apps/webapp/src/lib/search/kinds.ts"}},        // the same client, another area
		{kind: "fix", merged: at("2026-11-16 13:00"), files: []string{"core/application/sync/push.go", "docs/a.md"}}, // another package
		{kind: "fix", merged: at("2026-11-18 12:00"), files: []string{"core/application/items/create.go"}},           // more than seven days later
		{kind: "other", merged: at("2026-11-15 12:00"), files: []string{"core/application/items/create.go"}},
	}
	if got := fixAfterFeat(prs, week); got != 2 {
		t.Errorf("%d fixes after a feat, want 2", got)
	}
	if got := fixAfterFeat(prs, week.before()); got != 0 {
		t.Errorf("%d fixes after a feat in the window before, want 0", got)
	}
}

func TestReasonFor(t *testing.T) {
	body := "## Impact\n\n- `TestGone` asserted the old retry, which this change removes on purpose | see UC-SYN-03\n"
	if got, want := reasonFor(body, losttests.Test{Name: "TestGone", Identifier: true}), "- `TestGone` asserted the old retry, which this change removes on purpose \\| see UC-SYN-03"; got != want {
		t.Errorf("reason %q, want %q", got, want)
	}
	if got := reasonFor(body, losttests.Test{Name: "TestOther", Identifier: true}); got != "the description does not name it" {
		t.Errorf("reason %q for a test the description does not name", got)
	}
}

func TestRefusal(t *testing.T) {
	err := errString("gh api: exit status 1: gh: Resource not accessible by integration (HTTP 403)")
	if got := refusal(err); got != "the API refused this token (HTTP 403)" {
		t.Errorf("refusal %q", got)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
