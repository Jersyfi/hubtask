// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
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
