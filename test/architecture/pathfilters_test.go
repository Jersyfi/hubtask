// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"context"
	"os"
	"os/exec"
	"path"
	"slices"
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
// filter, or be named below as one that deliberately triggers nothing. The list is short on
// purpose - it is the place where "nothing needs to check this" has to be said out loud rather
// than happen by omission.
func TestEveryTrackedPathIsClaimedByAFilter(t *testing.T) {
	filters := loadPathFilters(t)
	if len(filters) < 5 {
		t.Fatalf("only %d filters parsed out of ci.yml - the block has moved, not shrunk", len(filters))
	}

	// Deliberately unfiltered: read by an editor and by git, by no job.
	unfiltered := map[string]string{
		".editorconfig": "editor configuration; no job reads it",
		".gitignore":    "git's own; no job reads it",
		// An outside reviewer's own configuration (ADR-0106); it comments and decides nothing.
		".coderabbit.yaml": "read by the outside reviewer's service; no job reads it",
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

// Being claimed is not enough for a file these tests read: a skill is Markdown, so the `docs`
// filter claims it, and the documentation job runs none of these tests. A skill, its link or the
// reviewer's wrapper changed alone has to set the `go` filter, which is what runs this package.
// The root AGENTS.md is not read here: the documentation gate, which runs on every change, holds
// its list of skills to the folders.
func TestWhatTheArchitectureTestsReadRunsThem(t *testing.T) {
	filters := loadPathFilters(t)
	read := []string{".claude/settings.json", ".agents/skills/", ".claude/skills/", ".claude/agents/"}

	found := map[string]bool{}
	for _, file := range trackedFiles(t) {
		for _, prefix := range read {
			if !strings.HasPrefix(file, prefix) {
				continue
			}
			found[prefix] = true
			if !cilocal.Touched(filters, []string{file})["go"] {
				t.Errorf("%s sets no `go` filter in .github/workflows/ci.yml - test/architecture reads it, "+
					"and a change confined to it would not run the test", file)
			}
		}
	}
	for _, prefix := range read {
		if !found[prefix] {
			// A list that names nothing passes for the wrong reason.
			t.Errorf("no tracked file under %s - the path has moved, and this test holds nothing for it", prefix)
		}
	}
}

// The browser job is the one that judges the walks' answers (`make gate-walks`), and it is behind
// the web app's filters, not `go`: walking every suite to test a Go change would cost twenty
// minutes a pull request. So what the judge is made of sets a filter of its own, and the job's
// condition names it - else a change to the judge alone would be merged unjudged.
func TestTheWalksJudgeRunsTheJobThatJudges(t *testing.T) {
	filters := loadPathFilters(t)
	judge := map[string]string{
		"test/contract/walks.go":                    "walk_judge",
		"test/contract/walks_test.go":               "walk_judge",
		"test/contract/schema.go":                   "walk_judge",
		"test/contract/schema_test.go":              "walk_judge",
		"test/contract/testdata/walks-baseline.txt": "walk_judge",
		"api/openapi.yaml":                          "openapi",
	}
	tracked := trackedFiles(t)
	condition := jobCondition(t, "engines")
	for file, filter := range judge {
		if !slices.Contains(tracked, file) {
			t.Errorf("%s is not tracked - the judge has moved, and this test holds nothing for it", file)
		}
		if !cilocal.Touched(filters, []string{file})[filter] {
			t.Errorf("%s does not set the `%s` filter in %s", file, filter, cilocal.WorkflowPath)
		}
		if !strings.Contains(condition, "needs.changes.outputs."+filter+" == 'true'") {
			t.Errorf("the `engines` job does not run on the `%s` filter, so a change to %s alone is not judged: %s", filter, file, condition)
		}
	}
	// A Go file beside the judge must not set it: the filter is narrow so that the job stays cheap.
	if cilocal.Touched(filters, []string{"test/contract/health_test.go"})["walk_judge"] {
		t.Errorf("test/contract/health_test.go sets `walk_judge` - the filter names more than the judge")
	}
}

// jobCondition reads the `if:` of a job of ci.yml.
func jobCondition(t *testing.T, job string) string {
	t.Helper()
	raw, err := os.ReadFile("../../" + cilocal.WorkflowPath)
	if err != nil {
		t.Fatalf("ci.yml is not readable: %v", err)
	}
	_, after, found := strings.Cut(string(raw), "\n  "+job+":\n")
	if !found {
		t.Fatalf("%s has no job %q", cilocal.WorkflowPath, job)
	}
	for _, line := range strings.Split(after, "\n") {
		if condition, ok := strings.CutPrefix(line, "    if: "); ok {
			return condition
		}
		// The next job: this one has no condition.
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") && strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			break
		}
	}
	t.Fatalf("the job %q of %s has no `if:`", job, cilocal.WorkflowPath)
	return ""
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

// loadPathFilters reads the filters through tools/cilocal, which parses the block the way the
// `changes` job hands it to paths-filter, aliases resolved.
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
