// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The readiness record's rules (docs/backlog/ready/README.md), as far as a program can hold them.
//
// The record is where a task's concept is settled before its code: two thirds of the questions that
// stalled milestone SC were answerable from documents before the first line was written
// (2026-10-04). A rule only a session reads is a rule a session can skip, so this checks what can be
// checked - that the record exists for the task the commits carry, was ready before the first code
// commit, is filled rather than copied, and has no decision open. Whether its content is true is the
// independent reviewer's question (/ready-check § 3), not this program's.

// gateSince is the day the gate began to require records. A pull request opened earlier may answer
// n/a: its code was written before the process existed, and a record written now would be written
// after the code - which is what the process forbids.
var gateSince = time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)

var (
	recordPath  = regexp.MustCompile(`docs/backlog/ready/([A-Z][A-Z0-9]*-\d+[a-z]?)\.md`)
	verdictLine = regexp.MustCompile(`(?m)^\*\*Verdict:\*\*\s*(.+?)\s*$`)
	taskLine    = regexp.MustCompile(`(?m)^\*\*Task:\*\*\s*([A-Z][A-Z0-9]*-\d+[a-z]?)\b`)
	sizeLine    = regexp.MustCompile(`(?m)^\*\*Size:\*\*\s*([SML])\b`)
	recordPart  = regexp.MustCompile(`(?m)^## (\d+)\. .*$`)
	openBox     = regexp.MustCompile(`(?m)^\s*- \[ \]`)
	// The template's placeholders: a record that still carries one was copied, not written.
	recordPlaceholders = []string{"<TASK>", "<title>", "<sha>", "<YYYY-MM-DD>", "<n>", "<the question>",
		"<how the independent review ran>", "<X>"}
	// Paths whose change always needs a record, whatever the description says: the contract, the
	// schema, dependencies, the decisions and the yardstick.
	weighty = []string{"api/openapi.yaml", "db/migrations/", "go.mod", "package.json", "pnpm-lock.yaml",
		"docs/adr/", "docs/usecases/"}
	// What makes a task L whatever else it is. A use case's *Today* is updated by the smallest fix,
	// so it is not among them.
	large = []string{"api/openapi.yaml", "db/migrations/", "go.mod", "package.json", "pnpm-lock.yaml", "docs/adr/"}
)

// recordReader reads a file of the branch by its path from the repository root.
type recordReader func(path string) ([]byte, error)

// branchFacts is what the branch's history says, read by git (main.go). Its zero value - a run
// without a base, as `make gate-pr` runs locally - checks the description and the records only.
type branchFacts struct {
	// known is whether the history was read at all.
	known bool
	// tasks are the `Task:` trailers of the branch's own commits.
	tasks []string
	// changed are the paths the branch changes against its base.
	changed []string
	// beforeCode reads a file as it was just before the first commit that touched anything outside
	// docs/ - nil when the branch has no such commit.
	beforeCode recordReader
	// opened is when the pull request was opened; zero when not known.
	opened time.Time
}

func (f branchFacts) touchesWeighty() []string { return f.touching(weighty) }

func (f branchFacts) touchesLarge() []string { return f.touching(large) }

func (f branchFacts) touching(prefixes []string) []string {
	var hit []string
	for _, path := range f.changed {
		for _, prefix := range prefixes {
			if path == prefix || strings.HasPrefix(path, prefix) || strings.HasSuffix(path, "/"+prefix) {
				hit = append(hit, path)
				break
			}
		}
	}
	return hit
}

