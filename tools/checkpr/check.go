// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The rules a pull request description is held to. They are read against the current template
// rather than written down a second time: the headings come from .github/PULL_REQUEST_TEMPLATE.md,
// so a section added there is required here on the same day.
//
// What each rule is for, because each one has been skipped before:
//
//   - Every section of the template is present, in its order. A description written from scratch
//     drops exactly the sections its author judged irrelevant - and "Does this need an ADR?" is
//     the one that matters on a small change to a gate.
//   - The description says which issue it closes, as `Closes #n` at the start of a line (inside
//     backticks GitHub ignores it), or says why there is none, as `No issue: <reason>`.
//   - Use cases names the use cases whose checks the work makes true - each one existing - or says
//     n/a (docs/usecases/README.md).
//   - Affected areas has at least one area ticked.
//   - Exactly one answer to "Does this need an ADR?" is ticked, and a ticked answer that asks
//     "Which one" names it.
//   - Every Definition of Done item is ticked or says n/a: the template's rule is to mark, not to
//     delete.
//   - Impact's four lines no longer carry the template's placeholders.

var (
	heading      = regexp.MustCompile(`(?m)^## (.+?)\s*$`)
	comment      = regexp.MustCompile(`(?s)<!--.*?-->`)
	closesLine   = regexp.MustCompile(`(?m)^Closes #\d+\b`)
	noIssueLine  = regexp.MustCompile(`(?m)^No issue: \S`)
	useCaseID    = regexp.MustCompile(`\bUC-[A-Z]{2,3}-\d{2,3}\b`)
	checkbox     = regexp.MustCompile(`(?m)^- \[( |x|X)\] (.*)$`)
	notApplied   = regexp.MustCompile(`(?i)\bn/a\b`)
	adrNamed     = regexp.MustCompile(`ADR-\d{4}|[a-z0-9-]+\.md §\s?\d+|none by (number|ADR)`)
	placeholders = []string{
		"UC-…: check n — met / not met — confirmed by …",
		"yes / no — if yes: the migration path",
		`the threats touched (T-xx), or "none"`,
		"new processing of personal data? Purpose and retention",
		"new configuration, new dependency, new alerts?",
	}
)

// section is one level-two part of a description.
type section struct {
	title string
	text  string
}

// templateHeadings reads the headings the template has, in order.
func templateHeadings(root string) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(root, ".github", "PULL_REQUEST_TEMPLATE.md")) //nolint:gosec // G304: a fixed path under the repository root
	if err != nil {
		return nil, fmt.Errorf("reading the template: %w", err)
	}
	var titles []string
	for _, match := range heading.FindAllStringSubmatch(string(raw), -1) {
		titles = append(titles, match[1])
	}
	return titles, nil
}

// knownUseCases reads every use case id there is.
func knownUseCases(root string) (map[string]bool, error) {
	files, err := filepath.Glob(filepath.Join(root, "docs", "usecases", "*", "UC-*.md"))
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, file := range files {
		if id := useCaseID.FindString(filepath.Base(file)); id != "" {
			known[id] = true
		}
	}
	return known, nil
}

func sectionsOf(body string) []section {
	locations := heading.FindAllStringSubmatchIndex(body, -1)
	sections := make([]section, 0, len(locations))
	for i, at := range locations {
		end := len(body)
		if i+1 < len(locations) {
			end = locations[i+1][0]
		}
		sections = append(sections, section{
			title: strings.TrimSpace(body[at[2]:at[3]]),
			text:  body[at[1]:end],
		})
	}
	return sections
}

// check answers every problem the description has, in the template's order.
func check(body string, required []string, useCases map[string]bool) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = comment.ReplaceAllString(body, "")
	sections := sectionsOf(body)

	var problems []string
	found := map[string]section{}
	position := map[string]int{}
	for i, s := range sections {
		if _, seen := found[s.title]; !seen {
			found[s.title] = s
			position[s.title] = i
		}
	}

	last := -1
	for _, title := range required {
		s, ok := found[title]
		if !ok {
			problems = append(problems, fmt.Sprintf("the section %q from the template is missing - keep every section, and write n/a where one does not apply", title))
			continue
		}
		if position[title] < last {
			problems = append(problems, fmt.Sprintf("the section %q is out of the template's order", title))
		}
		last = position[title]
		if strings.TrimSpace(s.text) == "" {
			problems = append(problems, fmt.Sprintf("the section %q is empty", title))
		}
	}

	for _, placeholder := range placeholders {
		if strings.Contains(body, placeholder) {
			problems = append(problems, fmt.Sprintf("the template's placeholder %q is still there", placeholder))
		}
	}

	if !closesLine.MatchString(body) && !noIssueLine.MatchString(body) {
		problems = append(problems, "no issue is closed: write `Closes #n` at the start of a line (not in backticks), or `No issue: <reason>`")
	}

	if s, ok := found["Use cases"]; ok {
		cited := useCaseID.FindAllString(s.text, -1)
		if len(cited) == 0 && !notApplied.MatchString(s.text) {
			problems = append(problems, "Use cases names no use case and does not say n/a")
		}
		for _, id := range cited {
			if !useCases[id] {
				problems = append(problems, fmt.Sprintf("Use cases names %s, which does not exist in docs/usecases", id))
			}
		}
	}

	if s, ok := found["Affected areas"]; ok && ticked(s.text) == 0 {
		problems = append(problems, "Affected areas has nothing ticked")
	}

	if s, ok := found["Does this need an ADR?"]; ok {
		answers := checkbox.FindAllStringSubmatch(s.text, -1)
		chosen := 0
		for _, answer := range answers {
			if !isTicked(answer[1]) {
				continue
			}
			chosen++
			if strings.Contains(answer[2], "Which one") || strings.Contains(answer[2], "Where it lives") || strings.Contains(answer[2], "already merged") {
				if strings.Contains(answer[2], "ADR-….") || !adrNamed.MatchString(answer[2]) {
					problems = append(problems, "the ADR answer that is ticked does not name where the rule lives (a subject-document section, an ADR, or \"none by number\" and why)")
				}
			}
		}
		if chosen != 1 {
			problems = append(problems, fmt.Sprintf("Does this need an ADR? has %d answers ticked, not one", chosen))
		}
	}

	if s, ok := found["Definition of Done"]; ok {
		for _, item := range checkbox.FindAllStringSubmatch(s.text, -1) {
			if !isTicked(item[1]) && !notApplied.MatchString(item[2]) {
				problems = append(problems, fmt.Sprintf("Definition of Done: %q is neither ticked nor marked n/a", strings.TrimSpace(item[2])))
			}
		}
	}

	return problems
}

func ticked(text string) int {
	count := 0
	for _, box := range checkbox.FindAllStringSubmatch(text, -1) {
		if isTicked(box[1]) {
			count++
		}
	}
	return count
}

func isTicked(mark string) bool { return mark == "x" || mark == "X" }
