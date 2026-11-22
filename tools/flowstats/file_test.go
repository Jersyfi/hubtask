// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// recorder is a tracker that writes down what filing asked of it.
type recorder struct {
	calls []string
	next  int
}

func (r *recorder) create(title, _ string, labels ...string) (int, error) {
	r.next++
	r.calls = append(r.calls, fmt.Sprintf("create %q %v -> #%d", title, labels, r.next))
	return r.next, nil
}

func (r *recorder) comment(number int, body string) error {
	first, _, _ := strings.Cut(body, "\n")
	r.calls = append(r.calls, fmt.Sprintf("comment #%d %q", number, first))
	return nil
}

func (r *recorder) setState(number int, state string) error {
	r.calls = append(r.calls, fmt.Sprintf("#%d %s", number, state))
	return nil
}

const bot = "github-actions[bot]"

// The search's answer, reduced to the fields read: the job's thread, its finding for the failed
// runs, and a stranger's issues under the same titles.
func searchAnswer(findingState string) []byte {
	return []byte(`{"total_count": 4, "items": [
 {"number": 90, "title": "Flow report", "state": "open", "user": {"login": "stranger"}, "labels": []},
 {"number": 100, "title": "Flow report", "state": "open", "user": {"login": "github-actions[bot]"}, "labels": []},
 {"number": 95, "title": "Flow report: failed is over its threshold", "state": "open", "user": {"login": "stranger"}, "labels": [{"name": "finding"}]},
 {"number": 101, "title": "Flow report: failed is over its threshold", "state": "` + findingState + `", "user": {"login": "github-actions[bot]"}, "labels": [{"name": "finding"}]},
 {"number": 102, "title": "Flow report: failed is over its threshold", "state": "open", "user": {"login": "github-actions[bot]"}, "labels": []}
]}`)
}

var (
	failed = finding{Measure: "failed", Title: "Flow report: failed is over its threshold", Body: "The weekly flow report measured a value over its threshold."}
	median = finding{Measure: "median", Title: "Flow report: median is over its threshold", Body: "The weekly flow report measured a value over its threshold."}
)

func TestFile(t *testing.T) {
	cases := []struct {
		name     string
		issues   []byte
		breached []finding
		within   []standing
		want     []string
	}{
		{"the first run opens the thread and one finding per breach", []byte(`{"items": []}`), []finding{failed, median}, nil, []string{
			`create "Flow report" [] -> #1`,
			`create "Flow report: failed is over its threshold" [finding] -> #2`,
			`create "Flow report: median is over its threshold" [finding] -> #3`,
			`comment #1 "## Flow"`,
		}},
		{"a breach whose finding is open adds nothing but the week's comment", searchAnswer("open"), []finding{failed}, nil, []string{
			`comment #100 "## Flow"`,
		}},
		{"a breach whose finding is closed opens it again with one comment", searchAnswer("closed"), []finding{failed}, nil, []string{
			`comment #101 "The weekly flow report measured a value over its threshold."`,
			`#101 open`,
			`comment #100 "## Flow"`,
		}},
		{"a measure back within its threshold closes its open finding with one comment", searchAnswer("open"), nil,
			[]standing{{Measure: "failed", Title: failed.Title, Value: "12 (n = 34 runs)"}, {Measure: "median", Title: median.Title, Value: "18.0"}}, []string{
				`comment #101 "Within the threshold again: 12 (n = 34 runs)"`,
				`#101 closed`,
				`comment #100 "## Flow"`,
			}},
		{"a measure within its threshold leaves a closed finding closed", searchAnswer("closed"), nil,
			[]standing{{Measure: "failed", Title: failed.Title, Value: "12"}}, []string{
				`comment #100 "## Flow"`,
			}},
		{"a stranger's issues of the same titles are not the job's", []byte(`{"items": [
 {"number": 90, "title": "Flow report", "state": "open", "user": {"login": "stranger"}, "labels": []},
 {"number": 95, "title": "Flow report: failed is over its threshold", "state": "open", "user": {"login": "stranger"}, "labels": [{"name": "finding"}]}
]}`), []finding{failed}, nil, []string{
			`create "Flow report" [] -> #1`,
			`create "Flow report: failed is over its threshold" [finding] -> #2`,
			`comment #1 "## Flow"`,
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			issues, err := parseIssues(c.issues)
			if err != nil {
				t.Fatal(err)
			}
			r := &recorder{}
			if err := file(r, issues, bot, "## Flow\n\nthe week\n", c.breached, c.within, nil); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(r.calls, c.want) {
				t.Errorf("filing did\n  %s\nwant\n  %s", strings.Join(r.calls, "\n  "), strings.Join(c.want, "\n  "))
			}
		})
	}
}

// A report over the length a comment takes fails the job, after the findings are filed.
func TestAReportTooLongFailsAfterTheFindings(t *testing.T) {
	r := &recorder{}
	err := file(r, nil, bot, strings.Repeat("ä", commentLimit+1), []finding{failed}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "the findings are filed, the report is not posted") {
		t.Fatalf("a report too long ends with %v", err)
	}
	want := []string{`create "Flow report" [] -> #1`, `create "Flow report: failed is over its threshold" [finding] -> #2`}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("filing did %v, want %v", r.calls, want)
	}
	// Characters are counted, not bytes: at the limit it is posted.
	r = &recorder{}
	if err := file(r, nil, bot, strings.Repeat("ä", commentLimit), nil, nil, nil); err != nil {
		t.Errorf("a report of exactly the limit ends with %v", err)
	}
	if want := []string{`create "Flow report" [] -> #1`, fmt.Sprintf("comment #1 %q", strings.Repeat("ä", commentLimit))}; !reflect.DeepEqual(r.calls, want) {
		t.Errorf("a report of exactly the limit is not posted on the thread: %d calls, the first %.60q", len(r.calls), strings.Join(r.calls, " "))
	}
}

func TestWithin(t *testing.T) {
	limits, err := parseThresholds([]byte(`{"pr_run_median_minutes": {"max": 20}, "pr_runs_failed_percent": {"max": 15}, "fixed_waits": {"rule": "no_rise"}, "main_red_hours": {"max": 2}}`))
	if err != nil {
		t.Fatal(err)
	}
	measures := inOrder(map[string]measure{
		"pr_run_median_minutes":  {now: number(18, "%.1f")},
		"pr_runs_failed_percent": {now: number(40, "%.0f")},
		"fixed_waits":            {now: count(3), before: unmeasured("the commit has no bounds")},
		"main_red_hours":         {now: unmeasured(noRuns)},
		"open_findings":          {now: count(4)},
	}, true)
	got := within(measures, limits)
	want := []standing{{Measure: "pr_run_median_minutes", Title: "Flow report: Pull request run, last attempt, median wall minutes is over its threshold", Value: "18.0"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("within: %+v, want %+v", got, want)
	}
}
