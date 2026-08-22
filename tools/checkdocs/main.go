// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command checkdocs is the documentation gate (make gate-docs).
//
// The documents in this repository are load-bearing: the ADRs are what the code cites instead of
// repeating an argument, and CLAUDE.md sends a reader through them in a fixed order. A link that
// stops resolving therefore does not only annoy - it quietly removes the reason a piece of code
// looks the way it does.
//
// What it checks, and why each one has been wrong somewhere before:
//
//   - Every relative link between documents resolves, including its anchor. A renamed heading
//     leaves a link that lands at the top of a long page, which reads as "the section is gone".
//   - Every ADR is in the index, and every index entry has a file. An ADR nobody indexed is one
//     nobody finds; an index entry without a file is a decision that looks recorded and is not.
//   - Every ADR-xxxx named anywhere in the repository exists. Code cites ADR numbers in comments,
//     and a typo there points a reader at nothing.
//   - arc42 §9 lists every ADR the index lists, with the same status. It repeats the decision list,
//     and a repetition nobody checks drifts - this one stood three decisions behind.
//   - Every statement of the Go version agrees with go.mod. It is repeated in seventeen places
//     across eight files, and a base image bumped on its own would have the release built by a
//     compiler no gate ever ran.
//   - Every document CLAUDE.md's reading order names exists, because that list is what a new
//     session is told to read.
//   - The support matrix and the workflows agree in both directions, so that support can neither
//     be claimed without a job nor removed by deleting one (see matrix.go).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func main() {
	root, err := repositoryRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	docs, err := markdownFiles(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var problems []string
	problems = append(problems, checkLinks(root, docs)...)
	problems = append(problems, checkADRIndex(root)...)
	problems = append(problems, checkADRReferences(root)...)
	problems = append(problems, checkArc42ADRTable(root)...)
	problems = append(problems, checkGoVersion(root)...)
	problems = append(problems, checkSupportMatrix(root)...)

	if len(problems) > 0 {
		sort.Strings(problems)
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, p)
		}
		fmt.Fprintf(os.Stderr, "\n%d problem(s) in the documentation\n", len(problems))
		os.Exit(1)
	}
	fmt.Printf("docs: %d files, links and ADR index consistent\n", len(docs))
}

// link matches a markdown link target. Reference-style links and bare URLs are deliberately not
// matched: what this gate is about is one document pointing at another.
var link = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)

// heading matches an ATX heading, which is what an anchor is derived from.
var heading = regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*$`)

// adrFile matches the ADR file naming convention, docs/adr/ADR-0001-something.md.
var adrFile = regexp.MustCompile(`^ADR-(\d{4})-[a-z0-9-]+\.md$`)

// adrReference matches a citation anywhere in the repository: ADR-0007.
var adrReference = regexp.MustCompile(`ADR-(\d{4})`)

// checkLinks resolves every relative link, and its anchor when it has one.
func checkLinks(root string, docs []string) []string {
	anchors := map[string]map[string]bool{}
	for _, doc := range docs {
		anchors[doc] = anchorsOf(read(root, doc))
	}

	var problems []string
	for _, doc := range docs {
		dir := filepath.Dir(doc)
		for _, match := range link.FindAllStringSubmatch(read(root, doc), -1) {
			target := match[1]
			switch {
			case strings.HasPrefix(target, "http://"), strings.HasPrefix(target, "https://"),
				strings.HasPrefix(target, "mailto:"):
				continue
			case strings.HasPrefix(target, "#"):
				// A link inside the same document.
				if !anchors[doc][strings.TrimPrefix(target, "#")] {
					problems = append(problems, fmt.Sprintf("%s: no heading for the anchor %s", doc, target))
				}
				continue
			}

			path, anchor, _ := strings.Cut(target, "#")
			if path == "" {
				continue
			}
			resolved := filepath.Clean(filepath.Join(dir, path))
			if _, err := os.Stat(filepath.Join(root, resolved)); err != nil {
				problems = append(problems, fmt.Sprintf("%s: the link %s points at nothing", doc, target))
				continue
			}
			if anchor == "" || !strings.HasSuffix(resolved, ".md") {
				continue
			}
			known, read := anchors[resolved]
			if !read {
				continue
			}
			if !known[anchor] {
				problems = append(problems, fmt.Sprintf("%s: %s has no heading for the anchor #%s", doc, resolved, anchor))
			}
		}
	}
	return problems
}

// anchorsOf derives the anchors a document offers from its headings, the way GitHub does: lower
// case, spaces to hyphens, and everything that is not a letter, a digit or a hyphen removed.
func anchorsOf(content string) map[string]bool {
	anchors := map[string]bool{}
	seen := map[string]int{}
	for _, match := range heading.FindAllStringSubmatch(content, -1) {
		slug := slugify(match[1])
		if slug == "" {
			continue
		}
		// A repeated heading gets -1, -2 and so on, and both spellings have to resolve.
		if n := seen[slug]; n > 0 {
			anchors[fmt.Sprintf("%s-%d", slug, n)] = true
		}
		seen[slug]++
		anchors[slug] = true
	}
	return anchors
}

func slugify(headingText string) string {
	// Inline markup is not part of the anchor: `code`, **bold** and links contribute their text.
	text := strings.NewReplacer("`", "", "*", "", "_", "").Replace(headingText)
	text = link.ReplaceAllString(text, "")
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		case r > 127:
			// A letter with a diacritic keeps its place in the anchor; GitHub keeps it too.
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-")
}

