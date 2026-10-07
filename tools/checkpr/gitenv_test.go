// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scratchGitEnv is the environment for git in a test's scratch repository. Every GIT_* variable of
// the caller is dropped: under a git hook or `git rebase --exec`, GIT_DIR names the real repository,
// and a scratch `git init` would re-initialise it - which once wrote core.bare = true into the
// checkout's config and stopped every worktree (known-traps.md, "Tests, gates and tooling").
func scratchGitEnv() []string {
	env := make([]string, 0, len(os.Environ())+4)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_NOSYSTEM=1")
}

// A scratch repository leaves the repository a caller's GIT_DIR names untouched.
func TestAScratchRepositoryIgnoresTheCallersGitDir(t *testing.T) {
	victim := newScratchRepo(t)
	t.Setenv("GIT_DIR", filepath.Join(victim.dir, ".git"))
	t.Setenv("GIT_WORK_TREE", victim.dir)

	r := newScratchRepo(t)
	r.write("a.txt", "a\n")
	r.run("add", "-A")
	r.run("commit", "-q", "-m", "a")

	config, err := os.ReadFile(filepath.Join(victim.dir, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(config), "bare = true") {
		t.Fatalf("the scratch repository re-initialised the caller's repository:\n%s", config)
	}
	if _, err := os.Stat(filepath.Join(r.dir, ".git")); err != nil {
		t.Fatalf("the scratch repository was not created in its own directory: %v", err)
	}
}
