// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const readyRecord = "# PH-02 — Extending a deadline\n\n**Task:** PH-02 · issue #1087\n**Verdict:** ready <!-- exactly one of -->\n\n" +
	"## 1. Premise — what is true today\n\nThe deadline is fixed · true · core/x.go:12\n\n" +
	"## 2. Coverage and the matrix\n\n<!-- guidance -->\n(a) check 1 → this task\n\n" +
	"## 3. Decisions\n\n- [x] D1 … — decided by: worker\n\n" +
	"## 4. Proof and steps\n\n1. The domain rule.\n\n" +
	"## 5. Review\n\nA second agent, fresh context: no findings; checked § 1 against the code.\n"

func files(m map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if s, ok := m[p]; ok {
			return []byte(s), nil
		}
		return nil, errors.New("no such file")
	}
}

func TestHistoryRules(t *testing.T) {
	after := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	record := map[string]string{"docs/backlog/ready/PH-02.md": readyRecord}
	beforeReady := func(string) ([]byte, error) { return []byte(readyRecord), nil }

	cases := []struct {
		name  string
		body  string
		facts branchFacts
		files map[string]string
		want  string // "" = no problem
	}{
		{"no history read", "", branchFacts{}, nil, ""},
		{"a task with a ready record", "", branchFacts{known: true, opened: after, tasks: []string{"PH-02"}, beforeCode: beforeReady}, record, ""},
		{"a task without a record", "", branchFacts{known: true, opened: after, tasks: []string{"PH-02"}}, nil, "carries no readiness record"},
		{"a record not ready at the head", "", branchFacts{known: true, opened: after, tasks: []string{"PH-02"}},
			map[string]string{"docs/backlog/ready/PH-02.md": strings.Replace(readyRecord, "**Verdict:** ready", "**Verdict:** waiting on the owner", 1)}, "leaves draft only when its record says `ready`"},
		{"an open decision box", "", branchFacts{known: true, opened: after, tasks: []string{"PH-02"}},
			map[string]string{"docs/backlog/ready/PH-02.md": strings.Replace(readyRecord, "- [x] D1", "- [ ] O1", 1)}, "open decision"},
		{"a record written after the code", "", branchFacts{known: true, opened: after, tasks: []string{"PH-02"},
			beforeCode: func(string) ([]byte, error) { return nil, errors.New("absent") }}, record, "was not ready before the first commit outside docs/"},
		{"waiting on the owner before the code is fine", "", branchFacts{known: true, opened: after, tasks: []string{"PH-02"},
			beforeCode: func(string) ([]byte, error) {
				return []byte(strings.Replace(readyRecord, "**Verdict:** ready", "**Verdict:** waiting on the owner", 1)), nil
			}}, record, ""},
		{"a record without a review", "", branchFacts{known: true, opened: after, tasks: []string{"PH-02"}},
			map[string]string{"docs/backlog/ready/PH-02.md": readyRecord[:strings.Index(readyRecord, "## 5. Review")] + "## 5. Review\n\n<!-- Who attacked this record -->\n"}, "a record without a review"},
		{"a record whose review heading is gone", "", branchFacts{known: true, opened: after, tasks: []string{"PH-02"}},
			map[string]string{"docs/backlog/ready/PH-02.md": readyRecord[:strings.Index(readyRecord, "## 5. Review")]}, "a record without a review"},
		{"an empty premise", "", branchFacts{known: true, opened: after, tasks: []string{"PH-02"}},
			map[string]string{"docs/backlog/ready/PH-02.md": strings.Replace(readyRecord, "The deadline is fixed · true · core/x.go:12\n", "<!-- claims -->\n", 1)}, "nothing under section 1"},
		{"an empty proof section", "", branchFacts{known: true, opened: after, tasks: []string{"PH-02"}},
			map[string]string{"docs/backlog/ready/PH-02.md": strings.Replace(readyRecord, "1. The domain rule.\n", "", 1)}, "nothing under section 4"},
		{"a record naming another task", "", branchFacts{known: true, opened: after, tasks: []string{"PH-02"}},
			map[string]string{"docs/backlog/ready/PH-02.md": strings.Replace(readyRecord, "**Task:** PH-02", "**Task:** PH-03", 1)}, "does not name PH-02"},
		{"a contract change without a task", "", branchFacts{known: true, opened: after, changed: []string{"api/openapi.yaml"}}, nil, "needs settling first (it changes api/openapi.yaml)"},
		{"a contract change that says n/a", "Readiness: n/a — the description of a field only", branchFacts{known: true, opened: after, changed: []string{"api/openapi.yaml"}}, nil, ""},
		{"a use case named without a task", "## Use cases\n\n- UC-PRV-01: check 9 — met", branchFacts{known: true, opened: after}, nil, "it names a use case"},
		{"a documentation change needs nothing", "", branchFacts{known: true, opened: after, changed: []string{"docs/architecture/ci-cd.md"}}, nil, ""},
		{"an ADR without a task", "", branchFacts{known: true, opened: after, changed: []string{"docs/adr/ADR-0081-x.md", "docs/adr/README.md"}}, nil, "needs settling first (it changes docs/adr/ADR-0081-x.md)"},
		{"the ADR index alone needs nothing", "", branchFacts{known: true, opened: after, changed: []string{"docs/adr/README.md"}}, nil, ""},
		{"a use case's promise changed without a task", "", branchFacts{known: true, opened: after, changed: []string{"docs/usecases/work/UC-WRK-01-x.md"},
			ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md", added: true}}}, nil, "needs settling first (it changes what UC-WRK-01 promises)"},
		{"a use case's state moved needs nothing", "", branchFacts{known: true, opened: after, changed: []string{"docs/usecases/work/UC-WRK-01-x.md"}}, nil, ""},
		{"opened before the rule", "", branchFacts{known: true, opened: before, tasks: []string{"PH-02"}}, nil, ""},
		{"a merged migration changed", "", branchFacts{known: true, opened: before, altered: []change{{"M", "db/migrations/0001_init.sql"}}}, nil, "is a merged migration"},
		{"a merged migration changed by an ADR", "Changes a merged migration: ADR-0052", branchFacts{known: true, opened: before, altered: []change{{"M", "db/migrations/0001_init.sql"}}}, nil, ""},
		{"a use case deleted", "", branchFacts{known: true, opened: before, altered: []change{{"D", "docs/usecases/work/UC-WRK-01-x.md"}}}, nil, "never removed"},
		{"a use case's checks changed without a word", "", branchFacts{known: true, opened: after, ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md"}}}, nil, "changes the Goal, How to check or Where it ends of UC-WRK-01"},
		{"a use case's checks changed as a correction", "Readiness: n/a — a wrong number\n\n## Use cases\n\n- UC-WRK-01: check 4 — correction, it named check 5\n",
			branchFacts{known: true, opened: after, ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md"}}}, nil, ""},
		{"a use case's checks changed by a decision", "Readiness: n/a — the owner's answer\n\n## Use cases\n\n- UC-WRK-01: Where it ends — decision #1201\n",
			branchFacts{known: true, opened: after, ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md"}}}, nil, ""},
		{"the word on another use case's line", "Readiness: n/a — x\n\n## Use cases\n\n- UC-WRK-01: check 1 — met\n- UC-WRK-02: correction\n",
			branchFacts{known: true, opened: after, ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md"}}}, nil, "of UC-WRK-01"},
		{"a new use case needs no correction", "Readiness: n/a — the owner asked for it", branchFacts{known: true, opened: after, ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md", added: true}}}, nil, ""},
		{"a use case's checks changed before the rule", "", branchFacts{known: true, opened: before, ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md"}}}, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(historyProblems(c.body, c.facts, files(c.files)), "\n")
			if c.want == "" && got != "" {
				t.Fatalf("want no problem, got:\n%s", got)
			}
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Fatalf("want a problem containing %q, got:\n%s", c.want, got)
			}
		})
	}
}

// The template copied unchanged is guidance only: once its comments are stripped, every one of its
// numbered sections reads as empty - which also proves the gate finds the template's headings.
func TestTemplateSectionsAreEmpty(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "backlog", "ready", "TEMPLATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	record := htmlComment.ReplaceAllString(string(raw), "")
	for n := 1; n <= recordSections; n++ {
		if !strings.Contains(string(raw), fmt.Sprintf("\n## %d. ", n)) {
			t.Errorf("the template has no section %d", n)
		}
		if body := strings.TrimSpace(numberedSection(record, n)); body != "" {
			t.Errorf("section %d of the template carries text outside a comment: %q", n, body)
		}
	}
}

const useCaseFile = "---\nid: x\n---\n\n## Goal\n\nA person keeps a hub private.\n\n## How to check\n\n1. A person sees it.\n\n## Where it ends\n\nHere.\n\n## Today\n\n* Check 1: not met — x\n"

// The history is read from a real repository: the trailer, the changed paths, the use cases whose
// owner-held text changed, and the record as it stood before the first commit outside docs/.
func TestReadHistory(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		cmd.Env = scratchGitEnv()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(p, s string) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("db/migrations/0001_init.sql", "-- one\n")
	write("docs/usecases/work/UC-WRK-01-a.md", useCaseFile)
	write("docs/usecases/work/UC-WRK-02-b.md", useCaseFile)
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	run("checkout", "-q", "-b", "work")
	write("docs/backlog/ready/PH-02.md", readyRecord)
	run("add", "-A")
	run("commit", "-q", "-m", "docs: the record\n\nTask: PH-02")
	write("core/x.go", "package x\n")
	write("db/migrations/0001_init.sql", "-- changed\n")
	write("docs/usecases/work/UC-WRK-01-a.md", strings.Replace(useCaseFile, "1. A person sees it.", "1. A person sees it at once.", 1))
	write("docs/usecases/work/UC-WRK-02-b.md", strings.Replace(useCaseFile, "* Check 1: not met", "* Check 1: not met in the web app", 1))
	write("docs/usecases/work/UC-WRK-03-c.md", strings.Replace(useCaseFile, "A person", "Another person", 1))
	run("add", "-A")
	run("commit", "-q", "-m", "feat: the code\n\nTask: PH-02")

	facts, err := readHistory(dir, "main", "work", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.ucText) != 2 || facts.ucText[0].id != "UC-WRK-01" || facts.ucText[0].added ||
		facts.ucText[1].id != "UC-WRK-03" || !facts.ucText[1].added {
		t.Errorf("the use case text changes: want UC-WRK-01 changed and UC-WRK-03 added, a Today edit not counted; got %+v", facts.ucText)
	}
	if len(facts.tasks) != 1 || facts.tasks[0] != "PH-02" {
		t.Errorf("tasks: %v", facts.tasks)
	}
	if facts.beforeCode == nil {
		t.Fatal("the first code commit was not found")
	}
	if raw, err := facts.beforeCode("docs/backlog/ready/PH-02.md"); err != nil || !strings.Contains(string(raw), "**Verdict:** ready") {
		t.Errorf("the record before the code: %q %v", raw, err)
	}
	found := false
	for _, c := range facts.altered {
		if c.path == "db/migrations/0001_init.sql" && c.status == "M" {
			found = true
		}
	}
	if !found {
		t.Errorf("the changed migration was not seen: %v", facts.altered)
	}
}
