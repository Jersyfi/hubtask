// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// taggedSections are the parts of the root AGENTS.md whose items are rules. Every item there says
// what checks it (ADR-0081): a gate, part of a gate, the owner, or nothing - and then why. A rule
// that claims a gate names one that exists.
var taggedSections = []string{"Rules that do not bend", "Working rules", "Working with the owner", "Code comments"}

// ruleTag matches the tag an item ends with: `[gate: gate-docs]`, `[partial: gate-quick; open: …]`,
// `[owner]`, `[unchecked: why]`.
var ruleTag = regexp.MustCompile("`\\[(gate|partial|owner|unchecked)(?::\\s*([^\\]]*))?\\]`")

// checkRuleTags holds every rule in AGENTS.md to a tag, and every tag that names a gate to a gate
// that exists in the Makefile or as a job in ci.yml.
func checkRuleTags(root string) []string {
	agents := read(root, "AGENTS.md")
	if agents == "" {
		return []string{"AGENTS.md: missing - it is the instruction for every coding worker"}
	}
	targets := makeTargets(read(root, "Makefile"))
	jobs := ciJobs(read(root, filepath.Join(".github", "workflows", "ci.yml")))
	problems := ruleTagProblems(agents, targets, jobs)
	for _, file := range nestedAgentsFiles(root) {
		problems = append(problems, nestedTagProblems(file, read(root, file), targets, jobs)...)
	}
	return problems
}

// frozenHeadings are cited by older documents, migrations and ADRs by their words ("CLAUDE.md rule
// N", "The loop for every task", step 6): they stay as they are, and so do the rule numbers and the
// loop's step numbers.
var frozenHeadings = []string{"Rules that do not bend", "What you do not decide yourself", "The loop for every task", "Which command checks what"}

var (
	ruleNumber = regexp.MustCompile(`(?m)^\|\s*(\d+)\s*\|`)
	loopStep   = regexp.MustCompile(`(?m)^(\d+)\.\s`)
)

// frozenProblems holds the cited headings to their words, the rules to the numbers 1 to 15 and on
// without a gap, and the loop to its steps 1 to 7.
func frozenProblems(agents string) []string {
	var problems []string
	for _, title := range frozenHeadings {
		if _, ok := sectionBody(agents, title); !ok {
			problems = append(problems, fmt.Sprintf("AGENTS.md: the heading %q is gone - it is cited by its words, so it stays as it is", title))
		}
	}
	if rules, ok := sectionBody(agents, "Rules that do not bend"); ok {
		numbers := ruleNumber.FindAllStringSubmatch(rules, -1)
		for i, m := range numbers {
			if m[1] != strconv.Itoa(i+1) {
				problems = append(problems, fmt.Sprintf("AGENTS.md § Rules that do not bend: rule %s stands where rule %d belongs - a rule's number is permanent, a new rule takes the next one", m[1], i+1))
				break
			}
		}
		if len(numbers) < 15 {
			problems = append(problems, fmt.Sprintf("AGENTS.md § Rules that do not bend: %d rules, not 15 - a rule is never removed, its number is cited", len(numbers)))
		}
	}
	if loop, ok := sectionBody(agents, "The loop for every task"); ok {
		// The steps are the list before the first subsection; "Reading" numbers its own.
		loop, _, _ = strings.Cut(loop, "\n### ")
		var steps []string
		for _, m := range loopStep.FindAllStringSubmatch(loop, -1) {
			steps = append(steps, m[1])
		}
		if strings.Join(steps, " ") != "1 2 3 4 5 6 7" {
			problems = append(problems, fmt.Sprintf("AGENTS.md § The loop for every task: the steps are numbered %v, not 1 to 7 - the step numbers are cited", steps))
		}
	}
	return problems
}

// A rule stands in a rule list, as an item with its tag; everything else in AGENTS.md explains.
// The rule lists are the tagged sections: "Rules that do not bend" and "Working rules", and the
// two lists of the same kind added beside them - "Working with the owner" and "Code comments" -
// whose items are rules held to the same tags. A sentence elsewhere that commands - must, never,
// always, do not - is a rule nobody tagged: it is rewritten to describe, or moved into a list with
// its tag.
var normativeWord = regexp.MustCompile(`(?i)\b(must|never|always|do not|don't)\b`)

