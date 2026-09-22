// SPDX-License-Identifier: BUSL-1.1
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

	"gopkg.in/yaml.v3"
)

// The pipeline decides what to run from `dorny/paths-filter`, and a path no filter names runs
// nothing at all. That is not a red build: `ci-required` counts a skipped job as a pass, so the
// pull request is green and the gates that would have had something to say never reported.
//
// It had already happened. The `go` filter said `**/*.go` and a list of manifests, so every
// non-Go file a Go test reads was outside it - thirty golden archives under test/backup, the
// adapters' testdata, the load guard's baseline, the Go SDK's templates. A golden archive changed
// on its own ran nothing, and the test whose whole purpose is to notice a changed archive format
// was the test that did not run (#941).
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
				if matchesPattern(pattern, file) {
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

// loadPathFilters reads the `filters:` block the `changes` job hands to dorny/paths-filter.
//
// The block is a YAML document inside a YAML string, so it is extracted by indentation and parsed
// on its own. It carries an anchor and aliases (`*workspace-manifests`), which is exactly why it
// is parsed rather than pattern-matched: a reader of this file has to see what a package's filter
// really contains, aliases resolved.
func loadPathFilters(t *testing.T) map[string][]string {
	t.Helper()

	raw, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("ci.yml is not readable: %v", err)
	}

	lines := strings.Split(string(raw), "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "filters: |" {
			start = i + 1
			break
		}
	}
	if start == -1 {
		t.Fatal("ci.yml has no `filters: |` block - the changes job is what every other job reads")
	}

	const indent = "            " // the block scalar's own indentation inside the workflow
	var block []string
	for _, line := range lines[start:] {
		if strings.TrimSpace(line) == "" {
			block = append(block, "")
			continue
		}
		if !strings.HasPrefix(line, indent) {
			break
		}
		block = append(block, strings.TrimPrefix(line, indent))
	}

	// An alias inserts the anchored *sequence* as one item, so a package's list is a list with a
	// list inside it. dorny/paths-filter flattens that itself - the anchor is the documented way
	// to share a group of paths - so this flattens it the same way rather than refusing the file.
	var parsed map[string][]any
	if err := yaml.Unmarshal([]byte(strings.Join(block, "\n")), &parsed); err != nil {
		t.Fatalf("the filters block does not parse as YAML: %v", err)
	}

	filters := make(map[string][]string, len(parsed))
	for name, items := range parsed {
		filters[name] = flattenPatterns(t, name, items)
	}
	return filters
}

func flattenPatterns(t *testing.T, filter string, items []any) []string {
	t.Helper()

	var patterns []string
	for _, item := range items {
		switch value := item.(type) {
		case string:
			patterns = append(patterns, value)
		case []any:
			patterns = append(patterns, flattenPatterns(t, filter, value)...)
		default:
			t.Fatalf("the filter %q holds a %T, which is neither a path nor a group of them", filter, item)
		}
	}
	return patterns
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
			if matchesPattern(pattern, file) {
				return true
			}
		}
	}
	return false
}

// matchesPattern covers the three shapes the filter list uses: a tree (`core/**`), an extension
// anywhere (`**/*.md`), and an exact path (`go.mod`). Deliberately not a general glob engine -
// a fourth shape should be read here by whoever writes it rather than resolved by a dependency.
func matchesPattern(pattern, file string) bool {
	switch {
	case strings.HasPrefix(pattern, "**/*."):
		return strings.HasSuffix(file, strings.TrimPrefix(pattern, "**/*"))
	case strings.HasSuffix(pattern, "/**"):
		return strings.HasPrefix(file, strings.TrimSuffix(pattern, "**"))
	case strings.Contains(pattern, "*"):
		ok, err := path.Match(pattern, file)
		return err == nil && ok
	default:
		return pattern == file
	}
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
