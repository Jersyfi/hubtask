// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rootForTest(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// filled is a description as a person fills the current template: every section, n/a where it
// does not apply.
const filled = `## What and why

Moves the privacy gates into the pull request check, because four tables reached main
uncatalogued.

Closes #246

## Use cases

- UC-PRV-08: check 5 — met — the data job runs PG-7

## Affected areas

- [ ] ` + "`area:core`" + ` — the domain, the application layer, the ports
- [x] ` + "`area:ci`" + ` — workflows, gates, release

## Does this need an ADR?

- [x] No — this implements a decision that is already recorded. Which one: ADR-0015.
- [ ] Yes, and it is in this pull request or already merged: ADR-….
- [ ] It deviates from an existing ADR

## Definition of Done

- [x] Tests at every relevant level green, coverage thresholds held
- [ ] Event schema added under ` + "`api/events/`" + ` — n/a

## Impact

- **Breaking change:** no
- **Security:** none
- **Data protection:** none
- **Operations:** the data job takes a few seconds longer
`

func TestAFilledDescriptionPasses(t *testing.T) {
	root := rootForTest(t)
	required, err := templateHeadings(root)
	if err != nil {
		t.Fatal(err)
	}
	known, err := knownUseCases(root)
	if err != nil {
		t.Fatal(err)
	}
	if problems := check(filled, required, known); len(problems) != 0 {
		t.Errorf("a filled description was refused:\n%s", strings.Join(problems, "\n"))
	}
}

// The template as GitHub offers it, untouched, is the one description that must never pass: it is
// exactly what a pull request looks like when nobody filled it in.
func TestTheUnfilledTemplateIsRefused(t *testing.T) {
	root := rootForTest(t)
	raw, err := os.ReadFile(filepath.Join(root, ".github", "PULL_REQUEST_TEMPLATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	required, _ := templateHeadings(root)
	known, _ := knownUseCases(root)
	problems := check(string(raw), required, known)
	if len(problems) == 0 {
		t.Fatal("the unfilled template passed")
	}
	joined := strings.Join(problems, "\n")
	for _, want := range []string{"no issue is closed", "Affected areas has nothing ticked", "answers ticked", "placeholder"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the unfilled template was not refused for %q:\n%s", want, joined)
		}
	}
}

// missingUseCase is an id no use case carries.
var missingUseCase = "UC-" + "PRV-" + "99"

func TestEachRuleRefusesItsOwnMistake(t *testing.T) {
	root := rootForTest(t)
	required, _ := templateHeadings(root)
	known, _ := knownUseCases(root)

	cases := []struct {
		name   string
		mutate func(string) string
		want   string
	}{
		{"a section dropped", func(s string) string {
			i := strings.Index(s, "## Does this need an ADR?")
			j := strings.Index(s, "## Definition of Done")
			return s[:i] + s[j:]
		}, `"Does this need an ADR?" from the template is missing`},
		{"the issue in backticks", func(s string) string {
			return strings.Replace(s, "Closes #246", "`Closes #246`", 1)
		}, "no issue is closed"},
		// Assembled at run time: written out, the id would be a citation the doc gate refuses.
		{"a use case that does not exist", func(s string) string {
			return strings.Replace(s, "UC-PRV-08", missingUseCase, 1)
		}, missingUseCase + ", which does not exist"},
		{"no use case and no n/a", func(s string) string {
			return strings.Replace(s, "- UC-PRV-08: check 5 — met — the data job runs PG-7", "- something", 1)
		}, "names no use case"},
		{"no area", func(s string) string {
			return strings.Replace(s, "- [x] `area:ci`", "- [ ] `area:ci`", 1)
		}, "Affected areas has nothing ticked"},
		{"two ADR answers", func(s string) string {
			return strings.Replace(s, "- [ ] Yes, and it is", "- [x] Yes, and it is", 1)
		}, "2 answers ticked"},
		{"the ADR not named", func(s string) string {
			return strings.Replace(s, "Which one: ADR-0015.", "Which one: ADR-….", 1)
		}, "does not name the ADR"},
		{"a DoD item left open", func(s string) string {
			return strings.Replace(s, " — n/a", "", 1)
		}, "neither ticked nor marked n/a"},
		{"an Impact placeholder left", func(s string) string {
			return strings.Replace(s, "**Breaking change:** no", "**Breaking change:** yes / no — if yes: the migration path", 1)
		}, "placeholder"},
		{"sections out of order", func(s string) string {
			i := strings.Index(s, "## Impact")
			impact := s[i:]
			j := strings.Index(s, "## Use cases")
			return s[:j] + impact + s[j:i]
		}, "out of the template's order"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			problems := check(c.mutate(filled), required, known)
			if !strings.Contains(strings.Join(problems, "\n"), c.want) {
				t.Errorf("want a problem containing %q, got:\n%s", c.want, strings.Join(problems, "\n"))
			}
		})
	}
}

// A pull request with no issue says why rather than leaving the line empty.
func TestNoIssueWithAReasonPasses(t *testing.T) {
	root := rootForTest(t)
	required, _ := templateHeadings(root)
	known, _ := knownUseCases(root)
	body := strings.Replace(filled, "Closes #246", "No issue: the owner asked for it in the session", 1)
	if problems := check(body, required, known); len(problems) != 0 {
		t.Errorf("a stated reason for no issue was refused:\n%s", strings.Join(problems, "\n"))
	}
}
