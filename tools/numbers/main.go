// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command numbers holds the numbered files - migrations and ADRs - to one file per number
// (`make gate-numbers`, ADR-0083).
//
// A pull request merges without catching up with `main`, so two pull requests that each add
// migration 0140 are both green on their own and collide only on `main`, where goose refuses the
// duplicate version and every container gate turns red. Two checks close that:
//
//   - the tree: no two migrations and no two ADRs carry one number. On `main` this is the backstop;
//     on a pull request it reads the merge with the base, so a number `main` took meanwhile counts.
//   - the open pull requests: no number this branch adds is added by another open pull request,
//     drafts included, under another file name. Of two pull requests, the run that looks second sees
//     the first one's file, because each looks only after its own push; so at least one of them is
//     always told. The same file in both is one migration carried by a stacked branch, not two.
//
// Usage: numbers [-base origin/main] [-pr <n>] [-open=false]
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const callTimeout = 60 * time.Second

// kind is a family of numbered files: where they live and how the number is read off the name.
type kind struct {
	name string
	dir  string
	file *regexp.Regexp
}

var kinds = []kind{
	{name: "migration", dir: "db/migrations", file: regexp.MustCompile(`^(\d+)_[^/]+\.sql$`)},
	{name: "ADR", dir: "docs/adr", file: regexp.MustCompile(`^ADR-(\d+)-[^/]+\.md$`)},
}

// numbered is one file that carries a number.
type numbered struct {
	kind   string
	number int
	path   string
}

// parse reads the number off a repository path, if the path is a numbered file.
func parse(path string) (numbered, bool) {
	for _, k := range kinds {
		if filepath.ToSlash(filepath.Dir(path)) != k.dir {
			continue
		}
		m := k.file.FindStringSubmatch(filepath.Base(path))
		if m == nil {
			return numbered{}, false
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return numbered{}, false
		}
		return numbered{kind: k.name, number: n, path: filepath.ToSlash(path)}, true
	}
	return numbered{}, false
}

// treeProblems names every number two files of one kind share.
func treeProblems(paths []string) []string {
	byNumber := map[string][]string{}
	for _, p := range paths {
		if f, ok := parse(p); ok {
			key := fmt.Sprintf("%s %04d", f.kind, f.number)
			byNumber[key] = append(byNumber[key], f.path)
		}
	}
	var problems []string
	for key, files := range byNumber {
		if len(files) > 1 {
			sort.Strings(files)
			problems = append(problems, fmt.Sprintf("%s is carried by %d files: %s - one of them takes the next free number", key, len(files), strings.Join(files, ", ")))
		}
	}
	sort.Strings(problems)
	return problems
}

// collisionProblems names every number this branch adds that another open pull request adds under
// another file name. others maps a pull request's number to the paths it adds.
func collisionProblems(added []string, others map[int][]string) []string {
	var problems []string
	for _, a := range added {
		mine, ok := parse(a)
		if !ok {
			continue
		}
		var holders []string
		for pr, paths := range others {
			for _, p := range paths {
				theirs, ok := parse(p)
				if ok && theirs.kind == mine.kind && theirs.number == mine.number && theirs.path != mine.path {
					holders = append(holders, fmt.Sprintf("#%d (%s)", pr, theirs.path))
				}
			}
		}
		if len(holders) > 0 {
			sort.Strings(holders)
			problems = append(problems, fmt.Sprintf("%s adds %s %04d, which the open pull request %s adds too - "+
				"renumber above the highest number on main and on every open pull request (known-traps.md, migration and ADR numbers)",
				mine.path, mine.kind, mine.number, strings.Join(holders, ", ")))
		}
	}
	sort.Strings(problems)
	return problems
}

