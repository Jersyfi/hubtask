// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
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
	idle := []run{
		{created: at("2026-11-10 10:00"), updated: at("2026-11-10 10:30")},
		{created: at("2026-11-14 10:00"), updated: at("2026-11-14 12:00"), failed: true},
	}
	if got := redHours(idle, week, at("2026-11-25 00:00")); got.n != 12+6*24 || got.String() != "156.0 (n = 1 `main` runs)" {
		t.Errorf("red for %v hours, printed %q; want 156 beside the one run", got.n, got)
	}
}

// Main is what its newest commit's run left, so the runs are taken in the order they began.
func TestRedHoursFollowTheCommitsWhereRunsOverlap(t *testing.T) {
	green := run{created: at("2026-11-10 10:00"), updated: at("2026-11-10 10:30")}
	// The newer commit fails fast while the older one's run still goes, and passes after it.
	newerRed := []run{green,
		{created: at("2026-11-15 09:00"), updated: at("2026-11-15 09:20")},
		{created: at("2026-11-15 09:05"), updated: at("2026-11-15 09:10"), failed: true},
		{created: at("2026-11-15 12:00"), updated: at("2026-11-15 12:10")},
	}
	if got := redHours(newerRed, week, at("2026-11-25 00:00")); got.n != 3 {
		t.Errorf("red for %v hours, want the 3 from the newer commit's failure to the next green", got.n)
	}
	// The older commit's run fails after the newer commit's has passed: main stays green.
	olderRed := []run{green,
		{created: at("2026-11-15 09:00"), updated: at("2026-11-15 09:40"), failed: true},
		{created: at("2026-11-15 09:05"), updated: at("2026-11-15 09:25")},
	}
	if got := redHours(olderRed, week, at("2026-11-25 00:00")); got.n != 0 {
		t.Errorf("red for %v hours, want none", got.n)
	}
}

// What main was when the window began is the last run before it. Where the runs read hold none,
// it is not known - and not green: a zero would close an open finding.
func TestRedHoursWithoutARunBeforeTheWindow(t *testing.T) {
	for name, runs := range map[string][]run{
		"no run at all":             nil,
		"runs inside the window":    {{created: at("2026-11-15 10:00"), updated: at("2026-11-15 10:30")}},
		"a run that ended inside":   {{created: at("2026-11-13 23:50"), updated: at("2026-11-14 00:20")}},
		"a pull request's run only": {{pullRequest: true, created: at("2026-11-10 10:00"), updated: at("2026-11-10 10:30")}},
	} {
		if got := redHours(runs, week, at("2026-11-25 00:00")); got.numeric || got.why == "" {
			t.Errorf("%s: red hours read %q, want not measured", name, got)
		}
	}
}

// The second attempt of a run that ran its two failed jobs again: the answer lists the job it
// did not run again with the first attempt's times.
const (
	firstAttempt = `{"total_count": 4, "jobs": [
 {"name": "Unit and domain tests", "status": "completed", "conclusion": "success", "started_at": "2026-11-16T10:00:20Z", "completed_at": "2026-11-16T10:04:20Z"},
 {"name": "The client runs in Chromium (1/3)", "status": "completed", "conclusion": "failure", "started_at": "2026-11-16T10:00:30Z", "completed_at": "2026-11-16T10:12:30Z"},
 {"name": "Workspace (${{ matrix.package }})", "status": "completed", "conclusion": "skipped", "started_at": "2026-11-16T10:00:30Z", "completed_at": "2026-11-16T10:00:30Z"},
 {"name": "CI required", "status": "completed", "conclusion": "failure", "started_at": "2026-11-16T10:13:00Z", "completed_at": "2026-11-16T10:13:10Z"}
]}`
	secondAttempt = `{"total_count": 3, "jobs": [
 {"name": "Unit and domain tests", "status": "completed", "conclusion": "success", "started_at": "2026-11-16T10:00:20Z", "completed_at": "2026-11-16T10:04:20Z"},
 {"name": "The client runs in Chromium (1/3)", "status": "completed", "conclusion": "timed_out", "started_at": "2026-11-16T12:42:30Z", "completed_at": "2026-11-16T13:12:30Z"},
 {"name": "CI required", "status": "completed", "conclusion": "failure", "started_at": "2026-11-16T13:13:00Z", "completed_at": "2026-11-16T13:13:10Z"}
]}`
)

