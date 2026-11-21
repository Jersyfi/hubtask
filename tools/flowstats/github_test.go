// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"testing"
	"time"
)

func at(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestRunMeasures(t *testing.T) {
	runs := []run{
		{pullRequest: true, wall: 10 * time.Minute},
		{pullRequest: true, wall: 30 * time.Minute, failed: true},
		{pullRequest: true, wall: 50 * time.Minute},
		{pullRequest: true, wall: time.Minute}, // a draft's run checked nothing
		{attempt: 1}, {attempt: 2}, {attempt: 3}, {attempt: 1},
	}
	if got := prMedian(runs).String(); got != "30.0" {
		t.Errorf("median %q, want 30.0", got)
	}
	if got := prFailed(runs).String(); got != "33" {
		t.Errorf("failed %q, want 33", got)
	}
	if got := secondAttempts(runs).String(); got != "50" {
		t.Errorf("second attempts %q, want 50", got)
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
	if got := redHours(runs, week, at("2026-11-25 00:00")); got != 3+1.5+1 {
		t.Errorf("red for %.2f hours, want 5.5", got)
	}
	// A window that has not ended is red until now, not until its end.
	if got := redHours(runs, week, at("2026-11-20 23:30")); got != 3+1.5+0.5 {
		t.Errorf("red for %.2f hours, want 5", got)
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

func TestFixAfterFeat(t *testing.T) {
	feat := pullRequest{kind: "feat", merged: at("2026-11-10 12:00"), files: []string{"core/application/items/create.go", "apps/webapp/src/lib/a.ts"}}
	prs := []pullRequest{
		feat,
		{kind: "fix", merged: at("2026-11-15 12:00"), files: []string{"core/application/items/create_test.go"}},      // the same Go package
		{kind: "fix", merged: at("2026-11-16 12:00"), files: []string{"apps/webapp/e2e/board.test.mjs"}},             // the same workspace package
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

func TestLostIn(t *testing.T) {
	diff := `diff --git a/core/a/a_test.go b/core/a/a_test.go
--- a/core/a/a_test.go
+++ b/core/a/a_test.go
@@ -10,3 +9,0 @@
-func TestGone(t *testing.T) {
-	run(t)
-}
@@ -20 +17 @@
-func TestOldName(t *testing.T) {
+func TestNewName(t *testing.T) {
@@ -30,2 +26,0 @@
-func TestMoved(t *testing.T) {
-}
@@ -40,0 +37 @@
+	t.Skip("slow on this runner")
@@ -50,0 +48 @@
+	// t.Skip is not what this does
diff --git a/core/b/b_test.go b/core/b/b_test.go
--- a/core/b/b_test.go
+++ b/core/b/b_test.go
@@ -5,0 +6,2 @@
+func TestMoved(t *testing.T) {
+}
diff --git a/apps/webapp/src/lib/a.test.ts b/apps/webapp/src/lib/a.test.ts
--- a/apps/webapp/src/lib/a.test.ts
+++ /dev/null
@@ -1,2 +0,0 @@
-test('a name matches when it holds every word', () => {
-});
`
	got := lostIn(diff)
	want := []lostTest{
		{path: "core/a/a_test.go", name: "TestGone", how: "removed"},
		{path: "core/a/a_test.go", name: "a_test.go", how: "a skip added"},
		{path: "apps/webapp/src/lib/a.test.ts", name: "a name matches when it holds every word", how: "removed"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestReasonFor(t *testing.T) {
	body := "## Impact\n\n- `TestGone` asserted the old retry, which this change removes on purpose | see UC-SYN-03\n"
	if got, want := reasonFor(body, lostTest{name: "TestGone"}), "- `TestGone` asserted the old retry, which this change removes on purpose \\| see UC-SYN-03"; got != want {
		t.Errorf("reason %q, want %q", got, want)
	}
	if got := reasonFor(body, lostTest{name: "TestOther"}); got != "the description does not name it" {
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
