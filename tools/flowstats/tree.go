// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

func git(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // G204: git with arguments this tool builds
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// The files the tree's measures read. The first is the root instruction file, whose length is
// measured; its name is put together so that it reads as a path and not as a citation.
const (
	agentsPath   = "AGENTS" + ".md"
	boundsPath   = "apps/webapp/e2e/bounds.json"
	baselinePath = "test/contract/testdata/walks-baseline.txt"
	uncitedPath  = "tools/checkdocs/usecases.go"
	useCasesDir  = "docs/usecases/"
)

// commentDirs are the directories whose long comment blocks are counted.
var commentDirs = []string{"core/", "presentation/", "infrastructure/", "cmd/"}

// snapshot is the files of one commit that the measures read, by path.
type snapshot map[string][]byte

// snapshotAt reads them out of the commit, so both windows are read the same way and a change
// that is not committed counts in neither.
func snapshotAt(rev string) (snapshot, error) {
	if _, err := git("rev-parse", "--verify", "--quiet", rev+"^{commit}"); err != nil {
		return nil, err
	}
	s := snapshot{}
	paths := append([]string{agentsPath, boundsPath, baselinePath, uncitedPath, useCasesDir}, commentDirs...)
	for _, path := range paths {
		out, err := git("archive", "--format=tar", rev, "--", strings.TrimSuffix(path, "/"))
		if err != nil {
			continue // the commit does not have the path; its measure says so
		}
		files := tar.NewReader(bytes.NewReader(out))
		for {
			header, err := files.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			if header.Typeflag != tar.TypeReg {
				continue
			}
			data, err := io.ReadAll(files)
			if err != nil {
				return nil, err
			}
			s[header.Name] = data
		}
	}
	return s, nil
}

// commitAt is the newest commit of the checkout's history before t, or "" where it has none.
func commitAt(t time.Time) (string, error) {
	out, err := git("rev-list", "-1", "--before="+t.Format(time.RFC3339), "HEAD")
	return strings.TrimSpace(string(out)), err
}

// treePart reads the tree at the checkout's commit, at the commit the window started with and at
// the one the window before started with.
func treePart(found map[string]measure, w window) error {
	head, err := git("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	revs := []string{strings.TrimSpace(string(head))}
	for _, t := range []time.Time{w.from, w.before().from} {
		rev, err := commitAt(t)
		if err != nil {
			return err
		}
		revs = append(revs, rev)
	}
	values := make([]map[string]value, 2)
	for i := range values {
		if revs[i] == "" {
			values[i] = map[string]value{}
			continue
		}
		s, err := snapshotAt(revs[i])
		if err != nil {
			return err
		}
		values[i] = treeValues(s)
		values[i]["docs_lines"] = docsLines(revs[i+1], revs[i])
	}
	for id, now := range values[0] {
		before, ok := values[1][id]
		if !ok {
			before = unmeasured("the history has no commit before the window")
		}
		found[id] = measure{now: now, before: before}
	}
	for _, id := range []string{"unreachable_go_functions", "unused_translation_keys"} {
		found[id] = measure{now: untooled[id], before: untooled[id]}
	}
	return nil
}

// untooled are the two counts of engineering-guidelines.md §3 whose tools the repository does not
// hold yet.
var untooled = map[string]value{
	"unreachable_go_functions": unmeasured("`deadcode` is not among the tools `make tools` pins"),
	"unused_translation_keys":  unmeasured("no tool in the repository counts them, and Knip is not a dependency of the workspace"),
}

var (
	uncitedBound = regexp.MustCompile(`(?m)^const uncitedBuiltAtMost = (\d+)$`)
	stateLine    = regexp.MustCompile(`(?m)^state:\s*(\w+)\s*$`)
)

// treeValues are the measures one commit's files give.
func treeValues(s snapshot) map[string]value {
	out := map[string]value{}
	absent := func(path string) value { return unmeasured("the commit has no `%s`", path) }

	out["agents_lines"] = absent(agentsPath)
	if data, ok := s[agentsPath]; ok {
		out["agents_lines"] = count(bytes.Count(data, []byte("\n")))
	}

	for _, id := range []string{"fixed_waits", "timer_waits", "suites_without_use_case"} {
		out[id] = absent(boundsPath)
	}
	if data, ok := s[boundsPath]; ok {
		var bounds struct {
			FixedWaits  map[string]int `json:"fixed_waits"`
			TimerWaits  map[string]int `json:"timer_waits"`
			NoCheckCite []string       `json:"no_check_cited"`
		}
		if err := json.Unmarshal(data, &bounds); err != nil {
			for _, id := range []string{"fixed_waits", "timer_waits", "suites_without_use_case"} {
				out[id] = unmeasured("`%s` does not parse", boundsPath)
			}
		} else {
			out["fixed_waits"] = count(sum(bounds.FixedWaits))
			out["timer_waits"] = count(sum(bounds.TimerWaits))
			out["suites_without_use_case"] = count(len(bounds.NoCheckCite))
		}
	}

	out["answers_outside_contract"] = absent(baselinePath)
	if data, ok := s[baselinePath]; ok {
		n := 0
		for _, row := range strings.Split(string(data), "\n") {
			if row = strings.TrimSpace(row); row != "" && !strings.HasPrefix(row, "#") {
				n++
			}
		}
		out["answers_outside_contract"] = count(n)
	}

	out["uncited_checks_built"] = absent(uncitedPath)
	if data, ok := s[uncitedPath]; ok {
		out["uncited_checks_built"] = unmeasured("`%s` holds no `uncitedBuiltAtMost`", uncitedPath)
		if m := uncitedBound.FindSubmatch(data); m != nil {
			n, _ := strconv.Atoi(string(m[1]))
			out["uncited_checks_built"] = count(n)
		}
	}

	blocks, goFiles := 0, 0
	states := map[string]int{}
	for path, data := range s {
		switch {
		case strings.HasPrefix(path, useCasesDir) && strings.HasSuffix(path, ".md"):
			if front, ok := frontMatter(data); ok {
				if m := stateLine.FindSubmatch(front); m != nil {
					states[string(m[1])]++
				}
			}
		case strings.HasSuffix(path, ".go") && path != uncitedPath:
			goFiles++
			blocks += longComments(data, 20)
		}
	}
	out["comment_blocks"] = unmeasured("the commit has no Go file under %s", strings.Join(commentDirs, ", "))
	if goFiles > 0 {
		out["comment_blocks"] = count(blocks)
	}
	out["use_cases_by_state"] = unmeasured("the commit has no use case with a `state:`")
	if len(states) > 0 {
		out["use_cases_by_state"] = words(tally(states))
	}
	return out
}

func sum(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

// frontMatter is the block between a document's first two `---` lines.
func frontMatter(data []byte) ([]byte, bool) {
	rest, ok := bytes.CutPrefix(data, []byte("---\n"))
	if !ok {
		return nil, false
	}
	front, _, ok := bytes.Cut(rest, []byte("\n---"))
	return front, ok
}

// longComments counts the runs of at least atLeast lines that each start with `//`.
func longComments(data []byte, atLeast int) int {
	blocks, run := 0, 0
	for _, row := range bytes.Split(data, []byte("\n")) {
		if bytes.HasPrefix(bytes.TrimSpace(row), []byte("//")) {
			run++
			continue
		}
		if run >= atLeast {
			blocks++
		}
		run = 0
	}
	if run >= atLeast {
		blocks++
	}
	return blocks
}

// tally is "verified 145, built 115": the larger count first, then by name.
func tally(counts map[string]int) string {
	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	for i, name := range names {
		names[i] = fmt.Sprintf("%s %d", name, counts[name])
	}
	return strings.Join(names, ", ")
}

// docsLines is what changed under docs/ outside archive/ between two commits.
func docsLines(from, to string) value {
	if from == "" {
		return unmeasured("the history has no commit before the window")
	}
	out, err := git("diff", "--numstat", from, to, "--", "docs", ":(exclude)docs/archive")
	if err != nil {
		return unmeasured("%v", err)
	}
	added, removed := 0, 0
	for _, row := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(row)
		if len(fields) < 3 {
			continue
		}
		a, errA := strconv.Atoi(fields[0])
		r, errR := strconv.Atoi(fields[1])
		if errA != nil || errR != nil {
			continue // a binary file has no lines
		}
		added += a
		removed += r
	}
	return words(fmt.Sprintf("+%d / −%d", added, removed))
}
