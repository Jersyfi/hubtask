// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const readyRecord = "# PH-02 — Extending a deadline\n\n**Task:** PH-02 · issue #1087\n**Verdict:** ready <!-- exactly one of -->\n\n## 3. Decisions\n\n- [x] D1 … — decided by: worker\n\n## 4. Proof and steps\n"

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
		{"a record naming another task", "", branchFacts{known: true, opened: after, tasks: []string{"PH-02"}},
			map[string]string{"docs/backlog/ready/PH-02.md": strings.Replace(readyRecord, "**Task:** PH-02", "**Task:** PH-03", 1)}, "does not name PH-02"},
		{"a contract change without a task", "", branchFacts{known: true, opened: after, changed: []string{"api/openapi.yaml"}}, nil, "needs settling first (it changes api/openapi.yaml)"},
		{"a contract change that says n/a", "Readiness: n/a — the description of a field only", branchFacts{known: true, opened: after, changed: []string{"api/openapi.yaml"}}, nil, ""},
		{"a use case named without a task", "## Use cases\n\n- UC-PRV-01: check 9 — met", branchFacts{known: true, opened: after}, nil, "it names a use case"},
		{"a documentation change needs nothing", "", branchFacts{known: true, opened: after, changed: []string{"docs/architecture/ci-cd.md"}}, nil, ""},
		{"opened before the rule", "", branchFacts{known: true, opened: before, tasks: []string{"PH-02"}}, nil, ""},
		{"a merged migration changed", "", branchFacts{known: true, opened: before, altered: []change{{"M", "db/migrations/0001_init.sql"}}}, nil, "is a merged migration"},
		{"a merged migration changed by an ADR", "Changes a merged migration: ADR-0052", branchFacts{known: true, opened: before, altered: []change{{"M", "db/migrations/0001_init.sql"}}}, nil, ""},
		{"a use case deleted", "", branchFacts{known: true, opened: before, altered: []change{{"D", "docs/usecases/work/UC-WRK-01-x.md"}}}, nil, "never removed"},
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

// The history is read from a real repository: the trailer, the changed paths, and the record as it
// stood before the first commit outside docs/.
func TestReadHistory(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
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
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	run("checkout", "-q", "-b", "work")
	write("docs/backlog/ready/PH-02.md", readyRecord)
	run("add", "-A")
	run("commit", "-q", "-m", "docs: the record\n\nTask: PH-02")
	write("core/x.go", "package x\n")
	write("db/migrations/0001_init.sql", "-- changed\n")
	run("add", "-A")
	run("commit", "-q", "-m", "feat: the code\n\nTask: PH-02")

	facts, err := readHistory(dir, "main", "work", time.Time{})
	if err != nil {
		t.Fatal(err)
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
