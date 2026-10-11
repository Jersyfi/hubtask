// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// What code may cite (AGENTS.md, "Code comments"): stable references that keep resolving when work
// moves on - a rule, a principle, a use case check, a subject-document section, an ADR for the
// reasoning. Not a task, an issue, a milestone file or an instruction file: that context lives in
// the commit and the pull request, and in a comment it is history a reader cannot act on.

var (
	sectionCitation = regexp.MustCompile(`([a-z0-9-]+\.md)\s*§\s*(\d+(?:\.\d+)*)`)
	taskCitation    = regexp.MustCompile(`\b(?:SC|SI|PH|F\d{1,2})-\d{2}\b`)
	letterTask      = regexp.MustCompile(`\b[A-Z]-\d{2}\b`)
	letterHeading   = regexp.MustCompile(`(?m)^## ([A-Z]-\d{2}) `)
	issueCitation   = regexp.MustCompile(`(?:(?:^|[^\w&/])#\d{3,4}\b)|(?:\b[Ii]ssues? #?\d{3,4}\b)`)
	milestoneFile   = regexp.MustCompile(`milestone-[A-Za-z0-9.]+\.md`)
	instructionFile = regexp.MustCompile(`\b(?:CLAUDE|AGENTS)\.md\b`)
	numberedHeading = regexp.MustCompile(`^#{2,5}\s+(\d+(?:\.\d+)*)\.?\s`)
	boldNumber      = regexp.MustCompile(`^\s*\*\*(\d+\.\d+(?:\.\d+)*)\b`)
	numberedItem    = regexp.MustCompile(`^\s*(?:[-*]\s+)?(?:\*\*)?(\d+)[.)]`)
	publicReference = regexp.MustCompile(`ADR-\d{4}|\b[a-z0-9-]+\.md\b|§`)
)

// codeExtensions are the files whose comments and strings this reads. A `.json` file is code or
// data the code reads (schemas, dashboards, manifests), and a `.md` file outside docs/ documents
// the code beside it (a package's README, a runbook, a prompt): both are read as code is.
var codeExtensions = map[string]bool{
	".go": true, ".ts": true, ".svelte": true, ".js": true, ".mjs": true, ".sql": true,
	".yaml": true, ".yml": true, ".sh": true, ".tpl": true, ".json": true, ".md": true,
	".py": true, ".html": true, ".css": true,
}

// citationExempt are files that name these things as data, not as citations: the gates that check
// them, the script that recreates the repository, the issue forms' examples.
var citationExempt = map[string]bool{
	"test/architecture/instructions_test.go": true,
	"tools/checkdocs/agents.go":              true,
	"tools/checkdocs/agents_test.go":         true,
	"tools/checkdocs/citations.go":           true,
	"tools/checkdocs/citations_test.go":      true,
	"tools/checkdocs/milestones.go":          true,
	"tools/checkdocs/usecases.go":            true,
	"tools/checkdocs/main.go":                true,
	"tools/checkpr/main.go":                  true,
	"tools/checkpr/readiness.go":             true,
	"scripts/relaunch.sh":                    true,
}

func citationScope(file string) bool {
	switch {
	case strings.HasPrefix(file, "db/migrations/"), strings.HasPrefix(file, "docs/"),
		strings.HasPrefix(file, "third-party/"), strings.HasPrefix(file, "locales/"),
		strings.HasPrefix(file, "infrastructure/postgres/sqlc/"), strings.HasPrefix(file, "sdk/python/hubtask/"),
		strings.HasPrefix(file, ".github/ISSUE_TEMPLATE/"), strings.Contains(file, "/node_modules/"),
		strings.HasSuffix(file, ".gen.go"), strings.HasSuffix(file, ".gen.ts"),
		file == "api/openapi.json", file == "pnpm-lock.yaml", citationExempt[file]:
		return false
	case path.Ext(file) == ".md" && !codeDocument(file):
		return false
	}
	return codeExtensions[path.Ext(file)] || path.Base(file) == "Makefile" || path.Base(file) == "Dockerfile"
}

// codeDocument is a Markdown file that documents code: one inside a code directory. The documents
// at the root (README, CONTRIBUTING, SECURITY, …) and under docs/ are the project's documents, which
// cite tasks and issues as their history; .github/ holds the process's own forms; an AGENTS.md is
// an instruction file and names the others by design.
func codeDocument(file string) bool {
	return strings.Contains(file, "/") && !strings.HasPrefix(file, ".github/") && path.Base(file) != "AGENTS.md"
}

// stableLetters are the single letters of identifiers that stay: alerts (A-14), constraints (C-03)
// and quality goals (Q-02) in arc42, principles (P-05), risks (R-09) and threats (T-07). The early
// milestones lettered their tasks A to W, and a task A-05 cannot be told from alert A-05 by its
// shape - so a citation of a task lettered A, C or P is left to review.
const stableLetters = "ACPQRT"

