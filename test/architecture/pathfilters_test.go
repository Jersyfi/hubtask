// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"context"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/tools/cilocal"
)

// The pipeline decides what to run from `dorny/paths-filter`, and a path no filter names runs
// nothing at all. That is not a red build: `ci-required` counts a skipped job as a pass, so the
// pull request is green and the gates that would have had something to say never reported.
//
// The trap: a `go` filter of `**/*.go` and a list of manifests leaves out every non-Go file a Go
// test reads - the golden archives under test/backup, the adapters' testdata, the load guard's
// baseline, the Go SDK's templates. A golden archive changed on its own then runs nothing, and the
// test whose whole purpose is to notice a changed archive format is the test that does not run.
//
// So this asks the question the other way round: every tracked file must be claimed by some
// filter, or be named below as one that deliberately triggers nothing. Two entries are named
// today. The list is short on purpose - it is the place where "nothing needs to check this" has
// to be said out loud rather than happen by omission.
func TestEveryTrackedPathIsClaimedByAFilter(t *testing.T) {
	filters := loadPathFilters(t)
	if len(filters) < 5 {
		t.Fatalf("only %d filters parsed out of ci.yml - the block has moved, not shrunk", len(filters))
	}

	// Deliberately unfiltered: read by an editor and by git, by no job.
	unfiltered := map[string]string{
		".editorconfig": "editor configuration; no job reads it",
		".gitignore":    "git's own; no job reads it",
	}

	var unclaimed []string
	for _, file := range trackedFiles(t) {
		if _, ok := unfiltered[file]; ok {
			continue
		}
		if !claimed(filters, file) {
			unclaimed = append(unclaimed, file)
		}
	}

	// Grouped by directory: a new tree nobody named produces one line per file otherwise, and the
	// answer is the same for all of them.
	if len(unclaimed) > 0 {
		for _, group := range groupByDirectory(unclaimed) {
			t.Errorf("%s is claimed by no filter in .github/workflows/ci.yml - a change confined to it "+
				"runs no job and reports green. Name the tree in the filter whose jobs should see it, "+
				"or add it to the unfiltered list in this test with the reason", group)
		}
	}
}

// And the other direction: a filter that names a path nothing has is a filter that has stopped
// doing what its author meant. It is how a renamed directory becomes an unwatched one.
func TestEveryFilterNamesSomethingThatExists(t *testing.T) {
	tracked := trackedFiles(t)

	for name, patterns := range loadPathFilters(t) {
		for _, pattern := range patterns {
			matches := false
			for _, file := range tracked {
				if cilocal.Matches(pattern, file) {
					matches = true
					break
				}
			}
			if !matches {
				t.Errorf("the filter %q names %q and nothing in the repository matches it - "+
					"a pattern that matches nothing silently stops gating what it was written for",
					name, pattern)
			}
		}
	}
}

// loadPathFilters reads the filters through the same code `make verify-pr` selects its gates with
// (tools/cilocal, ADR-0079), so that a filter this test approves is the filter both sides use.
func loadPathFilters(t *testing.T) map[string][]string {
	t.Helper()

	raw, err := os.ReadFile("../../" + cilocal.WorkflowPath)
	if err != nil {
		t.Fatalf("ci.yml is not readable: %v", err)
	}
	filters, err := cilocal.LoadFilters(raw)
	if err != nil {
		t.Fatal(err)
	}
	return filters
}

func trackedFiles(t *testing.T) []string {
	t.Helper()

	// With a deadline, like every other call here (rule 7): a `git` that hangs on a lock would
	// otherwise hang the gate rather than fail it.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "git", "-C", "../..", "ls-files").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	var files []string
	for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if file != "" {
			files = append(files, file)
		}
	}
	if len(files) == 0 {
		t.Fatal("git ls-files returned nothing")
	}
	return files
}

func claimed(filters map[string][]string, file string) bool {
	for _, patterns := range filters {
		for _, pattern := range patterns {
			if cilocal.Matches(pattern, file) {
				return true
			}
		}
	}
	return false
}

// groupByDirectory turns a list of unclaimed files into the shortest set of statements about
// them: one per directory, and the file itself when it is at the root.
func groupByDirectory(files []string) []string {
	seen := map[string]bool{}
	var groups []string
	for _, file := range files {
		key := file
		if directory := path.Dir(file); directory != "." {
			key = directory + "/"
		}
		if !seen[key] {
			seen[key] = true
			groups = append(groups, key)
		}
	}
	return groups
}