func TestParseJobs(t *testing.T) {
	summary := map[string]bool{"CI required": true}
	seen := map[string]bool{}
	var jobs []job
	for _, answer := range []string{firstAttempt, secondAttempt} {
		read, err := parseJobs([]byte(answer), summary, seen)
		if err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, read...)
	}
	want := []job{
		{name: "Unit and domain tests", minutes: 4},
		{name: "The client runs in Chromium (1/3)", failed: true, minutes: 12},
		{name: "The client runs in Chromium (1/3)", failed: true, minutes: 30},
	}
	if len(jobs) != len(want) {
		t.Fatalf("jobs %+v, want %+v", jobs, want)
	}
	for i := range want {
		if jobs[i] != want[i] {
			t.Errorf("%d: %+v, want %+v", i, jobs[i], want[i])
		}
	}
}

// The reading of jobs has a time of its own: runs it does not reach are counted as not read, and
// the report is still printed and filed.
func TestTheReadingOfJobsStopsWhenItsTimeIsUp(t *testing.T) {
	start := at("2026-11-21 05:00")
	clock := start
	asked := 0
	fetch := func(int64, int) ([]byte, error) {
		asked++
		clock = clock.Add(4 * time.Minute) // each answer is slow
		return []byte(firstAttempt), nil
	}
	runs := []run{{id: 1, attempt: 1}, {id: 2, attempt: 2}, {id: 3, attempt: 1}, {id: 4, attempt: 1}}
	jobs, unread, why := readJobs(runs, map[string]bool{"CI required": true}, start.Add(jobsBudget), func() time.Time { return clock }, fetch)
	// Run 1, then the first attempt of run 2, then its second: twelve minutes, over the ten.
	if asked != 3 || len(jobs) != 4 || unread != 2 {
		t.Errorf("%d requests, %d jobs, %d runs not read; want 3 requests, 4 jobs and the last 2 runs not read", asked, len(jobs), unread)
	}
	if !strings.Contains(why, "ran out") {
		t.Errorf("the reason reads %q", why)
	}
	// A refusal stays the reason where it came first.
	clock = start
	_, unread, why = readJobs(runs, nil, start.Add(jobsBudget), func() time.Time { return clock }, func(id int64, _ int) ([]byte, error) {
		clock = clock.Add(6 * time.Minute)
		return nil, errString("gh api: exit status 1: gh: Server Error (HTTP 502)")
	})
	if unread != 4 || why != "the API did not answer (HTTP 502)" {
		t.Errorf("%d runs not read, because %q; want all 4 and the API's refusal", unread, why)
	}
}

// The names are read from ci.yml itself, so a renamed summary job does not come back into the
// tables unseen.
func TestSummaryJobsAreReadFromTheWorkflow(t *testing.T) {
	data, err := os.ReadFile("../../" + ciWorkflow)
	if err != nil {
		t.Fatal(err)
	}
	got := summaryJobsIn(data)
	for _, name := range []string{"CI required", "CI by hand", "CI not run (draft)"} {
		if !got[name] {
			t.Errorf("the summary jobs %v lack %q", got, name)
		}
	}
	if len(got) != 3 {
		t.Errorf("the summary jobs are %v, want three names", got)
	}
	if len(summaryJobsIn([]byte("jobs:\n  unit:\n    name: Unit\n"))) != 0 {
		t.Error("a workflow without the job names one")
	}
}

