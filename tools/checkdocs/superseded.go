// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// supersededStatus is an ADR's status line once a later one replaced it.
var supersededStatus = regexp.MustCompile(`(?m)^\*\*Status:\*\*\s*superseded`)

// checkSupersededADRCitations flags a subject document that links a superseded ADR as the reason
// for a rule it states today. A paragraph or a table row that names the ADR as history ("supersed…"
// or "retired" anywhere in it) passes; one that does not reads as the current
// rule, and a reader acting on it acts on a decision that no longer holds.
func checkSupersededADRCitations(root string) []string {
	superseded := map[string]bool{}
	adrs, err := os.ReadDir(filepath.Join(root, "docs", "adr"))
	if err != nil {
		return []string{fmt.Sprintf("docs/adr is not readable: %v", err)}
	}
	for _, entry := range adrs {
		if adrFile.MatchString(entry.Name()) &&
			supersededStatus.MatchString(read(root, filepath.Join("docs", "adr", entry.Name()))) {
			superseded[entry.Name()] = true
		}
	}

	subjects, _ := filepath.Glob(filepath.Join(root, "docs", "architecture", "*.md"))
	sort.Strings(subjects)
	var problems []string
	for _, path := range subjects {
		relative, _ := filepath.Rel(root, path)
		relative = filepath.ToSlash(relative)
		for _, block := range blocksOf(read(root, relative)) {
			lower := strings.ToLower(block.text)
			if strings.Contains(lower, "supersed") || strings.Contains(lower, "retired") {
				continue
			}
			for _, match := range link.FindAllStringSubmatch(block.text, -1) {
				name := filepath.Base(strings.SplitN(match[1], "#", 2)[0])
				if superseded[name] {
					problems = append(problems, fmt.Sprintf(
						"%s:%d cites %s, which is superseded, as a current rule: link the ADR that replaced it, or say it was superseded",
						relative, block.line, strings.TrimSuffix(name, ".md")))
				}
			}
		}
	}
	return problems
}

type block struct {
	line int
	text string
}

// blocksOf splits a document into paragraphs and table rows, each with its first line's number.
func blocksOf(content string) []block {
	var blocks []block
	open := false
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			open = false
		case strings.HasPrefix(trimmed, "|"):
			blocks = append(blocks, block{line: i + 1, text: line})
			open = false
		case !open:
			blocks = append(blocks, block{line: i + 1, text: line})
			open = true
		default:
			blocks[len(blocks)-1].text += "\n" + line
		}
	}
	return blocks
}
