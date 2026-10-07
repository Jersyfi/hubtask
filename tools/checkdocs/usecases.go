// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The use cases in docs/usecases are the owner's statement of what must be true for a person, and
// the yardstick a task is checked against (docs/vision/README.md). A yardstick is only useful while
// its marks are where they claim to be, so this holds four things together:
//
//   - every use case cites personas, deployments and principles that exist in docs/vision, so a
//     reviewer following `serves: [P-06]` lands on the principle it names;
//   - every use case carries the three sections that make it checkable - Goal, How to check (with
//     at least one numbered check) and Where it ends - and, while it is not built, a Today section
//     with one line per check not met, which is gone once it is built;
//   - the index lists every use case with the state the file declares, because the index is what a
//     reader skims and a state that drifted there is a promise nobody is keeping;
//   - every UC-… cited anywhere in the repository exists, and so does every check cited by its
//     number (UC-ID-12/4, "UC-ID-12 check 4"); what a milestone delivers is in
//     milestones.go.
//
// The front matter is a deliberately flat subset of YAML - `key: value` and `key: [a, b]` - read by
// hand here rather than through a YAML library, because a dependency is a supply-chain decision
// (AGENTS.md) and nothing richer is allowed in the files.

// ucContexts maps a folder under docs/usecases to the prefix its use cases carry. The folder is the
// name of the service package that implements the context (core/application/service/<name>).
var ucContexts = map[string]string{
	"identity":     "ID",
	"admin":        "INS",
	"work":         "WRK",
	"jumble":       "JUM",
	"automation":   "AUT",
	"integration":  "INT",
	"notification": "NOT",
	"media":        "MED",
	"lifecycle":    "LIF",
	"backup":       "BAK",
	"privacy":      "PRV",
	"audit":        "AUD",
	"suggestion":   "AI",
	"sync":         "SYN",
	"importer":     "IMP",
}

var ucStates = map[string]bool{
	"specified": true, "partial": true, "built": true, "verified": true, "retired": true,
}

var (
	ucFileName  = regexp.MustCompile(`^(UC-([A-Z]{2,3})-\d{2,3})-[a-z0-9-]+\.md$`)
	ucReference = regexp.MustCompile(`\bUC-[A-Z]{2,3}-\d{2,3}\b`)
	personaID   = regexp.MustCompile("(?m)^\\| `(PE-[a-z-]+)` \\|")
	deployment  = regexp.MustCompile("(?m)^\\| `(D\\d)` \\|")
	principleID = regexp.MustCompile(`(?m)^### (P-\d{2}):`)
	numbered    = regexp.MustCompile(`(?m)^\d+\.\s+\S`)
	ucIndexRow  = regexp.MustCompile(`(?m)^\|\s*\[(UC-[A-Z]{2,3}-\d{2,3})\]\(([^)]+)\)\s*\|[^|]*\|[^|]*\|\s*` + "`?" + `([a-z]+)` + "`?" + `\s*\|`)
	taskHeading = regexp.MustCompile(`(?m)^## ([A-Z0-9]+-\d{2,3}) `)
)

// useCase is what the gate needs from one file.
type useCase struct {
	path   string
	id     string
	fields map[string]string
	lists  map[string][]string
	body   string
}

func checkUseCases(root string) []string {
	base := filepath.Join(root, "docs", "usecases")
	if _, err := os.Stat(base); err != nil {
		return []string{fmt.Sprintf("docs/usecases is not readable: %v", err)}
	}

	vision := func(name string) string { return read(root, filepath.Join("docs", "vision", name)) }
	personas := setOf(personaID.FindAllStringSubmatch(vision("personas.md"), -1))
	deployments := setOf(deployment.FindAllStringSubmatch(vision("deployments.md"), -1))
	principles := setOf(principleID.FindAllStringSubmatch(vision("principles.md"), -1))

	var problems []string
	if len(personas) == 0 || len(deployments) == 0 || len(principles) == 0 {
		problems = append(problems, "docs/vision: personas, deployments or principles could not be read - the use cases cite them")
	}

	cases, walkProblems := collectUseCases(root, base)
	problems = append(problems, walkProblems...)

	seen := map[string]string{}
	for _, uc := range cases {
		if other, dup := seen[uc.id]; dup {
			problems = append(problems, fmt.Sprintf("%s: %s is also the id of %s", uc.path, uc.id, other))
		}
		seen[uc.id] = uc.path
		problems = append(problems, checkUseCase(root, uc, personas, deployments, principles)...)
	}

	problems = append(problems, checkUseCaseIndex(root, cases)...)
	problems = append(problems, checkUseCaseReferences(root, seen, useCaseChecks(cases))...)
	problems = append(problems, checkMilestones(root, cases)...)
	return problems
}

