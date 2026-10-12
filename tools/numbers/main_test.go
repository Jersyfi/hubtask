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
)

func TestParseReadsTheNumberOfMigrationsAndADRsOnly(t *testing.T) {
	cases := []struct {
		path   string
		ok     bool
		kind   string
		number int
	}{
		{"db/migrations/0135_operator_roster.sql", true, "migration", 135},
		{"docs/adr/ADR-0083-a-merge-without-catching-up.md", true, "ADR", 83},
		{"docs/adr/README.md", false, "", 0},
		{"db/migrations/README.md", false, "", 0},
		{"db/queries/0135_not_a_migration.sql", false, "", 0},
		{"db/migrations/sub/0001_nested.sql", false, "", 0},
		{"docs/archive/adr/ADR-0001-old.md", false, "", 0},
	}
	for _, c := range cases {
		got, ok := parse(c.path)
		if ok != c.ok || got.kind != c.kind || got.number != c.number {
			t.Errorf("parse(%q) = %+v, %v; want %s %d, %v", c.path, got, ok, c.kind, c.number, c.ok)
		}
	}
}

func TestTheTreeHoldsOneFilePerNumber(t *testing.T) {
	clean := []string{"db/migrations/0001_a.sql", "db/migrations/0002_b.sql", "docs/adr/ADR-0001-x.md", "docs/adr/ADR-0002-y.md",
		// a migration and an ADR may share a number: they are different families
		"docs/adr/README.md"}
	if p := treeProblems(clean); len(p) != 0 {
		t.Errorf("a clean tree reported %v", p)
	}

	twice := append(clean, "db/migrations/0002_c.sql", "docs/adr/ADR-0001-z.md")
	p := treeProblems(twice)
	if len(p) != 2 {
		t.Fatalf("two duplicated numbers reported as %d problems: %v", len(p), p)
	}
	if !strings.Contains(p[0], "ADR 0001") || !strings.Contains(p[0], "ADR-0001-x.md") || !strings.Contains(p[0], "ADR-0001-z.md") {
		t.Errorf("the ADR duplicate is not named with both files: %s", p[0])
	}
	if !strings.Contains(p[1], "migration 0002") || !strings.Contains(p[1], "0002_b.sql") || !strings.Contains(p[1], "0002_c.sql") {
		t.Errorf("the migration duplicate is not named with both files: %s", p[1])
	}
}

func TestANumberAnotherOpenPullRequestAddsIsAProblem(t *testing.T) {
	added := []string{"db/migrations/0080_mine.sql", "docs/adr/ADR-0081-mine.md"}
	cases := []struct {
		name   string
		others map[int][]string
		want   []string // substrings, one per expected problem
	}{
		{"nobody else", nil, nil},
		{"other numbers", map[int][]string{1301: {"db/migrations/0081_theirs.sql"}, 1302: {"docs/adr/ADR-0082-theirs.md"}}, nil},
		{"the same migration number", map[int][]string{1301: {"db/migrations/0080_theirs.sql"}}, []string{"migration 0080", "#1301 (db/migrations/0080_theirs.sql)"}},
		{"the same ADR number", map[int][]string{1302: {"docs/adr/ADR-0081-theirs.md"}}, []string{"ADR 0081", "#1302"}},
		// a stacked pull request carries the same file: one migration, not two
		{"the same file", map[int][]string{1303: {"db/migrations/0080_mine.sql"}}, nil},
		// a number shared across families is no collision
		{"an ADR with the migration's number", map[int][]string{1304: {"docs/adr/ADR-0080-theirs.md"}}, nil},
		{"two others", map[int][]string{1305: {"db/migrations/0080_x.sql"}, 1306: {"db/migrations/0080_y.sql"}}, []string{"#1305", "#1306"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := collisionProblems(added, c.others)
			if c.want == nil {
				if len(p) != 0 {
					t.Errorf("reported %v", p)
				}
				return
			}
			if len(p) != 1 {
				t.Fatalf("want one problem, got %v", p)
			}
			for _, w := range c.want {
				if !strings.Contains(p[0], w) {
					t.Errorf("the problem does not name %q: %s", w, p[0])
				}
			}
		})
	}
}

// check end to end against a scratch repository: the tree, what the branch adds over its base, and
// the open pull requests through a stand-in for GitHub.
func TestCheckReadsTheBranchAgainstItsBaseAndTheOpenPullRequests(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		cmd.Env = scratchGitEnv()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(p string) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("-- x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q", "-b", "main")
	write("db/migrations/0001_first.sql")
	write("docs/adr/ADR-0001-first.md")
	run("add", "-A")
	run("commit", "-q", "-m", "base")
	run("switch", "-q", "-c", "branch")
	write("db/migrations/0002_mine.sql")
	run("add", "-A")
	run("commit", "-q", "-m", "mine")

	t.Chdir(dir)
	// check runs git with the process's environment: point it at the scratch repository explicitly,
	// so that a GIT_DIR inherited from a hook can never lead it to the real one.
	t.Setenv("GIT_DIR", filepath.Join(dir, ".git"))
	t.Setenv("GIT_WORK_TREE", dir)

	saved := openPullRequests
	t.Cleanup(func() { openPullRequests = saved })
	var askedSelf int
	stub := func(others map[int][]string, err error) {
		openPullRequests = func(_ context.Context, self int) (map[int][]string, error) {
			askedSelf = self
			return others, err
		}
	}

	stub(map[int][]string{7: {"db/migrations/0003_theirs.sql"}}, nil)
	problems, _, err := check(context.Background(), "main", 5, true)
	if err != nil || len(problems) != 0 {
		t.Fatalf("no collision: problems %v, err %v", problems, err)
	}
	if askedSelf != 5 {
		t.Errorf("the branch's own pull request was not left out: asked with %d", askedSelf)
	}

	stub(map[int][]string{7: {"db/migrations/0002_theirs.sql"}}, nil)
	problems, _, err = check(context.Background(), "main", 5, true)
	if err != nil || len(problems) != 1 || !strings.Contains(problems[0], "#7") {
		t.Fatalf("a collision with #7: problems %v, err %v", problems, err)
	}

	// GitHub not answering is an error, never a pass.
	stub(nil, errors.New("HTTP 502"))
	if _, _, err = check(context.Background(), "main", 5, true); err == nil {
		t.Fatal("a failed listing passed")
	}

	// On main only the tree counts, and GitHub is not asked.
	write("db/migrations/0001_again.sql")
	stub(nil, errors.New("must not be asked"))
	problems, _, err = check(context.Background(), "main", 0, false)
	if err != nil || len(problems) != 1 || !strings.Contains(problems[0], "migration 0001") {
		t.Fatalf("the tree's duplicate: problems %v, err %v", problems, err)
	}
}

// scratchGitEnv is the environment for git in a test's scratch repository: every GIT_* variable of
// the caller is dropped, or a scratch `git init` under a hook would re-initialise the real
// repository (known-traps.md, "A test's scratch repository inherits GIT_DIR").
func scratchGitEnv() []string {
	env := make([]string, 0, len(os.Environ())+5)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_NOSYSTEM=1")
}
