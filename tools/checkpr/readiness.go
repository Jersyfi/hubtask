// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The rules that read the branch's history (ADR-0081, AGENTS.md "Working rules"):
//
//   - A task is settled before its code. A branch that carries a task (a `Task:` trailer), names a
//     use case, or changes the contract, a migration, the queries, a dependency, an ADR or what a
//     use case promises has a readiness record, docs/backlog/ready/<TASK>.md. It said `ready` or
//     `waiting on the owner` before the first commit outside docs/, and it says `ready`, with no
//     open decision box, when the pull request leaves draft. A change without a task says
//     `Readiness: n/a — <why>` instead.
//   - A merged migration never changes (rule 12) unless the description names the ADR that
//     allows it: `Changes a merged migration: ADR-nnnn`.
//   - A use case is never deleted; one that no longer applies is retired, its ID kept. A new use
//     case never takes an ID an earlier file carried.
//   - A change to a use case's Goal, How to check or Where it ends is named in the description as
//     a correction or as the owner's decision (usecasetext.go).
//   - A settled ADR keeps its text; only its status line, its Rule lives in line and link targets
//     move (adr.go).
//
// What it cannot see: whether the record is good and whether the review was independent, and
// whether the owner did decide what a description says was decided. Those stay with the people
// who read it.

// readinessSince is when the readiness rule took effect. A pull request opened before it was
// written under the earlier process and is not held to it.
var readinessSince = time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)

type change struct{ status, path string }

// branchFacts is what the history says. known is false when no history was read (no -base): the
// description alone is then all there is to check.
type branchFacts struct {
	known      bool
	opened     time.Time
	tasks      []string
	changed    []string
	altered    []change
	ucText     []ucTextChange
	reused     []reusedID
	adrEdited  []string
	beforeCode func(path string) ([]byte, error)
}

// reusedID is a use case the branch adds under an ID an earlier file carried.
type reusedID struct{ id, path, earlier string }

// ucTextChange is a use case whose Goal, How to check or Where it ends the branch changes.
type ucTextChange struct {
	id, path string
	added    bool
}

var (
	readinessNA      = regexp.MustCompile(`(?m)^Readiness:\s*n/a\s*[—-]\s*\S`)
	migrationAllowed = regexp.MustCompile(`(?m)^Changes a merged migration:\s*ADR-\d{4}`)
	verdictLine      = regexp.MustCompile(`(?m)^\*\*Verdict:\*\*\s*(.+?)\s*$`)
	taskLine         = regexp.MustCompile(`(?m)^\*\*Task:\*\*\s*([A-Z][A-Z0-9]*-\d+[a-z]?)\b`)
	htmlComment      = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// weighty are the paths that change what the product promises or depends on: a change there is
// work that needs settling even without a task. An ADR is a decision taken. A use case counts
// through facts.ucText rather than here: only its Goal, How to check and Where it ends say what is
// promised, while state:, checked_by: and Today record what was built and are moved by the work
// itself, so a pull request that only records progress is not held to a record. db/queries/ is
// here beside the migrations because a query is the other half of the data model's contract.
func weighty(path string) bool {
	switch {
	case path == "api/openapi.yaml", path == "go.mod", path == "pnpm-lock.yaml":
		return true
	case strings.HasPrefix(path, "db/migrations/"), strings.HasPrefix(path, "db/queries/"):
		return true
	case strings.HasSuffix(path, "/package.json") || path == "package.json":
		return true
	case strings.HasPrefix(path, "docs/adr/ADR-"):
		return true
	}
	return false
}

func historyProblems(body string, facts branchFacts, read func(string) ([]byte, error)) []string {
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
	if !heldTo(facts.opened, readinessSince) {
		return problems
	}
	return append(problems, readinessProblems(body, facts, read)...)
}

func readinessProblems(body string, facts branchFacts, read func(string) ([]byte, error)) []string {
	reason := ""
	switch {
	case len(facts.tasks) > 0:
		reason = "it carries " + strings.Join(facts.tasks, ", ")
	case useCaseID.MatchString(sectionText(body, "Use cases")):
		reason = "it names a use case"
	case len(facts.ucText) > 0:
		reason = "it changes what " + facts.ucText[0].id + " promises"
	default:
		for _, path := range facts.changed {
			if weighty(path) {
				reason = "it changes " + path
				break
			}
		}
	}
	if reason == "" {
		return nil
	}
	if len(facts.tasks) == 0 {
		if readinessNA.MatchString(body) {
			return nil
		}
		return []string{fmt.Sprintf("this change needs settling first (%s): a task's readiness record (a `Task:` trailer and docs/backlog/ready/<TASK>.md), or, for a change without a task, a line `Readiness: n/a — <why>`", reason)}
	}

	var problems []string
	for _, task := range facts.tasks {
		path := "docs/backlog/ready/" + task + ".md"
		raw, err := read(path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s carries no readiness record: %s does not exist (docs/backlog/ready/TEMPLATE.md)", task, path))
			continue
		}
		record := htmlComment.ReplaceAllString(string(raw), "")
		if m := taskLine.FindStringSubmatch(record); m == nil || m[1] != task {
			problems = append(problems, fmt.Sprintf("%s does not name %s in its **Task:** line", path, task))
		}
		if v := verdict(record); v != "ready" {
			problems = append(problems, fmt.Sprintf("%s says %q - a pull request leaves draft only when its record says `ready`", path, v))
		}
		if strings.Contains(sectionText(record, "3. Decisions"), "- [ ]") {
			problems = append(problems, fmt.Sprintf("%s still has an open decision in section 3", path))
		}
		if facts.beforeCode != nil {
			before, err := facts.beforeCode(path)
			v := verdict(htmlComment.ReplaceAllString(string(before), ""))
			if err != nil || (v != "ready" && v != "waiting on the owner") {
				problems = append(problems, fmt.Sprintf("%s was not ready before the first commit outside docs/ - the record is written, attacked and committed first", path))
			}
		}
	}
	return problems
}

func verdict(record string) string {
	m := verdictLine.FindStringSubmatch(record)
	if m == nil {
		return "no verdict"
	}
	return strings.TrimSpace(m[1])
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