func collectUseCases(root, base string) ([]useCase, []string) {
	var cases []useCase
	var problems []string
	err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		name := entry.Name()
		match := ucFileName.FindStringSubmatch(name)
		if match == nil {
			if strings.HasPrefix(name, "UC-") {
				problems = append(problems, fmt.Sprintf("%s: a use case file is named UC-XX-nn-short-title.md", relative))
			}
			return nil
		}
		folder := filepath.Base(filepath.Dir(path))
		prefix, known := ucContexts[folder]
		if !known {
			problems = append(problems, fmt.Sprintf("%s: %s is not a context folder (see docs/usecases/README.md)", relative, folder))
			return nil
		}
		if match[2] != prefix {
			problems = append(problems, fmt.Sprintf("%s: a use case in %s/ carries the prefix UC-%s", relative, folder, prefix))
		}
		uc, parseProblem := parseUseCase(relative, read(root, relative))
		if parseProblem != "" {
			problems = append(problems, parseProblem)
			return nil
		}
		if uc.id != match[1] {
			problems = append(problems, fmt.Sprintf("%s: the id is %s but the file name says %s", relative, uc.id, match[1]))
		}
		if uc.fields["context"] != folder {
			problems = append(problems, fmt.Sprintf("%s: context is %q, the folder is %q", relative, uc.fields["context"], folder))
		}
		cases = append(cases, uc)
		return nil
	})
	if err != nil {
		problems = append(problems, fmt.Sprintf("walking docs/usecases: %v", err))
	}
	return cases, problems
}

// parseUseCase reads the flat front matter and keeps the body for the section checks.
func parseUseCase(path, content string) (useCase, string) {
	uc := useCase{path: path, fields: map[string]string{}, lists: map[string][]string{}}
	if !strings.HasPrefix(content, "---\n") {
		return uc, fmt.Sprintf("%s: a use case starts with a front matter block between two --- lines", path)
	}
	head, body, found := strings.Cut(content[4:], "\n---\n")
	if !found {
		return uc, fmt.Sprintf("%s: the front matter is not closed with ---", path)
	}
	for _, line := range strings.Split(head, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return uc, fmt.Sprintf("%s: %q is not a key: value line", path, line)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if strings.HasPrefix(value, "[") {
			if !strings.HasSuffix(value, "]") {
				return uc, fmt.Sprintf("%s: %s is a list and is written [a, b] on one line", path, key)
			}
			var items []string
			for _, item := range strings.Split(strings.Trim(value, "[]"), ",") {
				if item = strings.TrimSpace(item); item != "" {
					items = append(items, item)
				}
			}
			uc.lists[key] = items
			continue
		}
		uc.fields[key] = value
	}
	uc.id = uc.fields["id"]
	uc.body = body
	return uc, ""
}

func checkUseCase(root string, uc useCase, personas, deployments, principles map[string]bool) []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, uc.path+": "+fmt.Sprintf(format, args...))
	}

	for _, key := range []string{"id", "title", "context", "state"} {
		if uc.fields[key] == "" {
			add("%s is missing", key)
		}
	}
	for _, key := range []string{"actors", "deployments", "serves", "tasks", "checked_by"} {
		if _, ok := uc.lists[key]; !ok {
			add("%s is missing (write [] for none)", key)
		}
	}
	state := uc.fields["state"]
	if state != "" && !ucStates[state] {
		add("state %q is not one of specified, partial, built, verified, retired", state)
	}

	cite := func(key string, known map[string]bool, where string) {
		if len(uc.lists[key]) == 0 && state != "retired" {
			add("%s names nobody", key)
		}
		for _, item := range uc.lists[key] {
			if !known[item] {
				add("%s names %s, which is not in docs/vision/%s", key, item, where)
			}
		}
	}
	cite("actors", personas, "personas.md")
	cite("deployments", deployments, "deployments.md")
	cite("serves", principles, "principles.md")

	for _, evidence := range uc.lists["checked_by"] {
		if _, err := os.Stat(filepath.Join(root, evidence)); err != nil {
			add("checked_by names %s, which does not exist", evidence)
		}
	}
	if state == "verified" && len(uc.lists["checked_by"]) == 0 {
		add("a verified use case names what proves it in checked_by")
	}

	sections := sectionsOf(uc.body)
	if state == "retired" {
		return problems
	}
	for _, name := range []string{"Goal", "How to check", "Where it ends"} {
		if _, ok := sections[name]; !ok {
			add("the section ## %s is missing", name)
		}
	}
	if checks, ok := sections["How to check"]; ok && !numbered.MatchString(checks) {
		add("## How to check has no numbered check")
	}
	today, hasToday := sections["Today"]
	switch {
	case (state == "specified" || state == "partial") && !hasToday:
		add("a %s use case says in ## Today what is not met yet", state)
	case (state == "built" || state == "verified") && hasToday:
		add("a %s use case has no ## Today - every check is met, so nothing is left to list", state)
	}
	if hasToday {
		problems = append(problems, todayProblems(uc.path, today, numberedChecks(sections["How to check"]))...)
	}
	return problems
}

