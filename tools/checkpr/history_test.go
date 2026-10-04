// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The history is read from a real repository: a record committed waiting, code, then the record
// ready - the order the process forbids - is seen as such, and the Task trailers are found.
func TestTheHistoryIsReadFromGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		out, err := git(dir, args...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	write := func(path, content string) {
		t.Helper()
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(message string) {
		t.Helper()
		run("add", "-A")
		run("-c", "user.name=t", "-c", "user.email=t@example.org", "commit", "-q", "-m", message)
	}

	run("init", "-q", "-b", "main")
	write("README.md", "base\n")
	commit("base")
	base := run("rev-parse", "HEAD")[:40]

	write("docs/backlog/ready/PG-07.md", "**Verdict:** waiting on the owner\n")
	commit("docs: the record\n\nTask: PG-07")
	write("tools/x.go", "package x\n")
	commit("feat: code before the answer\n\nTask: PG-07")
	write("docs/backlog/ready/PG-07.md", "**Verdict:** ready\n")
	commit("docs: the record is ready\n\nTask: PG-07")

	facts, err := readHistory(dir, base, "HEAD", time.Time{})
	if err != nil {
		t.Fatalf("reading the history: %v", err)
	}
	if len(facts.tasks) != 1 || facts.tasks[0] != "PG-07" {
		t.Errorf("tasks %v, want [PG-07]", facts.tasks)
	}
	if facts.beforeCode == nil {
		t.Fatal("the code commit was not found")
	}
	before, err := facts.beforeCode("docs/backlog/ready/PG-07.md")
	if err != nil || verdictOf(clean(string(before))) != "waiting on the owner" {
		t.Errorf("the record before the code reads %q (%v), want the waiting one", before, err)
	}
}