var inlineCode = regexp.MustCompile("`[^`]*`")

// normativeProblems reports the lines outside the rule items that command. Headings, code blocks
// and inline code are not prose; a quoted heading is a name.
func normativeProblems(agents string) []string {
	tagged := map[string]bool{}
	for _, s := range taggedSections {
		tagged[s] = true
	}
	var problems []string
	section, fenced, inItem := "", false, false
	for i, line := range strings.Split(agents, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			fenced = !fenced
			continue
		case fenced:
			continue
		case strings.HasPrefix(line, "## "):
			section, inItem = strings.TrimSpace(strings.TrimPrefix(line, "## ")), false
			continue
		case strings.HasPrefix(line, "#"):
			continue
		case trimmed == "":
			inItem = false
			continue
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "|"):
			inItem = true
		}
		if tagged[section] && inItem {
			continue
		}
		prose := inlineCode.ReplaceAllString(line, "")
		for _, title := range append(append([]string{}, frozenHeadings...), nestedRuleSections...) {
			prose = strings.ReplaceAll(prose, title, "")
		}
		if m := normativeWord.FindString(prose); m != "" {
			problems = append(problems, fmt.Sprintf("AGENTS.md:%d: %q outside the rule lists - describe instead of command, or make it an item of a rule list with its tag", i+1, strings.ToLower(m)))
		}
	}
	return problems
}

func ruleTagProblems(agents string, targets, jobs map[string]bool) []string {
	problems := frozenProblems(agents)
	problems = append(problems, normativeProblems(agents)...)
	for _, section := range taggedSections {
		body, ok := sectionBody(agents, section)
		if !ok {
			problems = append(problems, fmt.Sprintf("AGENTS.md: the section %q is missing", section))
			continue
		}
		for _, item := range ruleItems(body) {
			problems = append(problems, itemTagProblems("AGENTS.md § "+section, item, targets, jobs)...)
		}
	}
	return problems
}

// itemTagProblems holds one rule to its tag: there is one, a gate or CI job it names exists - one
// or several, comma-separated - a partial one says what stays open, an unchecked one says why.
func itemTagProblems(where, item string, targets, jobs map[string]bool) []string {
	short := item
	if len(short) > 60 {
		short = short[:60] + "…"
	}
	m := ruleTag.FindStringSubmatch(item)
	if m == nil {
		return []string{fmt.Sprintf("%s: %q says nothing about what checks it - tag it [gate: …], [partial: …; open: …], [owner] or [unchecked: why]", where, short)}
	}
	var problems []string
	kind, arg := m[1], strings.TrimSpace(m[2])
	switch kind {
	case "gate", "partial":
		for _, target := range strings.Split(strings.SplitN(arg, ";", 2)[0], ",") {
			target = strings.TrimSpace(target)
			if strings.HasPrefix(target, "ci:") {
				if !jobs[strings.TrimPrefix(target, "ci:")] {
					problems = append(problems, fmt.Sprintf("%s: %q names the CI job %s, which ci.yml does not have", where, short, target))
				}
			} else if !targets[target] {
				problems = append(problems, fmt.Sprintf("%s: %q names the gate %s, which the Makefile does not have", where, short, target))
			}
		}
		if kind == "partial" && !strings.Contains(arg, "open:") {
			problems = append(problems, fmt.Sprintf("%s: %q is partly gated but does not say what stays open", where, short))
		}
	case "unchecked":
		if arg == "" {
			problems = append(problems, fmt.Sprintf("%s: %q is unchecked without saying why", where, short))
		}
	}
	return problems
}

// nestedRuleSections are the sections of a directory's own AGENTS.md whose items are rules: what
// must not happen in that directory, and for the website what it may claim. The other sections -
// what the directory is, how to check a change - explain, and carry no tags.
var nestedRuleSections = []string{"What must not happen here", "What the site may claim"}