// openPullRequests lists the paths every other open pull request adds under the numbered
// directories. It is a variable so that the tests can stand in for GitHub.
var openPullRequests = func(ctx context.Context, self int) (map[int][]string, error) {
	out, err := gh(ctx, "api", "repos/{owner}/{repo}/pulls?state=open&per_page=100", "--paginate", "--jq", ".[].number")
	if err != nil {
		return nil, err
	}
	others := map[int][]string{}
	for _, field := range strings.Fields(out) {
		n, err := strconv.Atoi(field)
		if err != nil {
			return nil, fmt.Errorf("an open pull request's number %q does not parse", field)
		}
		if n == self {
			continue
		}
		files, err := gh(ctx, "api", fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/files?per_page=100", n), "--paginate",
			"--jq", `.[] | select(.status == "added" or .status == "renamed" or .status == "copied") | .filename`)
		if err != nil {
			return nil, err
		}
		for _, p := range strings.Fields(files) {
			if _, ok := parse(p); ok {
				others[n] = append(others[n], p)
			}
		}
	}
	return others, nil
}

func main() {
	os.Exit(run())
}

func run() int {
	base := flag.String("base", "origin/main", "what the branch is compared with, to find the files it adds")
	self := flag.Int("pr", 0, "this branch's pull request, left out of the comparison (default: asked of gh; none is fine)")
	open := flag.Bool("open", true, "compare with the other open pull requests (false on main, where only the tree is held)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*callTimeout)
	defer cancel()

	problems, report, err := check(ctx, *base, *self, *open)
	if err != nil {
		fmt.Fprintf(os.Stderr, "numbers: %v\n", err)
		return 1
	}
	for _, line := range report {
		fmt.Println(line)
	}
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintf(os.Stderr, "numbers: %s\n", p)
		}
		return 1
	}
	return 0
}

func check(ctx context.Context, base string, self int, open bool) ([]string, []string, error) {
	var tree []string
	for _, k := range kinds {
		entries, err := os.ReadDir(k.dir)
		if err != nil {
			return nil, nil, fmt.Errorf("reading %s: %w (run from the repository root)", k.dir, err)
		}
		for _, e := range entries {
			tree = append(tree, k.dir+"/"+e.Name())
		}
	}
	problems := treeProblems(tree)
	report := []string{fmt.Sprintf("numbers: %d numbered files in the tree", len(tree))}
	if !open {
		return problems, report, nil
	}

	added, err := addedSince(ctx, base)
	if err != nil {
		return nil, nil, err
	}
	var mine []string
	for _, p := range added {
		if _, ok := parse(p); ok {
			mine = append(mine, p)
		}
	}
	if len(mine) == 0 {
		return problems, append(report, "numbers: this branch adds no migration and no ADR"), nil
	}
	if self == 0 {
		self = ownPullRequest(ctx)
	}
	others, err := openPullRequests(ctx, self)
	if err != nil {
		return nil, nil, fmt.Errorf("listing the open pull requests: %w - the comparison cannot be skipped; try again, or check `gh auth status`", err)
	}
	report = append(report, fmt.Sprintf("numbers: this branch adds %s; compared with %d other open pull requests that add numbered files", strings.Join(mine, ", "), len(others)))
	return append(problems, collisionProblems(mine, others)...), report, nil
}

// addedSince is what the branch adds over where it left base. In CI the checkout is the merge
// commit and base its first parent, so the merge base is base itself.
func addedSince(ctx context.Context, base string) ([]string, error) {
	mergeBase, err := git(ctx, "merge-base", base, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("finding where the branch left %s: %w (git fetch origin, or pass -base)", base, err)
	}
	var added []string
	for _, k := range kinds {
		out, err := git(ctx, "diff", "--name-only", "--diff-filter=AR", mergeBase, "HEAD", "--", k.dir)
		if err != nil {
			return nil, err
		}
		added = append(added, strings.Fields(out)...)
	}
	return added, nil
}

// ownPullRequest is the pull request of the checked-out branch, or 0 when it has none yet.
func ownPullRequest(ctx context.Context) int {
	out, err := gh(ctx, "pr", "view", "--json", "number", "--jq", ".number")
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n
}

func git(ctx context.Context, args ...string) (string, error) {
	return command(ctx, "git", args...)
}

func gh(ctx context.Context, args ...string) (string, error) {
	return command(ctx, "gh", args...)
}

func command(ctx context.Context, name string, args ...string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(callCtx, name, args...) //nolint:gosec // G204: fixed git and gh subcommands
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}