// readiness holds the Readiness section, and every record it names, to the rules.
func readiness(text, useCasesText string, records recordReader, facts branchFacts) []string {
	named := map[string]string{}
	for _, m := range recordPath.FindAllStringSubmatch(text, -1) {
		named[m[1]] = m[0]
	}
	transitional := !facts.opened.IsZero() && facts.opened.Before(gateSince)

	if len(named) == 0 {
		if transitional && notApplied.MatchString(text) {
			return nil
		}
		var problems []string
		switch {
		case len(facts.tasks) > 0:
			problems = append(problems, fmt.Sprintf("Readiness names no readiness record, and the commits carry Task: %s - write the record first (/ready-check)", strings.Join(facts.tasks, ", ")))
		case len(useCaseID.FindAllString(useCasesText, -1)) > 0:
			problems = append(problems, "Readiness names no readiness record, and the work serves use cases: write the record first (docs/backlog/ready/<TASK>.md, /ready-check)")
		case len(facts.touchesWeighty()) > 0:
			problems = append(problems, fmt.Sprintf("Readiness names no readiness record, and the branch changes %s - the contract, the schema, a dependency, an ADR or a use case always needs one", strings.Join(facts.touchesWeighty(), ", ")))
		case !notApplied.MatchString(text):
			problems = append(problems, "Readiness names no readiness record and does not say n/a")
		}
		return problems
	}

	var problems []string
	for _, task := range facts.tasks {
		if _, ok := named[task]; !ok {
			problems = append(problems, fmt.Sprintf("the commits carry Task: %s, and Readiness does not name docs/backlog/ready/%s.md", task, task))
		}
	}
	for task, path := range named {
		raw, err := records(path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("the readiness record %s does not exist in this branch", path))
			continue
		}
		problems = append(problems, recordProblems(path, task, string(raw), facts)...)
		if facts.beforeCode != nil {
			before, err := facts.beforeCode(path)
			if err != nil || verdictOf(clean(string(before))) != "ready" {
				problems = append(problems, fmt.Sprintf("the readiness record %s was not ready before the first code commit - no code before ready (docs/backlog/ready/README.md, rule 1)", path))
			}
		}
	}
	return problems
}

// recordProblems holds one record to its form.
func recordProblems(path, task, raw string, facts branchFacts) []string {
	record := clean(raw)
	var problems []string

	if m := taskLine.FindStringSubmatch(record); m == nil || m[1] != task {
		problems = append(problems, fmt.Sprintf("the readiness record %s does not say **Task:** %s on its first lines", path, task))
	}
	if verdicts := verdictLine.FindAllStringSubmatch(record, -1); len(verdicts) != 1 {
		problems = append(problems, fmt.Sprintf("the readiness record %s has %d **Verdict:** lines, not one", path, len(verdicts)))
	} else if verdicts[0][1] != "ready" {
		problems = append(problems, fmt.Sprintf("the readiness record %s is not ready (verdict: %s)", path, verdicts[0][1]))
	}
	for _, placeholder := range recordPlaceholders {
		if strings.Contains(record, placeholder) {
			problems = append(problems, fmt.Sprintf("the readiness record %s still carries the template's %s", path, placeholder))
		}
	}

	size := ""
	if m := sizeLine.FindStringSubmatch(record); m != nil {
		size = m[1]
	}
	switch size {
	case "":
		problems = append(problems, fmt.Sprintf("the readiness record %s says no **Size:** (M or L)", path))
	case "S":
		problems = append(problems, fmt.Sprintf("the readiness record %s says Size S - a task needs at least M; S work answers n/a and has no record", path))
	case "M":
		if hit := facts.touchesLarge(); len(hit) > 0 {
			problems = append(problems, fmt.Sprintf("the readiness record %s says Size M, and the branch changes %s - that is L", path, strings.Join(hit, ", ")))
		}
	}

	parts := recordPart.FindAllStringSubmatchIndex(record, -1)
	seen := map[string]bool{}
	for i, at := range parts {
		number := record[at[2]:at[3]]
		seen[number] = true
		end := len(record)
		if i+1 < len(parts) {
			end = parts[i+1][0]
		}
		body := strings.TrimSpace(record[at[1]:end])
		// The escapes are filled during the build; a table head alone is a correct § 10 before it.
		if number != "10" && body == "" {
			problems = append(problems, fmt.Sprintf("the readiness record %s leaves section %s empty - write what it found, or none and why", path, number))
		}
		if number == "8" && openBox.MatchString(body) {
			problems = append(problems, fmt.Sprintf("the readiness record %s has an open decision in § 8", path))
		}
	}
	for n := 1; n <= 10; n++ {
		if !seen[fmt.Sprint(n)] {
			problems = append(problems, fmt.Sprintf("the readiness record %s has no section %d", path, n))
		}
	}
	return problems
}

// clean drops the template's guidance, which lives in comments, so an unfilled section reads empty.
func clean(raw string) string {
	return comment.ReplaceAllString(strings.ReplaceAll(raw, "\r\n", "\n"), "")
}

func verdictOf(record string) string {
	verdicts := verdictLine.FindAllStringSubmatch(record, -1)
	if len(verdicts) != 1 {
		return ""
	}
	return verdicts[0][1]
}