// letterTasks are the single-letter task IDs the milestones ever named in a task heading (## G-02),
// except those whose letter a stable identifier uses.
func letterTasks(root string) map[string]bool {
	tasks := map[string]bool{}
	for _, dir := range []string{filepath.Join("docs", "backlog"), filepath.Join("docs", "archive", "backlog")} {
		files, _ := filepath.Glob(filepath.Join(root, dir, "milestone-*.md"))
		for _, file := range files {
			relative, _ := filepath.Rel(root, file)
			for _, m := range letterHeading.FindAllStringSubmatch(read(root, relative), -1) {
				if !strings.ContainsRune(stableLetters, rune(m[1][0])) {
					tasks[m[1]] = true
				}
			}
		}
	}
	return tasks
}

// isTestData is a file whose literals may carry a task or issue number as data (a fixture, a test
// of the gate that refuses them); its section citations are still checked.
func isTestData(file string) bool {
	base := path.Base(file)
	return strings.HasSuffix(file, "_test.go") || strings.Contains(base, ".test.") ||
		strings.Contains(file, "/testdata/") || strings.Contains(file, "/e2e/fixture")
}

func checkCodeCitations(root string) []string {
	files, err := trackedFiles(root)
	if err != nil {
		return []string{fmt.Sprintf("citations: %v", err)}
	}
	sections := documentSections(root, files)
	tasks := letterTasks(root)
	var problems []string
	for _, file := range files {
		if !citationScope(file) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file))) //nolint:gosec // G304: a tracked file of this repository
		if err != nil {
			continue
		}
		problems = append(problems, citationProblems(file, string(raw), sections, tasks)...)
	}
	problems = append(problems, publicTextProblems(read(root, filepath.Join("api", "openapi.yaml")))...)
	return problems
}

func citationProblems(file, text string, sections map[string]*docSections, tasks map[string]bool) []string {
	var problems []string
	data := isTestData(file)
	for i, line := range strings.Split(text, "\n") {
		where := fmt.Sprintf("%s:%d", file, i+1)
		for _, m := range sectionCitation.FindAllStringSubmatch(line, -1) {
			if m[1] == "README.md" {
				continue
			}
			doc, ok := sections[m[1]]
			if !ok {
				problems = append(problems, fmt.Sprintf("%s: cites %s, which is no document under docs/", where, m[1]))
			} else if !doc.has(m[2]) {
				problems = append(problems, fmt.Sprintf("%s: cites %s §%s, which %s does not have - cite the section that holds the rule now", where, m[1], m[2], m[1]))
			}
		}
		if data {
			continue
		}
		if m := taskCitation.FindString(line); m != "" {
			problems = append(problems, fmt.Sprintf("%s: cites the task %s - cite the rule or the use case check instead (AGENTS.md, Code comments)", where, m))
		}
		for _, m := range letterTask.FindAllString(line, -1) {
			if tasks[m] {
				problems = append(problems, fmt.Sprintf("%s: cites the task %s - cite the rule or the use case check instead (AGENTS.md, Code comments)", where, m))
				break
			}
		}
		if m := issueIn(line); m != "" {
			problems = append(problems, fmt.Sprintf("%s: cites an issue or pull request (%s) - the reason, not the ticket", where, strings.TrimSpace(m)))
		}
		if m := milestoneFile.FindString(line); m != "" {
			problems = append(problems, fmt.Sprintf("%s: cites %s - a milestone's decisions live in the subject documents now", where, m))
		}
		// A package's README sends a reader to the AGENTS.md beside it, which is a map rather than a
		// citation; a comment in code cites the rule itself.
		if m := instructionFile.FindString(line); m != "" && path.Ext(file) != ".md" {
			problems = append(problems, fmt.Sprintf("%s: cites %s - cite the rule (\"rule N\") or the subject document", where, m))
		}
	}
	return problems
}

// issueIn finds an issue or pull request number in a line. `#359` is also a colour, which a
// stylesheet, a Svelte style block or tokens.json writes as a value - after a colon, an equals sign
// or a quote - and an issue citation never stands there, so a match in that place is skipped.
func issueIn(line string) string {
	for _, at := range issueCitation.FindAllStringIndex(line, -1) {
		match := line[at[0]:at[1]]
		if i := strings.Index(match, "#"); i >= 0 {
			before := strings.TrimRight(line[:at[0]+i], " \t")
			if before != "" && strings.ContainsAny(before[len(before)-1:], ":=\"'") {
				continue
			}
		}
		return match
	}
	return ""
}

// publicTextProblems holds the API description - rendered on the website and into the SDKs - to
// no internal reference: a reader of the contract cannot resolve an ADR number or a section.
func publicTextProblems(openapi string) []string {
	var problems []string
	for i, line := range strings.Split(openapi, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if m := publicReference.FindString(line); m != "" {
			problems = append(problems, fmt.Sprintf("api/openapi.yaml:%d: the public API description names %q - describe the behaviour instead", i+1, m))
		}
		if m := taskCitation.FindString(line); m != "" {
			problems = append(problems, fmt.Sprintf("api/openapi.yaml:%d: the public API description names the task %s", i+1, m))
		}
	}
	return problems
}

