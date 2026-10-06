// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	return ruleTagProblems(agents, targets, jobs)
}

func ruleTagProblems(agents string, targets, jobs map[string]bool) []string {
	var problems []string
	for _, section := range taggedSections {
		body, ok := sectionBody(agents, section)
		if !ok {
			problems = append(problems, fmt.Sprintf("AGENTS.md: the section %q is missing", section))
			continue
		}
		for _, item := range ruleItems(body) {
			short := item
			if len(short) > 60 {
				short = short[:60] + "…"
			}
			m := ruleTag.FindStringSubmatch(item)
			if m == nil {
				problems = append(problems, fmt.Sprintf("AGENTS.md § %s: %q says nothing about what checks it - tag it [gate: …], [partial: …; open: …], [owner] or [unchecked: why]", section, short))
				continue
			}
			kind, arg := m[1], strings.TrimSpace(m[2])
			switch kind {
			case "gate", "partial":
				target := strings.TrimSpace(strings.SplitN(arg, ";", 2)[0])
				if strings.HasPrefix(target, "ci:") {
					if !jobs[strings.TrimPrefix(target, "ci:")] {
						problems = append(problems, fmt.Sprintf("AGENTS.md § %s: %q names the CI job %s, which ci.yml does not have", section, short, target))
					}
				} else if !targets[target] {
					problems = append(problems, fmt.Sprintf("AGENTS.md § %s: %q names the gate %s, which the Makefile does not have", section, short, target))
				}
				if kind == "partial" && !strings.Contains(arg, "open:") {
					problems = append(problems, fmt.Sprintf("AGENTS.md § %s: %q is partly gated but does not say what stays open", section, short))
				}
			case "unchecked":
				if arg == "" {
					problems = append(problems, fmt.Sprintf("AGENTS.md § %s: %q is unchecked without saying why", section, short))
				}
			}
		}
	}
	return problems
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
		case strings.HasPrefix(trimmed, "- "):
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

var ciJob = regexp.MustCompile(`(?m)^  ([a-z0-9][a-z0-9-]*):\s*$`)

func ciJobs(workflow string) map[string]bool {
	out := map[string]bool{}
	for _, m := range ciJob.FindAllStringSubmatch(workflow, -1) {
		out[m[1]] = true
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