// todayProblems holds a Today section to what it is for: one line per check not met yet, naming
// the check, and nothing else - a Today that narrates is a second description of the use case that
// drifts from the first.
func todayProblems(path, today string, checks map[int]bool) []string {
	var problems []string
	listed := map[int]bool{}
	for _, line := range strings.Split(today, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := todayLine.FindStringSubmatch(line)
		if m == nil {
			short := strings.TrimSpace(line)
			if len(short) > 60 {
				short = short[:60] + "…"
			}
			problems = append(problems, fmt.Sprintf("%s: ## Today has %q - each line is \"* Check n: not met — …\" for one check", path, short))
			continue
		}
		n, _ := strconv.Atoi(m[1])
		switch {
		case !checks[n]:
			problems = append(problems, fmt.Sprintf("%s: ## Today lists check %d, which ## How to check does not have", path, n))
		case listed[n]:
			problems = append(problems, fmt.Sprintf("%s: ## Today lists check %d twice - one line per check", path, n))
		}
		listed[n] = true
	}
	if len(listed) == 0 {
		problems = append(problems, fmt.Sprintf("%s: ## Today names no check - it lists each check not met yet, one line each", path))
	}
	return problems
}

// sectionsOf splits a body at its level-two headings.
func sectionsOf(body string) map[string]string {
	sections := map[string]string{}
	var current string
	var text strings.Builder
	flush := func() {
		if current != "" {
			sections[current] = text.String()
		}
		text.Reset()
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			current = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			continue
		}
		text.WriteString(line)
		text.WriteString("\n")
	}
	flush()
	return sections
}

func checkUseCaseIndex(root string, cases []useCase) []string {
	index := read(root, filepath.Join("docs", "usecases", "README.md"))
	listed := map[string]string{}
	var problems []string
	for _, row := range ucIndexRow.FindAllStringSubmatch(index, -1) {
		listed[row[1]] = row[3]
	}
	for _, uc := range cases {
		state, ok := listed[uc.id]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("docs/usecases/README.md: %s is not in the index", uc.id))
		case state != uc.fields["state"]:
			problems = append(problems, fmt.Sprintf("docs/usecases/README.md: the index says %s is %s, the file says %s", uc.id, state, uc.fields["state"]))
		}
		delete(listed, uc.id)
	}
	for id := range listed {
		problems = append(problems, fmt.Sprintf("docs/usecases/README.md: the index lists %s, which has no file", id))
	}
	return problems
}

// ucCheckReference is a citation of one check: `UC-ID-12/4`, or `UC-ID-12 check 4`.
var ucCheckReference = regexp.MustCompile(`\b(UC-[A-Z]{2,3}-\d{2,3})(?:/(\d+)\b|:?,? check (\d+)\b)`)

// checkUseCaseReferences makes sure a UC-… cited anywhere resolves, the way checkADRReferences does
// for decisions: a task, a pull request template or a code comment pointing at a use case that does
// not exist is a requirement that looks recorded and is not. A check cited by its number exists
// under the use case's How to check, for the same reason.
func checkUseCaseReferences(root string, known map[string]string, checks map[string]ucChecks) []string {
	cited := map[string][]string{}
	missingChecks := map[string][]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		if !citesADRs(entry.Name()) || snapshot(relative) {
			return nil
		}
		content, readErr := os.ReadFile(path) //nolint:gosec // G304: walking this repository is the job
		if readErr != nil {
			return readErr
		}
		for _, id := range ucReference.FindAllString(string(content), -1) {
			cited[id] = append(cited[id], relative)
		}
		for _, m := range ucCheckReference.FindAllStringSubmatch(string(content), -1) {
			number := m[2] + m[3]
			uc, ok := checks[m[1]]
			if n, _ := strconv.Atoi(number); ok && !uc.checks[n] {
				key := m[1] + " check " + number
				missingChecks[key] = append(missingChecks[key], relative)
			}
		}
		return nil
	})
	if err != nil {
		return []string{fmt.Sprintf("walking the repository: %v", err)}
	}
	var problems []string
	for id, files := range cited {
		if _, ok := known[id]; ok {
			continue
		}
		sort.Strings(files)
		problems = append(problems, fmt.Sprintf("%s is cited in %s and does not exist", id, strings.Join(unique(files), ", ")))
	}
	for check, files := range missingChecks {
		sort.Strings(files)
		problems = append(problems, fmt.Sprintf("%s is cited in %s and that use case has no such check", check, strings.Join(unique(files), ", ")))
	}
	return problems
}

// todayLine is one line of a Today section: "* Check 4: not met — …".
var todayLine = regexp.MustCompile(`(?m)^\*\s+Check (\d+)\b`)

// todayChecks are the checks a Today section lists as not met.
func todayChecks(text string) map[int]bool {
	out := map[int]bool{}
	for _, m := range todayLine.FindAllStringSubmatch(text, -1) {
		n, _ := strconv.Atoi(m[1])
		out[n] = true
	}
	return out
}

func setOf(matches [][]string) map[string]bool {
	set := map[string]bool{}
	for _, match := range matches {
		set[match[1]] = true
	}
	return set
}
