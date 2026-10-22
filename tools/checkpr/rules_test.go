// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryRules(t *testing.T) {
	after := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name  string
		body  string
		facts branchFacts
		want  string // "" = no problem
	}{
		{"no history read", "", branchFacts{}, ""},
		{"a merged migration changed", "", branchFacts{known: true, opened: before, altered: []change{{"M", "db/migrations/0001_init.sql"}}}, "is a merged migration"},
		{"a merged migration changed by an ADR", "Changes a merged migration: ADR-0052", branchFacts{known: true, opened: before, altered: []change{{"M", "db/migrations/0001_init.sql"}}}, ""},
		{"a use case deleted", "", branchFacts{known: true, opened: before, altered: []change{{"D", "docs/usecases/work/UC-WRK-01-x.md"}}}, "never removed"},
		{"a use case's checks changed without a word", "", branchFacts{known: true, opened: after, ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md"}}}, "changes the Goal, How to check or Where it ends of UC-WRK-01"},
		{"a use case's checks changed as a correction", "Readiness: n/a — a wrong number\n\n## Use cases\n\n- UC-WRK-01: check 4 — correction, it named check 5\n",
			branchFacts{known: true, opened: after, ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md"}}}, ""},
		{"a use case's checks changed by a decision", "Readiness: n/a — the owner's answer\n\n## Use cases\n\n- UC-WRK-01: Where it ends — decision #1201\n",
			branchFacts{known: true, opened: after, ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md"}}}, ""},
		{"the word on another use case's line", "Readiness: n/a — x\n\n## Use cases\n\n- UC-WRK-01: check 1 — met\n- UC-WRK-02: correction\n",
			branchFacts{known: true, opened: after, ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md"}}}, "of UC-WRK-01"},
		{"a new use case needs no correction", "Readiness: n/a — the owner asked for it", branchFacts{known: true, opened: after, ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md", added: true}}}, ""},
		{"a use case's checks changed before the rule", "", branchFacts{known: true, opened: before, ucText: []ucTextChange{{id: "UC-WRK-01", path: "docs/usecases/work/UC-WRK-01-x.md"}}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(historyProblems(c.body, c.facts), "\n")
			if c.want == "" && got != "" {
				t.Fatalf("want no problem, got:\n%s", got)
			}
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Fatalf("want a problem containing %q, got:\n%s", c.want, got)
			}
		})
	}
}

const useCaseFile = "---\nid: x\n---\n\n## Goal\n\nA person keeps a hub private.\n\n## How to check\n\n1. A person sees it.\n\n## Where it ends\n\nHere.\n\n## Today\n\n* Check 1: not met — x\n"

// The history is read from a real repository: the changed paths and the use cases whose owner-held
// text changed.
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
