// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	filledReviewer = "- **Reviewed by:** the reviewer subagent (model r-1), 2026-11-21"
	filledCounts   = "- **Findings:** found 3, fixed 2, open 1"
	filledOutside  = "- **Outside reviewer:** comments 4, fixed 1, declined 3"
)

// reviewOf is the filled description with its Review section replaced.
func reviewOf(text string) string {
	i := strings.Index(filled, "## Review")
	j := strings.Index(filled, "## Does this need an ADR?")
	return filled[:i] + "## Review\n\n" + text + "\n\n" + filled[j:]
}

func TestReviewProblems(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		draft bool
		want  string // "" = accepted
	}{
		{"filled", filled, false, ""},
		{"the outside reviewer did not run", strings.Replace(filled, filledOutside, "- **Outside reviewer:** not run: the diff is above its file limit", 1), false, ""},
		{"no findings", strings.Replace(filled, filledCounts, "- **Findings:** found 0, fixed 0, open 0", 1), false, ""},
		{"the section is absent - the template check names it", strings.Replace(filled, "## Review", "## Reviewers", 1), false, ""},
		{"the template's lines left", reviewOf("- **Reviewed by:** model, YYYY-MM-DD\n- **Findings:** found n, fixed n, open n\n- **Outside reviewer:** comments n, fixed n, declined n"), false,
			"who reviewed and when|found, fixed and left open|outside reviewer"},
		{"prose only", reviewOf("Reviewed, all fine."), false, "who reviewed and when|found, fixed and left open|outside reviewer"},
		{"no reviewer line", strings.Replace(filled, filledReviewer+"\n", "", 1), false, "who reviewed and when"},
		{"a date without a reviewer", strings.Replace(filled, filledReviewer, "- **Reviewed by:** 2026-11-21", 1), false, "who reviewed and when"},
		{"a reviewer without a date", strings.Replace(filled, filledReviewer, "- **Reviewed by:** the reviewer subagent, yesterday", 1), false, "who reviewed and when"},
		{"a date that is none", strings.Replace(filled, filledReviewer, "- **Reviewed by:** the reviewer subagent, 2026-13-41", 1), false, "who reviewed and when"},
		{"a count missing", strings.Replace(filled, filledCounts, "- **Findings:** found 3, fixed 3", 1), false, "found, fixed and left open"},
		{"counts in words", strings.Replace(filled, filledCounts, "- **Findings:** a few, all fixed", 1), false, "found, fixed and left open"},
		{"no outside reviewer row", strings.Replace(filled, filledOutside+"\n", "", 1), false, "outside reviewer"},
		{"not run without a reason", strings.Replace(filled, filledOutside, "- **Outside reviewer:** not run:", 1), false, "outside reviewer"},
		{"not yet, on a draft", reviewOf("not yet: the independent review of stage 1 is running"), true, ""},
		{"not yet, leaving draft", reviewOf("not yet: the independent review of stage 1 is running"), false, "not yet"},
		{"not yet without a reason, on a draft", reviewOf("not yet:"), true, "who reviewed and when"},
		{"a draft with the review done", filled, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(reviewProblems(c.body, c.draft), "\n")
			if c.want == "" {
				if got != "" {
					t.Fatalf("refused:\n%s", got)
				}
				return
			}
			for _, want := range strings.Split(c.want, "|") {
				if !strings.Contains(got, want) {
					t.Errorf("want a problem containing %q, got:\n%s", want, got)
				}
			}
		})
	}
}

// A pull request opened before the section existed is held neither to it nor to its form.
func TestTheReviewSectionHoldsFromItsDate(t *testing.T) {
	template := []string{"What and why", "Premises", reviewTitle, "Impact"}
	before := reviewSince.Add(-time.Hour)
	if got := requiredOf(template, before); slices.Contains(got, reviewTitle) || len(got) != 3 {
		t.Errorf("a pull request opened before the rule is asked for %v", got)
	}
	for _, opened := range []time.Time{{}, reviewSince} {
		if got := requiredOf(template, opened); !slices.Equal(got, template) {
			t.Errorf("a pull request opened at %v is asked for %v", opened, got)
		}
	}
}