func TestJobStats(t *testing.T) {
	stats := jobStats([]job{
		{name: "unit", minutes: 4}, {name: "unit", minutes: 6, failed: true}, {name: "unit", minutes: 8},
		{name: "node", minutes: 12},
	})
	if len(stats) != 2 || stats[0] != (jobStat{name: "node", attempts: 1, median: 12}) || stats[1] != (jobStat{name: "unit", attempts: 3, failed: 1, median: 6}) {
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

// A count that lacks pull requests is a lower bound: it is printed, and it neither breaches the
// rule nor comes back within it.
func TestAPartialFixAfterFeatCountHoldsNoThreshold(t *testing.T) {
	limits, err := parseThresholds([]byte(`{"fix_after_feat": {"rule": "no_rise_twice"}}`))
	if err != nil {
		t.Fatal(err)
	}
	whole := measure{id: "fix_after_feat", now: fixCount(2, 0), before: fixCount(2, 0), earlier: fixCount(1, 0)}
	if _, over, known := held(whole, limits); over || !known {
		t.Errorf("a whole count: over %v, known %v; want within its threshold", over, known)
	}
	partial := measure{id: "fix_after_feat", now: fixCount(2, 1), before: fixCount(2, 1), earlier: fixCount(1, 1)}
	if _, over, known := held(partial, limits); over || known {
		t.Errorf("a partial count: over %v, known %v; want neither", over, known)
	}
	if got, want := partial.now.String(), "2 (not read: 1 pull requests whose merge commit is not in this clone)"; got != want {
		t.Errorf("a partial count prints %q, want %q", got, want)
	}
}

func TestReasonFor(t *testing.T) {
	body := "## Impact\n\n- `TestGone` asserted the old retry, which this change removes on purpose\n"
	if got, want := reasonFor(body, losttests.Test{Name: "TestGone", Identifier: true}), "- `TestGone` asserted the old retry, which this change removes on purpose"; got != want {
		t.Errorf("reason %q, want %q", got, want)
	}
	if got := reasonFor(body, losttests.Test{Name: "TestOther", Identifier: true}); got != "" {
		t.Errorf("reason %q for a test the description does not name", got)
	}
}

// What a stranger wrote reaches the weekly comment as text and nothing else.
func TestInert(t *testing.T) {
	for text, want := range map[string]string{
		"`TestGone` is gone, ask @owner | see [here](https://example.com) ![x](https://example.com/x.png)": "`'TestGone' is gone, ask @owner \\| see [here](https://example.com) ![x](https://example.com/x.png)`",
		"<details><summary>the rest of the report":                                                         "`<details><summary>the rest of the report`",
		"a  line\twith\rspaces":  "`a line with spaces`",
		"":                       "—",
		strings.Repeat("ä", 200): "`" + strings.Repeat("ä", 120) + "…`",
	} {
		if got := inert(text, reasonLength); got != want {
			t.Errorf("%q prints %q, want %q", text, got, want)
		}
	}
}

func TestTheLostTestsTableIsBounded(t *testing.T) {
	var lost []lostTest
	for i := range 45 {
		lost = append(lost, lostTest{Test: losttests.Test{Path: "core/a/a_test.go", Name: fmt.Sprintf("Test%d", i), How: "removed"}, pr: 7})
	}
	lost[0].reason = "- `Test0` asserted what this change removes"
	var b bytes.Buffer
	printLostTests(&b, lost, 0)
	out := b.String()
	if got := strings.Count(out, "| #7 |"); got != lostRows {
		t.Errorf("%d rows, want %d", got, lostRows)
	}
	for _, want := range []string{"And 15 more, not printed.", "| #7 | `core/a/a_test.go` | `Test0` | removed | `- 'Test0' asserted what this change removes` |", "| `Test1` | removed | the description does not name it |"} {
		if !strings.Contains(out, want) {
			t.Errorf("the table lacks %q:\n%s", want, out)
		}
	}
}

func TestRefusal(t *testing.T) {
	for answer, want := range map[string]string{
		"gh api: exit status 1: gh: Resource not accessible by integration (HTTP 403)": "the API refused the request (HTTP 403)",
		"gh api: exit status 1: gh: Not Found (HTTP 404)":                              "the feature is off or the token may not read it (HTTP 404)",
		"gh api: exit status 1: gh: Server Error (HTTP 502)":                           "the API did not answer (HTTP 502)",
		"gh api: context deadline exceeded":                                            "gh api: context deadline exceeded",
	} {
		if got := refusal(errString(answer)); got != want {
			t.Errorf("%q reads %q, want %q", answer, got, want)
		}
	}
}

func TestCommentsIn(t *testing.T) {
	stamps := "2026-11-13T23:59:59Z\n2026-11-14T00:00:00Z\n2026-11-20T23:59:59Z\n2026-11-21T00:00:00Z\n"
	if got := commentsIn(stamps, week); !got.numeric || got.n != 2 || got.String() != "2" {
		t.Errorf("comments %v %q, want the two inside the window", got.n, got)
	}
	if got := commentsIn("", week); !got.numeric || got.n != 0 || got.String() != "none in the window" {
		t.Errorf("no comment reads %q", got)
	}
	if got := reviewerComments("o/r", "", week); got.why == "" {
		t.Errorf("without a login the comments read %q", got)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
