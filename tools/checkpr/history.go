// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"
)

// What the branch's history says, for the rules that need it (readiness.go). Read with git, so the CI
// job checks out the whole history (`fetch-depth: 0`) and names the pull request's base.

var trailer = regexp.MustCompile(`(?m)^Task:\s*([A-Z][A-Z0-9]*-\d+[a-z]?)\s*$`)

// gitTimeout bounds every git call: a gate that hangs is a gate somebody turns off.
const gitTimeout = 30 * time.Second

// readHistory reads the facts of base..head in the repository at root. The branch's own commits are
// its first-parent, non-merge commits: a merge of main brings main's commits along, and those are
// not this branch's to answer for.
func readHistory(root, base, head string, opened time.Time) (branchFacts, error) {
	facts := branchFacts{known: true, opened: opened}

	own, err := git(root, "log", "--first-parent", "--no-merges", "--reverse", "--format=%H", base+".."+head)
	if err != nil {
		return branchFacts{}, err
	}
	commits := strings.Fields(own)

	seen := map[string]bool{}
	for _, commit := range commits {
		message, err := git(root, "log", "-1", "--format=%B", commit)
		if err != nil {
			return branchFacts{}, err
		}
		for _, m := range trailer.FindAllStringSubmatch(message, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				facts.tasks = append(facts.tasks, m[1])
			}
		}
	}

	changed, err := git(root, "diff", "--name-status", "--no-renames", base+"..."+head)
	if err != nil {
		return branchFacts{}, err
	}
	for _, line := range strings.Split(strings.TrimSpace(changed), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		facts.changed = append(facts.changed, fields[1])
		if fields[0] != "A" {
			facts.altered = append(facts.altered, change{status: fields[0], path: fields[1]})
		}
	}

	if facts.ucText, err = useCaseTextChanges(root, base, head, changed); err != nil {
		return branchFacts{}, err
	}
	if facts.reused, err = reusedUseCaseIDs(root, base, head, changed); err != nil {
		return branchFacts{}, err
	}
	if facts.adrEdited, err = settledADRChanges(root, base, head, changed); err != nil {
		return branchFacts{}, err
	}
	if facts.sectionsLost, err = renumberedSections(root, base, head, changed); err != nil {
		return branchFacts{}, err
	}

	for _, commit := range commits {
		paths, err := git(root, "diff-tree", "--no-commit-id", "--name-only", "-r", commit)
		if err != nil {
			return branchFacts{}, err
		}
		if touchesCode(strings.Fields(paths)) {
			first := commit
			facts.beforeCode = func(path string) ([]byte, error) {
				out, err := git(root, "show", first+"^:"+path)
				return []byte(out), err
			}
			break
		}
	}
	return facts, nil
}

// ucFile is a use case file: docs/usecases/<context>/UC-XX-nn-title.md.
var ucFile = regexp.MustCompile(`^docs/usecases/[a-z]+/(UC-[A-Z]{2,3}-\d{2,3})-[a-z0-9-]+\.md$`)

// ucOwnerSections are the parts of a use case that say what has to hold for a person; only the
// owner's decision changes them, and a correction is named as one (usecasetext.go).
var ucOwnerSections = []string{"Goal", "How to check", "Where it ends"}

// useCaseTextChanges names the use cases whose Goal, How to check or Where it ends differ between
// the branch's merge base and its head - a new use case among them. Whitespace alone is not a
// change: a reflowed paragraph says the same thing.
func useCaseTextChanges(root, base, head, nameStatus string) ([]ucTextChange, error) {
	var out []ucTextChange
	mergeBase := ""
	for _, line := range strings.Split(strings.TrimSpace(nameStatus), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || (fields[0] != "A" && fields[0] != "M") {
			continue
		}
		m := ucFile.FindStringSubmatch(fields[1])
		if m == nil {
			continue
		}
		if mergeBase == "" {
			found, err := git(root, "merge-base", base, head)
			if err != nil {
				return nil, err
			}
			mergeBase = strings.TrimSpace(found)
		}
		before := ""
		if fields[0] == "M" {
			text, err := git(root, "show", mergeBase+":"+fields[1])
			if err != nil {
				return nil, err
			}
			before = text
		}
		after, err := git(root, "show", head+":"+fields[1])
		if err != nil {
			return nil, err
		}
		for _, title := range ucOwnerSections {
			if normalised(sectionText(before, title)) != normalised(sectionText(after, title)) {
				out = append(out, ucTextChange{id: m[1], path: fields[1], added: fields[0] == "A"})
				break
			}
		}
	}
	return out, nil
}

func normalised(text string) string { return strings.Join(strings.Fields(text), " ") }

// reusedUseCaseIDs names the use cases the branch adds under an ID that an earlier file carried -
// one deleted before deleting was refused, or one renamed away. Every file that ever existed in the
// base's history was added by some commit, so the paths the history touched are all of them. A
// file re-added at its own path is that use case coming back, not a new one under an old ID.
func reusedUseCaseIDs(root, base, head, nameStatus string) ([]reusedID, error) {
	var added []reusedID
	for _, line := range strings.Split(strings.TrimSpace(nameStatus), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "A" {
			continue
		}
		if m := ucFile.FindStringSubmatch(fields[1]); m != nil {
			added = append(added, reusedID{id: m[1], path: fields[1]})
		}
	}
	if len(added) == 0 {
		return nil, nil
	}
	mergeBase, err := git(root, "merge-base", base, head)
	if err != nil {
		return nil, err
	}
	history, err := git(root, "log", "--format=", "--name-only", "--no-renames", strings.TrimSpace(mergeBase), "--", "docs/usecases/")
	if err != nil {
		return nil, err
	}
	earlier := map[string][]string{}
	for _, path := range strings.Fields(history) {
		if m := ucFile.FindStringSubmatch(path); m != nil && !slices.Contains(earlier[m[1]], path) {
			earlier[m[1]] = append(earlier[m[1]], path)
		}
	}
	var out []reusedID
	for _, a := range added {
		for _, path := range earlier[a.id] {
			if path != a.path {
				out = append(out, reusedID{id: a.id, path: a.path, earlier: path})
				break
			}
		}
	}
	return out, nil
}

// touchesCode is whether a commit changes anything outside docs/ - the record itself, the backlog
// and the use cases are written before the code, and are not code.
func touchesCode(paths []string) bool {
	for _, path := range paths {
		if !strings.HasPrefix(path, "docs/") {
			return true
		}
	}
	return false
}

func git(root string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: fixed command, arguments from this program and the workflow's own refs
	cmd.Dir = root
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}
