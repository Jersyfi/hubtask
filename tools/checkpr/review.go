// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"regexp"
	"slices"
	"strings"
	"time"
)

// The Review section is what the owner reads instead of the diff: that a fresh agent reviewed the
// change, how many findings it had and what became of them, and what the outside reviewer said.
// Its three lines are held to their form - a reviewer and a date, the three counts, the outside
// reviewer's counts or why it did not run. What this cannot see is whether a review ran, whether
// it was good, or whether the counts are true; that stays with the people who read it.
//
// A draft may say `not yet: <why>` instead (-draft): its review comes before it leaves draft, and
// CI reads only a ready pull request, without the flag.

// reviewSince is when the section entered the template.
var reviewSince = time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)

const reviewTitle = "Review"

var (
	reviewedBy    = regexp.MustCompile(`(?m)^- \*\*Reviewed by:\*\*(.*)$`)
	isoDate       = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	letter        = regexp.MustCompile(`\pL`)
	findingCounts = regexp.MustCompile(`(?m)^- \*\*Findings:\*\*\s*found \d+, fixed \d+, open \d+\b`)
	outsideRow    = regexp.MustCompile(`(?m)^- \*\*Outside reviewer:\*\*\s*(?:comments \d+, fixed \d+, declined \d+\b|not run: \S)`)
	reviewNotYet  = regexp.MustCompile(`(?m)^not yet: \S`)
)

// requiredOf is the template's sections a pull request opened at opened is held to.
func requiredOf(template []string, opened time.Time) []string {
	if heldTo(opened, reviewSince) {
		return template
	}
	return slices.DeleteFunc(slices.Clone(template), func(title string) bool { return title == reviewTitle })
}

// reviewProblems holds the Review section to its form. An absent or empty section is the template
// check's to name.
func reviewProblems(body string, draft bool) []string {
	body = htmlComment.ReplaceAllString(strings.ReplaceAll(body, "\r\n", "\n"), "")
	text := ""
	for _, s := range sectionsOf(body) {
		if s.title == reviewTitle {
			text = s.text
			break
		}
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if reviewNotYet.MatchString(text) {
		if draft {
			return nil
		}
		return []string{"Review says `not yet` - the independent review comes before a pull request leaves draft (.agents/skills/finish-pull-request/SKILL.md); only a draft's description may say so (`make gate-pr DRAFT=1`)"}
	}

	var problems []string
	if !namesReviewerAndDate(text) {
		problems = append(problems, "Review does not say who reviewed and when: `- **Reviewed by:** <model>, <YYYY-MM-DD>`")
	}
	if !findingCounts.MatchString(text) {
		problems = append(problems, "Review does not count the findings found, fixed and left open: `- **Findings:** found <n>, fixed <n>, open <n>`, then one line per open finding with its reason")
	}
	if !outsideRow.MatchString(text) {
		problems = append(problems, "Review has no row for the outside reviewer: `- **Outside reviewer:** comments <n>, fixed <n>, declined <n>`, or `not run: <why>`")
	}
	return problems
}

func namesReviewerAndDate(text string) bool {
	line := reviewedBy.FindStringSubmatch(text)
	if line == nil {
		return false
	}
	date := isoDate.FindString(line[1])
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return false
	}
	return letter.MatchString(strings.Replace(line[1], date, "", 1))
}
