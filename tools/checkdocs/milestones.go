// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A released milestone promises a fixed set of use case checks - its `**Delivers:**` line - and
// lets its tasks move freely inside that promise (docs/backlog/README.md, "While a milestone
// runs"). Two things hold that together:
//
//   - every task carries a `**Use cases:**` line, and every check it names is one the milestone
//     delivers: a task outside Delivers is a milestone growing past what the owner released;
//   - a milestone that says `**Closed:**` has no Delivers check that a use case still lists as unmet
//     in its *Today*, and none of its use cases is still `specified`: closing is "every Delivers
//     check is met", not "the task list is empty".
//
// A use case named without check numbers means all of its checks, in Delivers and in a task alike.

var (
	deliversLine  = regexp.MustCompile(`(?m)^\*\*Delivers:\*\*(.*)$`)
	closedLine    = regexp.MustCompile(`(?m)^\*\*Closed:\*\*`)
	useCasesLine  = regexp.MustCompile(`(?m)^\*\*Use cases:\*\*(.*)$`)
	checkedUC     = regexp.MustCompile(`(UC-[A-Z]{2,3}-\d{2,3})(?:\s*\(([^)]*)\))?`)
	checkRange    = regexp.MustCompile(`^(\d+)\s*(?:[–-]\s*(\d+))?$`)
	everyDelivers = regexp.MustCompile("(?i)every check in `?Delivers`?")
)

// deliversExempt are tasks built before milestones had a Delivers line, which carry no use case
// check at all. They stay on record as they were built; the reason says why each is not a check.
var deliversExempt = map[string]string{
	"SC-28": "the deprecation headers keep the contract's own promise (versioning-release.md §5), which no use case states",
}

// ucChecks is what a milestone needs to know about one use case.
type ucChecks struct {
	state  string
	checks map[int]bool // the numbered items under How to check
	unmet  map[int]bool // the checks its Today lists
}

func useCaseChecks(cases []useCase) map[string]ucChecks {
	out := map[string]ucChecks{}
	for _, uc := range cases {
		sections := sectionsOf(uc.body)
		out[uc.id] = ucChecks{
			state:  uc.fields["state"],
			checks: numberedChecks(sections["How to check"]),
			unmet:  todayChecks(sections["Today"]),
		}
	}
	return out
}

var checkItem = regexp.MustCompile(`(?m)^(\d+)\.\s+\S`)

func numberedChecks(text string) map[int]bool {
	out := map[int]bool{}
	for _, m := range checkItem.FindAllStringSubmatch(text, -1) {
		n, _ := strconv.Atoi(m[1])
		out[n] = true
	}
	return out
}

// parseCheckList reads "UC-PRV-01 (9, 10), UC-PRV-03 (9–13), UC-LIF-06" into the checks it names.
// A use case without a list names every check it has; one the repository does not know is
// returned in unknown.
func parseCheckList(text string, known map[string]ucChecks) (named map[string]map[int]bool, problems []string) {
	named = map[string]map[int]bool{}
	for _, m := range checkedUC.FindAllStringSubmatch(text, -1) {
		id := m[1]
		uc, ok := known[id]
		if !ok {
			problems = append(problems, fmt.Sprintf("names %s, which does not exist", id))
			continue
		}
		if named[id] == nil {
			named[id] = map[int]bool{}
		}
		if strings.TrimSpace(m[2]) == "" {
			for n := range uc.checks {
				named[id][n] = true
			}
			continue
		}
		for _, part := range strings.Split(m[2], ",") {
			part = strings.TrimSpace(part)
			r := checkRange.FindStringSubmatch(part)
			if r == nil {
				problems = append(problems, fmt.Sprintf("names %s (%s), which is not a check number or a range", id, part))
				continue
			}
			from, _ := strconv.Atoi(r[1])
			to := from
			if r[2] != "" {
				to, _ = strconv.Atoi(r[2])
			}
			for n := from; n <= to; n++ {
				if !uc.checks[n] {
					problems = append(problems, fmt.Sprintf("names %s check %d, which %s does not have", id, n, id))
					continue
				}
				named[id][n] = true
			}
		}
	}
	return named, problems
}

// checkMilestones holds every milestone file - running and archived - to its Delivers line.
func checkMilestones(root string, cases []useCase) []string {
	var files []string
	for _, dir := range []string{filepath.Join("docs", "backlog"), filepath.Join("docs", "archive", "backlog")} {
		matches, err := filepath.Glob(filepath.Join(root, dir, "milestone-*.md"))
		if err != nil {
			return []string{fmt.Sprintf("listing %s: %v", dir, err)}
		}
		files = append(files, matches...)
	}
	known := useCaseChecks(cases)
	var problems []string
	for _, file := range files {
		relative, _ := filepath.Rel(root, file)
		problems = append(problems, milestoneProblems(relative, read(root, relative), known)...)
	}
	return problems
}

func milestoneProblems(file, content string, known map[string]ucChecks) []string {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, file+": "+fmt.Sprintf(format, args...)) }

	headings := taskHeading.FindAllStringSubmatchIndex(content, -1)
	if strings.Contains(content, "**Use cases:**") {
		for i, at := range headings {
			end := len(content)
			if i+1 < len(headings) {
				end = headings[i+1][0]
			}
			if !strings.Contains(content[at[0]:end], "**Use cases:**") {
				add("%s names no use cases, and the milestone names them for its other tasks", content[at[2]:at[3]])
			}
		}
	}

	d := deliversLine.FindStringSubmatch(content)
	if d == nil {
		return problems
	}
	delivers, parse := parseCheckList(d[1], known)
	for _, p := range parse {
		add("Delivers %s", p)
	}

	for i, at := range headings {
		end := len(content)
		if i+1 < len(headings) {
			end = headings[i+1][0]
		}
		task := content[at[2]:at[3]]
		line := useCasesLine.FindStringSubmatch(content[at[0]:end])
		if line == nil {
			add("%s has no **Use cases:** line - every task of a milestone with Delivers carries checks from it", task)
			continue
		}
		if everyDelivers.MatchString(line[1]) {
			continue
		}
		if _, exempt := deliversExempt[task]; exempt {
			continue
		}
		carried, parse := parseCheckList(line[1], known)
		for _, p := range parse {
			add("%s %s", task, p)
		}
		if len(carried) == 0 && len(parse) == 0 {
			add("%s carries no check from Delivers - a task of a released milestone carries the checks it makes true", task)
		}
		for _, id := range sortedKeys(carried) {
			var outside []string
			for _, n := range sortedInts(carried[id]) {
				if !delivers[id][n] {
					outside = append(outside, strconv.Itoa(n))
				}
			}
			if len(outside) > 0 {
				add("%s carries %s (%s), which Delivers does not name - the task moves inside Delivers, and changing Delivers is the owner's decision", task, id, strings.Join(outside, ", "))
			}
		}
	}

	if closedLine.MatchString(content) {
		for _, id := range sortedKeys(delivers) {
			uc := known[id]
			if uc.state == "specified" {
				add("is closed, and %s, which it delivers, is still specified", id)
				continue
			}
			var open []string
			for _, n := range sortedInts(delivers[id]) {
				if uc.unmet[n] {
					open = append(open, strconv.Itoa(n))
				}
			}
			if len(open) > 0 {
				add("is closed, and %s still lists check %s as not met in its Today - a milestone is done when every Delivers check is met", id, strings.Join(open, ", "))
			}
		}
	}
	return problems
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedInts(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