// checkADRIndex reconciles the files under docs/adr with the table in docs/adr/README.md.
func checkADRIndex(root string) []string {
	dir := filepath.Join(root, "docs", "adr")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{fmt.Sprintf("docs/adr is not readable: %v", err)}
	}

	files := map[string]string{}
	for _, entry := range entries {
		if match := adrFile.FindStringSubmatch(entry.Name()); match != nil {
			files[match[1]] = entry.Name()
		}
	}

	index := read(root, filepath.Join("docs", "adr", "README.md"))
	var problems []string
	indexed := map[string]bool{}
	for _, match := range link.FindAllStringSubmatch(index, -1) {
		name := strings.TrimPrefix(match[1], "./")
		adr := adrFile.FindStringSubmatch(name)
		if adr == nil {
			continue
		}
		indexed[adr[1]] = true
		if _, exists := files[adr[1]]; !exists {
			problems = append(problems, fmt.Sprintf("docs/adr/README.md: the index lists %s, which does not exist", name))
		}
	}

	for number, name := range files {
		if !indexed[number] {
			problems = append(problems, fmt.Sprintf("docs/adr/README.md: %s is not in the index", name))
		}
	}
	return problems
}

// checkADRReferences makes sure every ADR cited anywhere resolves. Code comments cite ADR numbers
// instead of repeating the argument, and a typo there points a reader at nothing.
func checkADRReferences(root string) []string {
	dir := filepath.Join(root, "docs", "adr")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{fmt.Sprintf("docs/adr is not readable: %v", err)}
	}
	known := map[string]bool{}
	for _, entry := range entries {
		if match := adrFile.FindStringSubmatch(entry.Name()); match != nil {
			known[match[1]] = true
		}
	}

	cited := map[string][]string{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if skipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(entry.Name()) {
		case ".go", ".md", ".sql", ".yaml", ".yml", ".tpl":
		default:
			return nil
		}
		content, readErr := os.ReadFile(path) //nolint:gosec // G304: walking this repository is the job
		if readErr != nil {
			return readErr
		}
		relative, _ := filepath.Rel(root, path)
		for _, match := range adrReference.FindAllStringSubmatch(string(content), -1) {
			cited[match[1]] = append(cited[match[1]], relative)
		}
		return nil
	})
	if err != nil {
		return []string{fmt.Sprintf("walking the repository: %v", err)}
	}

	var problems []string
	for number, files := range cited {
		if known[number] {
			continue
		}
		sort.Strings(files)
		problems = append(problems, fmt.Sprintf("ADR-%s is cited in %s and does not exist", number, strings.Join(unique(files), ", ")))
	}
	return problems
}

// markdownFiles collects every document, repository-relative.
func markdownFiles(root string) ([]string, error) {
	var docs []string
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
		if strings.HasSuffix(entry.Name(), ".md") {
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			docs = append(docs, relative)
		}
		return nil
	})
	sort.Strings(docs)
	return docs, err
}

// skipDir keeps the walk out of everything that is not written by hand.
func skipDir(name string) bool {
	switch name {
	case ".git", ".tools", "bin", "dist", "node_modules", "vendor":
		return true
	}
	return false
}

func read(root, relative string) string {
	content, err := os.ReadFile(filepath.Join(root, relative)) //nolint:gosec // G304: a path from the walk above
	if err != nil {
		return ""
	}
	return string(content)
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := values[:0]
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// repositoryRoot finds go.mod upwards, so the gate can be run from anywhere.
func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}
