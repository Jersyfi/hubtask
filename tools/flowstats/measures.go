// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// value is what a measure reads in one window: a number, a text, or the reason it has neither.
type value struct {
	n       float64
	numeric bool
	text    string
	why     string // not measured, and why
}

func number(n float64, format string) value {
	return value{n: n, numeric: true, text: fmt.Sprintf(format, n)}
}

func count(n int) value { return number(float64(n), "%.0f") }

func words(text string) value { return value{text: text} }

func unmeasured(format string, a ...any) value { return value{why: fmt.Sprintf(format, a...)} }

func (v value) String() string {
	switch {
	case v.why != "":
		return "not measured: " + v.why
	case v.text == "":
		return "—"
	}
	return v.text
}

// measure is one row of the report: this window, the one before it, and the one before that for
// the one rule that reads three.
type measure struct {
	id, name             string
	now, before, earlier value
}

// rows are the measures in the order the report prints them (ci-cd.md §11). github says the row
// is read from GitHub, so a run without it says so instead of leaving the row out.
var rows = []struct {
	id, name string
	github   bool
}{
	{"pr_run_median_minutes", "Pull request run, last attempt, median wall minutes", true},
	{"pr_runs_failed_percent", "Pull request runs failed, percent", true},
	{"main_second_attempt_percent", "`main` runs needing a second attempt, percent", true},
	{"main_red_hours", "Hours `main` was red", true},
	{"fix_after_feat", "`fix` pull requests merged within seven days of a `feat` touching the same package", true},
	{"open_findings", "Open `finding` issues, and the oldest", true},
	{"open_decisions", "Open `decision` issues, and the oldest", true},
	{"uncited_checks_built", "Uncited checks of built use cases, as the documentation gate bounds them", false},
	{"fixed_waits", "Fixed waits in the walks (`waitForTimeout`)", false},
	{"timer_waits", "Timer waits in the walks (`setTimeout`)", false},
	{"suites_without_use_case", "Suites without a use case", false},
	{"answers_outside_contract", "Stub answers outside the contract", false},
	{"use_cases_by_state", "Use cases by state", false},
	{"docs_lines", "Lines added / removed under `docs/` outside `archive/`", false},
	{"agents_lines", "Lines of the root `" + agentsPath + "`", false},
	{"comment_blocks", "Comment blocks of twenty lines or more", false},
	{"lost_tests", "Tests deleted or skipped in merged pull requests", true},
	{"unreachable_go_functions", "Unreachable Go functions", false},
	{"unused_workspace_code", "Unused files, exports and types of the TypeScript workspace", false},
	{"unused_translation_keys", "Unused translation keys", false},
	{"outside_reviewer_comments", "Outside reviewer: comments made", true},
	{"outside_reviewer_share", "Outside reviewer: share of comments that led to a change", true},
	{"alerts_code_scanning", "Open code scanning alerts, and the oldest", true},
	{"alerts_secret_scanning", "Open secret scanning alerts, and the oldest", true},
	{"alerts_dependabot", "Open Dependabot alerts, and the oldest", true},
	{badgeMeasure, "OpenSSF Best Practices badge", false},
}

// inOrder is every row, with what was found for it. A row nothing filled is not measured.
func inOrder(found map[string]measure, github bool) []measure {
	out := make([]measure, 0, len(rows))
	for _, row := range rows {
		m, ok := found[row.id]
		if !ok {
			why := "no source in this run"
			if row.github && !github {
				why = "run with -github=false"
			}
			m = measure{now: unmeasured("%s", why), before: unmeasured("%s", why)}
		}
		m.id, m.name = row.id, row.name
		out = append(out, m)
	}
	return out
}

// ----------------------------------------------------------------------------- thresholds

//go:embed thresholds.json
var thresholdsFile []byte

// threshold is what the owner set for one measure: a ceiling, a rule against rising, or the
// level the badge holds at least. Project is the badge's project on the badge site, 0 until the
// project is registered there.
type threshold struct {
	Max     *float64 `json:"max,omitempty"`
	Rule    string   `json:"rule,omitempty"`
	Level   string   `json:"level,omitempty"`
	Project int      `json:"project,omitempty"`
}

const (
	noRise      = "no_rise"       // the value is not above the window before
	noRiseTwice = "no_rise_twice" // the value did not rise in this window and in the one before
)

func (t threshold) String() string {
	switch {
	case t.Max != nil:
		return "≤ " + strconv.FormatFloat(*t.Max, 'f', -1, 64)
	case t.Rule == noRise:
		return "does not rise"
	case t.Rule == noRiseTwice:
		return "does not rise two windows running"
	case t.Level != "":
		return "at least " + t.Level
	}
	return "reported"
}

func readThresholds() (map[string]threshold, error) { return parseThresholds(thresholdsFile) }

