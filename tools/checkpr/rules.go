// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The rules that read the branch's history (ADR-0081):
//
//   - A merged migration never changes (rule 12) unless the description names the ADR that
//     allows it: `Changes a merged migration: ADR-nnnn`.
//   - A use case is never deleted; one that no longer applies is retired, its ID kept. A new use
//     case never takes an ID an earlier file carried.
//   - A change to a use case's Goal, How to check or Where it ends is named in the description as
//     a correction or as the owner's decision (usecasetext.go).
//   - A settled ADR keeps its text; only its status line, its Rule lives in line and link targets
//     move (adr.go).
//   - A numbered section of a subject document keeps its number; a section whose content moved
//     keeps its heading with one sentence (sections.go).
//
// What it cannot see: whether the owner did decide what a description says was decided. That
// stays with the people who read it.

// rulesSince is when the rules on the history took effect. A pull request opened before it was
// written under the earlier process and is not held to them.
var rulesSince = time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)

type change struct{ status, path string }

// branchFacts is what the history says. known is false when no history was read (no -base): the
// description alone is then all there is to check.
type branchFacts struct {
	known     bool
	opened    time.Time
	altered   []change
	ucText    []ucTextChange
	reused    []reusedID
	adrEdited []string
	// sectionsLost are the numbered sections of subject documents the head drops (sections.go).
	sectionsLost []lostSection
}

// reusedID is a use case the branch adds under an ID an earlier file carried.
type reusedID struct{ id, path, earlier string }

// ucTextChange is a use case whose Goal, How to check or Where it ends the branch changes.
type ucTextChange struct {
	id, path string
	added    bool
}

var (
	migrationAllowed = regexp.MustCompile(`(?m)^Changes a merged migration:\s*ADR-\d{4}`)
	htmlComment      = regexp.MustCompile(`(?s)<!--.*?-->`)
)

func historyProblems(body string, facts branchFacts) []string {
	if !facts.known {
		return nil
	}
	var problems []string

	for _, c := range facts.altered {
		if strings.HasPrefix(c.path, "db/migrations/") && strings.HasSuffix(c.path, ".sql") && !migrationAllowed.MatchString(body) {
			problems = append(problems, fmt.Sprintf("%s is a merged migration and this branch changes it (rule 12) - write a new migration, or name the ADR that allows it: `Changes a merged migration: ADR-nnnn`", c.path))
		}
		if c.status == "D" && strings.HasPrefix(c.path, "docs/usecases/") && strings.Contains(c.path, "/UC-") {
			problems = append(problems, fmt.Sprintf("%s is deleted - a use case is retired (state: retired, with why), never removed, and its ID is never reused", c.path))
		}
	}
	for _, r := range facts.reused {
		problems = append(problems, fmt.Sprintf("%s takes the ID %s, which %s carried - an ID is never reused; a new use case takes the next free number", r.path, r.id, r.earlier))
	}

	if heldTo(facts.opened, ucTextSince) {
		problems = append(problems, ucTextProblems(body, facts.ucText)...)
	}
	if heldTo(facts.opened, adrSince) {
		problems = append(problems, adrProblems(facts.adrEdited)...)
	}
	if heldTo(facts.opened, sectionsSince) {
		problems = append(problems, sectionProblems(facts.sectionsLost)...)
	}
	return problems
}

// heldTo is whether a pull request opened at opened answers to a rule that took effect at since.
// An unknown opening - a local run - is always held: the rule applies to what is written now.
func heldTo(opened, since time.Time) bool {
	return opened.IsZero() || !opened.Before(since)
}

// sectionText returns what stands under "## <title>" up to the next "## ".
func sectionText(doc, title string) string {
	i := strings.Index(doc, "## "+title)
	if i < 0 {
		return ""
	}
	rest := doc[i+len("## "+title):]
	if j := strings.Index(rest, "\n## "); j >= 0 {
		rest = rest[:j]
	}
	return rest
}