// nestedTagProblems holds every rule of a directory's AGENTS.md to a tag, the way the root's rules
// are held: a rule there binds whoever works in the directory just as much.
func nestedTagProblems(file, doc string, targets, jobs map[string]bool) []string {
	var problems []string
	for _, section := range nestedRuleSections {
		body, ok := sectionBody(doc, section)
		if !ok {
			continue
		}
		for _, item := range ruleItems(body) {
			problems = append(problems, itemTagProblems(file+" § "+section, item, targets, jobs)...)
		}
	}
	return problems
}

// nestedAgentsFiles lists every AGENTS.md below the root.
func nestedAgentsFiles(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == "AGENTS.md" {
			if relative, _ := filepath.Rel(root, path); relative != "AGENTS.md" {
				files = append(files, filepath.ToSlash(relative))
			}
		}
		return nil
	})
	return files
}

// sectionBody returns the text under a "## <title>" heading, up to the next heading of the same
// or a higher level.
func sectionBody(doc, title string) (string, bool) {
	lines := strings.Split(doc, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "## "+title {
			continue
		}
		var body []string
		for _, l := range lines[i+1:] {
			if strings.HasPrefix(l, "## ") || strings.HasPrefix(l, "# ") {
				break
			}
			body = append(body, l)
		}
		return strings.Join(body, "\n"), true
	}
	return "", false
}

// ruleItems splits a section into its rules: table rows that are not the header or the separator,
// and list items with their continuation lines.
func ruleItems(body string) []string {
	var items []string
	var current []string
	flush := func() {
		if len(current) > 0 {
			items = append(items, strings.Join(current, " "))
			current = nil
		}
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "|"):
			flush()
			if strings.HasPrefix(trimmed, "|---") || strings.HasPrefix(trimmed, "| #") || strings.HasPrefix(trimmed, "|#") {
				continue
			}
			items = append(items, trimmed)
		case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
			flush()
			current = []string{trimmed}
		case trimmed == "":
			flush()
		default:
			if current != nil {
				current = append(current, trimmed)
			}
		}
	}
	flush()
	return items
}

var makeTarget = regexp.MustCompile(`(?m)^([a-z0-9][a-z0-9-]*):`)

func makeTargets(makefile string) map[string]bool {
	out := map[string]bool{}
	for _, m := range makeTarget.FindAllStringSubmatch(makefile, -1) {
		out[m[1]] = true
	}
	return out
}

var ciJob = regexp.MustCompile(`^  ([a-z0-9][a-z0-9-]*):\s*$`)

// ciJobs reads the job ids under `jobs:` - only there: `on:` has keys at the same depth (`push:`,
// `pull_request:`) that are no job a rule could be checked by.
func ciJobs(workflow string) map[string]bool {
	out := map[string]bool{}
	inJobs := false
	for _, line := range strings.Split(workflow, "\n") {
		if line != "" && line[0] != ' ' && line[0] != '#' {
			inJobs = strings.TrimSpace(line) == "jobs:"
			continue
		}
		if m := ciJob.FindStringSubmatch(line); inJobs && m != nil {
			out[m[1]] = true
		}
	}
	return out
}

// checkADRRuleLines holds every ADR to the line that says where its rule lives (ADR-0081): an ADR
// is the record of why, and a reader who lands on it must find the current rule in one step.
func checkADRRuleLines(root string) []string {
	dir := filepath.Join(root, "docs", "adr")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{fmt.Sprintf("docs/adr: %v", err)}
	}
	var problems []string
	for _, e := range entries {
		if !adrFile.MatchString(e.Name()) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // G304: an ADR file listed from docs/adr
		if err != nil {
			problems = append(problems, fmt.Sprintf("docs/adr/%s: %v", e.Name(), err))
			continue
		}
		if !hasRuleLine(string(raw)) {
			problems = append(problems, fmt.Sprintf("docs/adr/%s: no \"**Rule lives in:**\" line under its status - name the section that holds its rule, or \"nowhere — <why>\"", e.Name()))
		}
	}
	return problems
}

func hasRuleLine(adr string) bool {
	lines := strings.Split(adr, "\n")
	if len(lines) > 25 {
		lines = lines[:25]
	}
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "**Rule lives in:**") {
			return true
		}
	}
	return false
}