func parseThresholds(data []byte) (map[string]threshold, error) {
	limits := map[string]threshold{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&limits); err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, row := range rows {
		known[row.id] = true
	}
	for id, t := range limits {
		switch {
		case !known[id]:
			return nil, fmt.Errorf("%q is no measure of the report", id)
		case kinds(t) != 1:
			return nil, fmt.Errorf("%q needs a max, a rule or a level, and only one", id)
		case t.Rule != "" && t.Rule != noRise && t.Rule != noRiseTwice:
			return nil, fmt.Errorf("%q has the unknown rule %q", id, t.Rule)
		case (t.Level != "" || t.Project != 0) && id != badgeMeasure:
			return nil, fmt.Errorf("%q is not the badge: it takes no level and no project", id)
		case t.Project < 0:
			return nil, fmt.Errorf("%q has a project below zero", id)
		}
		if _, ok := badgeRank(t.Level); t.Level != "" && !ok {
			return nil, fmt.Errorf("%q has the unknown level %q", id, t.Level)
		}
	}
	return limits, nil
}

// kinds counts the ways a threshold says what it holds.
func kinds(t threshold) int {
	n := 0
	for _, set := range []bool{t.Max != nil, t.Rule != "", t.Level != ""} {
		if set {
			n++
		}
	}
	return n
}

// finding is the text of the issue a breached threshold opens. The title carries no week: it is
// how the job finds the issue of a measure again.
type finding struct {
	Measure, Title, Body string
}

// held says how a measure stands to its threshold; over is the breach, which for the badge is a
// level below the one asked for. known is false where the measure has no
// threshold, was not measured, or its rule has no earlier value to hold it against: such a
// measure neither breaches nor comes back within anything.
func held(m measure, limits map[string]threshold) (t threshold, over, known bool) {
	t, ok := limits[m.id]
	if !ok || !m.now.numeric {
		return t, false, false
	}
	switch {
	case t.Max != nil:
		return t, m.now.n > *t.Max, true
	case t.Rule == noRise:
		return t, m.before.numeric && m.now.n > m.before.n, m.before.numeric
	case t.Rule == noRiseTwice:
		known = m.before.numeric && m.earlier.numeric
		return t, known && m.now.n > m.before.n && m.before.n > m.earlier.n, known
	case t.Level != "":
		rank, _ := badgeRank(t.Level)
		return t, m.now.n < float64(rank), true
	}
	return t, false, false
}

func findingTitle(m measure, t threshold) string {
	return "Flow report: " + strings.ReplaceAll(m.name, "`", "") + " is " + side(t) + " its threshold"
}

// side is where a breach stands to its threshold: over a ceiling, below the badge's level.
func side(t threshold) string {
	if t.Level != "" {
		return "below"
	}
	return "over"
}

// breaches are the thresholds the measures are over. A measure that was not measured breaches
// nothing: the report says so in its row.
func breaches(measures []measure, limits map[string]threshold, w window) []finding {
	var out []finding
	for _, m := range measures {
		t, over, _ := held(m, limits)
		if !over {
			continue
		}
		out = append(out, finding{
			Measure: m.id,
			Title:   findingTitle(m, t),
			Body: strings.Join([]string{
				"The weekly flow report measured a value " + side(t) + " its threshold.",
				"",
				"* Measure: " + m.name + " (`" + m.id + "`)",
				"* Value: " + m.now.String(),
				"* Threshold: " + t.String(),
				"* The window before: " + m.before.String(),
				"* Window: " + w.String() + ", beside " + w.before().String(),
				"",
				"The thresholds are the owner's to set (`docs/architecture/ci-cd.md` §11). This issue starts no work by itself.",
			}, "\n"),
		})
	}
	return out
}

// standing is a measure that is within its threshold, under the title its finding would carry.
type standing struct {
	Measure, Title, Value string
}

// within are the measures that were measured and hold their threshold: an open finding of one
// is closed.
func within(measures []measure, limits map[string]threshold) []standing {
	var out []standing
	for _, m := range measures {
		if t, over, known := held(m, limits); known && !over {
			out = append(out, standing{Measure: m.id, Title: findingTitle(m, t), Value: m.now.String()})
		}
	}
	return out
}

// ----------------------------------------------------------------------------- printing

func printMeasures(b *bytes.Buffer, measures []measure, limits map[string]threshold) {
	fmt.Fprintf(b, "### The measures\n\n")
	fmt.Fprintf(b, "What is read from the tree is read at the checkout's commit, and beside it at the commit the window started with.\n\n")
	fmt.Fprintf(b, "| measure | this window | the window before | threshold |\n|---|---|---|---|\n")
	for _, m := range measures {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", m.name, m.now, m.before, limits[m.id])
	}
	fmt.Fprintln(b)
}

func printBreaches(b *bytes.Buffer, found []finding) {
	fmt.Fprintf(b, "### Breaches\n\n")
	if len(found) == 0 {
		fmt.Fprintf(b, "No measure breaches its threshold.\n\n")
		return
	}
	for _, f := range found {
		fmt.Fprintf(b, "**%s**\n\n%s\n\n", f.Title, f.Body)
	}
}
