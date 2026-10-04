// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

## Readiness

Record: ` + "`docs/backlog/ready/PG-07.md`" + `
Escapes: none

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
	if problems := check(filled, required, known, readyRecords, branchFacts{}); len(problems) != 0 {
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
	problems := check(string(raw), required, known, readyRecords, branchFacts{})
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

// readyRecord is a readiness record that passes: verdict ready, ten sections, every decision taken.
const readyRecord = `# PG-07 — the privacy gates · readiness record

**Task:** PG-07 · issue #246
**Size:** L
**Verdict:** ready
**Checked:** 2026-10-04 against main

<!-- The verdict is exactly one of: ready / waiting on the owner / not ready. - [ ] in a comment is not a decision. -->

## 1. Premise — what is true today
The gates run nightly only - true, ci.yml.
## 2. Coverage — every check and promise this task touches
UC-PRV-08 check 5 - this task.
## 3. Doors and states
none - a CI change.
## 4. Standing rules
none touched.
## 5. Concurrency and transactions
none.
## 6. Known traps
none apply.
## 7. Acceptance — and how each is proven
the job runs PG-7.
## 8. Decisions
- [x] D1 — run it on every pull request — decided by ci-cd.md §5
## 9. Independent review
no findings.
## 10. Escapes
| # | Found by | Class | What | Row | Handled |
|---|---|---|---|---|---|
`

// readyRecords answers the records the descriptions in these tests name.
func readyRecords(path string) ([]byte, error) {
	records := map[string]string{"docs/backlog/ready/PG-07.md": readyRecord}
	record, ok := records[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(record), nil
}

// The readiness record is what keeps a pull request from being merged past an unanswered
// question; each way of slipping past it is refused.
func TestTheReadinessRecordIsHeldToItsRules(t *testing.T) {
	root := rootForTest(t)
	required, _ := templateHeadings(root)
	known, _ := knownUseCases(root)
	withRecord := func(record string) recordReader {
		return func(path string) ([]byte, error) {
			if path != "docs/backlog/ready/PG-07.md" {
				return nil, os.ErrNotExist
			}
			return []byte(record), nil
		}
	}
	cases := []struct {
		name    string
		body    string
		records recordReader
		want    string
	}{
		{"no record although the work serves a use case",
			strings.Replace(filled, "Record: `docs/backlog/ready/PG-07.md`", "Record: n/a — small", 1),
			readyRecords, "names no readiness record, and the work serves use cases"},
		{"a record that is not in the branch",
			strings.Replace(filled, "PG-07.md", "PG-99.md", 1), readyRecords, "does not exist in this branch"},
		{"a record still waiting on the owner", filled,
			withRecord(strings.Replace(readyRecord, "**Verdict:** ready", "**Verdict:** waiting on the owner", 1)),
			"is not ready (verdict: waiting on the owner)"},
		{"a record without a verdict", filled,
			withRecord(strings.Replace(readyRecord, "**Verdict:** ready\n", "", 1)), "has 0 **Verdict:** lines"},
		{"two verdicts", filled,
			withRecord(readyRecord + "\n**Verdict:** ready\n"), "has 2 **Verdict:** lines"},
		{"another task's record", filled,
			withRecord(strings.Replace(readyRecord, "**Task:** PG-07", "**Task:** PG-08", 1)), "does not say **Task:** PG-07"},
		{"a copied template", filled,
			withRecord(strings.Replace(readyRecord, "The gates run nightly only - true, ci.yml.", "<the question>", 1)), "still carries the template's <the question>"},
		{"an empty section", filled,
			withRecord(strings.Replace(readyRecord, "## 6. Known traps\nnone apply.\n", "## 6. Known traps\n<!-- guidance only -->\n", 1)), "leaves section 6 empty"},
		{"no size", filled,
			withRecord(strings.Replace(readyRecord, "**Size:** L\n", "", 1)), "says no **Size:**"},
		{"an open decision", filled,
			withRecord(strings.Replace(readyRecord, "## 9. Independent review", "- [ ] D2 — who may remove it — owner\n## 9. Independent review", 1)),
			"has an open decision in § 8"},
		{"a section missing", filled,
			withRecord(strings.Replace(readyRecord, "## 5. Concurrency and transactions\nnone.\n", "", 1)), "has no section 5"},
		{"the template's placeholder", strings.Replace(filled, "docs/backlog/ready/PG-07.md", "docs/backlog/ready/<TASK>.md", 1),
			readyRecords, "placeholder"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			problems := check(c.body, required, known, c.records, branchFacts{})
			if !strings.Contains(strings.Join(problems, "\n"), c.want) {
				t.Errorf("want a problem containing %q, got:\n%s", c.want, strings.Join(problems, "\n"))
			}
		})
	}

	// Work that serves no use case - tooling, CI, a document - may say n/a, with its reason.
	tooling := strings.Replace(filled, "- UC-PRV-08: check 5 — met — the data job runs PG-7", "n/a — a gate's own tooling", 1)
	tooling = strings.Replace(tooling, "Record: `docs/backlog/ready/PG-07.md`", "n/a — no use case, no task", 1)
	if problems := check(tooling, required, known, readyRecords, branchFacts{}); len(problems) != 0 {
		t.Errorf("tooling work answering n/a was refused:\n%s", strings.Join(problems, "\n"))
	}
}

// What the branch's history says cannot be talked around in the description.
func TestTheHistoryHoldsTheRecordToTheTaskAndToItsOrder(t *testing.T) {
	root := rootForTest(t)
	required, _ := templateHeadings(root)
	known, _ := knownUseCases(root)
	tooling := strings.Replace(filled, "- UC-PRV-08: check 5 — met — the data job runs PG-7", "n/a — a gate's own tooling", 1)
	tooling = strings.Replace(tooling, "Record: `docs/backlog/ready/PG-07.md`", "n/a — no use case, no task", 1)
	waiting := func(string) ([]byte, error) {
		return []byte(strings.Replace(readyRecord, "**Verdict:** ready", "**Verdict:** waiting on the owner", 1)), nil
	}
	cases := []struct {
		name  string
		body  string
		facts branchFacts
		want  string
	}{
		{"n/a while the commits carry a task", tooling,
			branchFacts{known: true, tasks: []string{"PG-07"}}, "the commits carry Task: PG-07"},
		{"n/a while the branch changes the contract", tooling,
			branchFacts{known: true, changed: []string{"api/openapi.yaml"}}, "the contract, the schema"},
		{"a record for another task than the commits carry", filled,
			branchFacts{known: true, tasks: []string{"PG-07", "PG-08"}}, "Readiness does not name docs/backlog/ready/PG-08.md"},
		{"code before the record was ready", filled,
			branchFacts{known: true, tasks: []string{"PG-07"}, beforeCode: waiting}, "was not ready before the first code commit"},
		{"size M on a migration", filled,
			branchFacts{known: true, changed: []string{"db/migrations/0120_x.sql"}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			records := readyRecords
			if c.name == "size M on a migration" {
				records = func(string) ([]byte, error) {
					return []byte(strings.Replace(readyRecord, "**Size:** L", "**Size:** M", 1)), nil
				}
				c.want = "says Size M, and the branch changes db/migrations/0120_x.sql"
			}
			problems := check(c.body, required, known, records, c.facts)
			if !strings.Contains(strings.Join(problems, "\n"), c.want) {
				t.Errorf("want a problem containing %q, got:\n%s", c.want, strings.Join(problems, "\n"))
			}
		})
	}

	// A pull request opened before the gate existed may answer n/a: its record would be written
	// after its code.
	before := branchFacts{known: true, tasks: []string{"PG-07"}, opened: gateSince.Add(-time.Hour)}
	if problems := check(tooling, required, known, readyRecords, before); len(problems) != 0 {
		t.Errorf("a pull request from before the gate was refused:\n%s", strings.Join(problems, "\n"))
	}
}

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
			problems := check(c.mutate(filled), required, known, readyRecords, branchFacts{})
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
	if problems := check(body, required, known, readyRecords, branchFacts{}); len(problems) != 0 {
		t.Errorf("a stated reason for no issue was refused:\n%s", strings.Join(problems, "\n"))
	}
}
