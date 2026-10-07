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

// scratchRepo is a git repository in a temporary directory, for the rules that read history.
type scratchRepo struct {
	t   *testing.T
	dir string
}

func newScratchRepo(t *testing.T) *scratchRepo {
	t.Helper()
	r := &scratchRepo{t: t, dir: t.TempDir()}
	r.run("init", "-q", "-b", "main")
	return r
}

func (r *scratchRepo) run(args ...string) {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir
	cmd.Env = scratchGitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r *scratchRepo) write(path, content string) {
	r.t.Helper()
	full := filepath.Join(r.dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *scratchRepo) remove(path string) {
	r.t.Helper()
	if err := os.Remove(filepath.Join(r.dir, filepath.FromSlash(path))); err != nil {
		r.t.Fatal(err)
	}
}

func (r *scratchRepo) commit(message string) {
	r.t.Helper()
	r.run("add", "-A")
	r.run("commit", "-q", "-m", message)
}

// An ID a deleted or renamed file carried is not taken again by a new file; a file that comes back
// at its own path is the same use case.
func TestReadHistoryReusedUseCaseID(t *testing.T) {
	r := newScratchRepo(t)
	r.write("docs/usecases/work/UC-WRK-04-the-old-one.md", useCaseFile)
	r.write("docs/usecases/work/UC-WRK-05-kept.md", useCaseFile)
	r.write("docs/usecases/work/UC-WRK-06-gone.md", useCaseFile)
	r.commit("docs: three use cases")
	r.remove("docs/usecases/work/UC-WRK-04-the-old-one.md")
	r.remove("docs/usecases/work/UC-WRK-06-gone.md")
	r.commit("docs: two deleted, before deleting was refused")

	r.run("checkout", "-q", "-b", "work")
	r.write("docs/usecases/work/UC-WRK-04-something-else.md", useCaseFile)
	r.write("docs/usecases/work/UC-WRK-06-gone.md", useCaseFile)
	r.write("docs/usecases/work/UC-WRK-07-new.md", useCaseFile)
	r.commit("docs: new use cases")

	facts, err := readHistory(r.dir, "main", "work", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.reused) != 1 || facts.reused[0].id != "UC-WRK-04" || facts.reused[0].earlier != "docs/usecases/work/UC-WRK-04-the-old-one.md" {
		t.Fatalf("want UC-WRK-04 reused and nothing else, got %+v", facts.reused)
	}
	problems := strings.Join(historyProblems("", facts, files(nil)), "\n")
	if !strings.Contains(problems, "takes the ID UC-WRK-04, which docs/usecases/work/UC-WRK-04-the-old-one.md carried") {
		t.Fatalf("the reused ID was not refused:\n%s", problems)
	}
}