type docSections struct {
	headings map[string]bool // "4", "4.2"
	items    map[string]bool // "4.1" = numbered item 1 inside section 4
}

func (d *docSections) has(number string) bool {
	if d.headings[number] || d.items[number] {
		return true
	}
	// "§4" also names a section whose first subsection is "4.1".
	for h := range d.headings {
		if strings.HasPrefix(h, number+".") {
			return true
		}
	}
	return false
}

func documentSections(root string, files []string) map[string]*docSections {
	out := map[string]*docSections{}
	for _, file := range files {
		if !strings.HasPrefix(file, "docs/") || strings.HasPrefix(file, "docs/archive/") || !strings.HasSuffix(file, ".md") {
			continue
		}
		d := &docSections{headings: map[string]bool{}, items: map[string]bool{}}
		current := ""
		for _, line := range strings.Split(read(root, file), "\n") {
			if m := numberedHeading.FindStringSubmatch(line); m != nil {
				d.headings[m[1]] = true
				current = m[1]
				continue
			}
			if strings.HasPrefix(line, "#") {
				continue
			}
			if m := boldNumber.FindStringSubmatch(line); m != nil {
				d.headings[m[1]] = true
				continue
			}
			if m := numberedItem.FindStringSubmatch(line); m != nil && current != "" {
				d.items[current+"."+m[1]] = true
			}
		}
		base := path.Base(file)
		if prev, ok := out[base]; ok {
			for k := range d.headings {
				prev.headings[k] = true
			}
			for k := range d.items {
				prev.items[k] = true
			}
			continue
		}
		out[base] = d
	}
	return out
}

func trackedFiles(root string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "ls-files")
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	return strings.Fields(out.String()), nil
}

// errataRow is a row of the errata table in docs/adr/README.md: | ADR-nnnn | cited | meant |.
var errataRow = regexp.MustCompile(`^\|\s*\[?ADR-(\d{4})\]?(?:\([^)]*\))?\s*\|\s*([a-z0-9-]+\.md)\s*§\s*(\d+(?:\.\d+)*)\s*\|\s*([a-z0-9-]+\.md)\s*§\s*(\d+(?:\.\d+)*)\s*\|`)

// linkedCitation is a section citation through a link, the way documents write it:
// [versioning-release.md](../architecture/versioning-release.md) §2.
var linkedCitation = regexp.MustCompile(`\[([a-z0-9-]+\.md)\]\([^)]*\)\s*§\s*(\d+(?:\.\d+)*)`)

// checkADRCitations holds the section citations of the ADRs to the documents they cite. An ADR keeps
// its text once accepted (docs/adr/README.md), so a citation that was wrong when it was written, or
// went wrong later, is not edited: the README's errata table names the section it means, and a
// citation it names is accepted as long as the section it means exists.
func checkADRCitations(root string) []string {
	files, err := trackedFiles(root)
	if err != nil {
		return []string{fmt.Sprintf("ADR citations: %v", err)}
	}
	return adrCitationProblems(files, func(file string) string { return read(root, file) }, documentSections(root, files))
}

func adrCitationProblems(files []string, content func(string) string, sections map[string]*docSections) []string {
	errata := map[string]bool{}
	var problems []string
	for i, line := range strings.Split(content("docs/adr/README.md"), "\n") {
		m := errataRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		errata["ADR-"+m[1]+" "+m[2]+" §"+m[3]] = true
		if doc, ok := sections[m[4]]; !ok || !doc.has(m[5]) {
			problems = append(problems, fmt.Sprintf("docs/adr/README.md:%d: the erratum means %s §%s, which does not exist", i+1, m[4], m[5]))
		}
	}

	for _, file := range files {
		name := path.Base(file)
		if !strings.HasPrefix(file, "docs/adr/") || !adrFile.MatchString(name) {
			continue
		}
		id := name[:8] // "ADR-" and its four digits
		for i, line := range strings.Split(content(file), "\n") {
			cited := sectionCitation.FindAllStringSubmatch(line, -1)
			cited = append(cited, linkedCitation.FindAllStringSubmatch(line, -1)...)
			for _, m := range cited {
				doc, ok := sections[m[1]]
				if m[1] == "README.md" || (ok && doc.has(m[2])) || errata[id+" "+m[1]+" §"+m[2]] {
					continue
				}
				problems = append(problems, fmt.Sprintf("%s:%d: cites %s §%s, which does not exist - an accepted ADR is not edited; name the section it means in the errata table of docs/adr/README.md", file, i+1, m[1], m[2]))
			}
		}
	}
	return problems
}
