// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

var week = window{from: day("2026-11-14"), to: day("2026-11-21")}

// A threshold set below the current value produces the text of a finding: its title, the
// measure, the value, the threshold and the window.
func TestAThresholdBelowTheValueProducesAFinding(t *testing.T) {
	limits, err := parseThresholds([]byte(`{"pr_run_median_minutes": {"max": 20}}`))
	if err != nil {
		t.Fatal(err)
	}
	measures := inOrder(map[string]measure{
		"pr_run_median_minutes": {now: number(20.5, "%.1f"), before: number(18, "%.1f")},
	}, true)
	found := breaches(measures, limits, week)
	if len(found) != 1 {
		t.Fatalf("want one finding, got %d: %+v", len(found), found)
	}
	f := found[0]
	if want := "Flow report: Pull request run, last attempt, median wall minutes is over its threshold"; f.Title != want {
		t.Errorf("title %q, want %q", f.Title, want)
	}
	for _, part := range []string{
		"* Measure: Pull request run, last attempt, median wall minutes (`pr_run_median_minutes`)",
		"* Value: 20.5",
		"* Threshold: ≤ 20",
		"* The window before: 18.0",
		"* Window: 2026-11-14 … 2026-11-20, beside 2026-11-07 … 2026-11-13",
	} {
		if !hasLine(f.Body, part) {
			t.Errorf("the finding's text lacks the line %q:\n%s", part, f.Body)
		}
	}
	var b bytes.Buffer
	printBreaches(&b, found)
	if !hasLine(b.String(), "**"+f.Title+"**") || !hasLine(b.String(), "* Value: 20.5") {
		t.Errorf("the report does not print the finding:\n%s", b.String())
	}

	// At the threshold is not over it.
	measures[0].now = number(20, "%.1f")
	if found := breaches(measures, limits, week); len(found) != 0 {
		t.Errorf("a value at its threshold breaches: %+v", found)
	}
}

// hasLine holds a whole line, so "* Value: 20.55" does not pass for "* Value: 20.5".
func hasLine(text, line string) bool {
	return slices.Contains(strings.Split(text, "\n"), line)
}

func TestBreachesByRule(t *testing.T) {
	limits, err := parseThresholds([]byte(`{"fixed_waits": {"rule": "no_rise"}, "fix_after_feat": {"rule": "no_rise_twice"}}`))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		m    measure
		want bool
	}{
		{"a bound that rose", measure{id: "fixed_waits", now: count(356), before: count(355)}, true},
		{"a bound that fell", measure{id: "fixed_waits", now: count(354), before: count(355)}, false},
		{"a bound with nothing before it", measure{id: "fixed_waits", now: count(355), before: unmeasured("absent")}, false},
		{"a bound that was not measured", measure{id: "fixed_waits", now: unmeasured("absent"), before: count(1)}, false},
		{"a rise two windows running", measure{id: "fix_after_feat", now: count(3), before: count(2), earlier: count(1)}, true},
		{"a rise in one window", measure{id: "fix_after_feat", now: count(3), before: count(2), earlier: count(2)}, false},
		{"a measure without a threshold", measure{id: "agents_lines", now: count(900), before: count(1)}, false},
	}
	for _, c := range cases {
		if got := len(breaches([]measure{c.m}, limits, week)) == 1; got != c.want {
			t.Errorf("%s: breach %v, want %v", c.name, got, c.want)
		}
	}
}

func TestThresholdsAreRefused(t *testing.T) {
	for name, data := range map[string]string{
		"an unknown measure": `{"coverage": {"max": 1}}`,
		"neither":            `{"fixed_waits": {}}`,
		"both":               `{"fixed_waits": {"max": 1, "rule": "no_rise"}}`,
		"an unknown rule":    `{"fixed_waits": {"rule": "falls"}}`,
		"an unknown field":   `{"fixed_waits": {"min": 1}}`,
		"no object":          `null`,
		"a second value":     `{"fixed_waits": {"rule": "no_rise"}} {"fixed_waits": {"max": 1}}`,
		"text after it":      `{"fixed_waits": {"rule": "no_rise"}} and more`,
	} {
		if _, err := parseThresholds([]byte(data)); err == nil {
			t.Errorf("%s is accepted: %s", name, data)
		}
	}
}

// A measure nothing read is printed as not measured, with the reason - never as a zero.
func TestAMeasureWithoutASourceSaysSo(t *testing.T) {
	var b bytes.Buffer
	printMeasures(&b, inOrder(map[string]measure{}, false), nil)
	rowsPrinted := strings.Count(b.String(), "\n| ") - 1
	if rowsPrinted != len(rows) {
		t.Fatalf("%d rows printed, want %d", rowsPrinted, len(rows))
	}
	for _, row := range strings.Split(b.String(), "\n")[6:] {
		if strings.HasPrefix(row, "| ") && strings.Count(row, "not measured: ") != 2 {
			t.Errorf("a row without a source does not say so twice: %s", row)
		}
	}
	if !strings.Contains(b.String(), "not measured: run with -github=false") {
		t.Errorf("a GitHub row does not name the flag:\n%s", b.String())
	}
}

var sectionRow = regexp.MustCompile("(?m)^\\| [^|]+ \\| `([a-z_]+)` \\| ([^|]+) \\|$")

// ci-cd.md §11 is where the owner reads the thresholds, thresholds.json where the tool does: the
// two say the same.
func TestTheDocumentAndTheFileHoldTheSameThresholds(t *testing.T) {
	doc, err := os.ReadFile("../../docs/architecture/ci-cd.md")
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(doc), "\n## 11. ")
	if !ok {
		t.Fatal("ci-cd.md has no §11")
	}
	limits, err := readThresholds()
	if err != nil {
		t.Fatal(err)
	}
	documented := map[string]string{}
	for _, m := range sectionRow.FindAllStringSubmatch(section, -1) {
		documented[m[1]] = strings.TrimSpace(m[2])
	}
	for _, row := range rows {
		want := limits[row.id].String()
		if got, ok := documented[row.id]; !ok {
			t.Errorf("ci-cd.md §11 has no row for `%s`", row.id)
		} else if got != want {
			t.Errorf("`%s`: ci-cd.md §11 says %q, thresholds.json %q", row.id, got, want)
		}
		delete(documented, row.id)
	}
	for id := range documented {
		t.Errorf("ci-cd.md §11 has a row for `%s`, which the report does not print", id)
	}
}
