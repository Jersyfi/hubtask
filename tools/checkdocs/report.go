// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/catalogue"
)

// The coverage report half of the gate (docs/evidence/COVERAGE-<date>.md, milestone-F6.md
// decision 16).
//
// The report is one row per use case the catalogue serves, with where a client reaches it or why
// it does not. It is a document rather than a snapshot only if it cannot drift: a use case added
// without a row would be a surface nobody has said anything about, and a row naming a use case
// the catalogue no longer has would be a claim about nothing. Both directions, against the
// catalogue itself - `catalogue.Descriptors()` is what the registry serves, and the report's
// first column is held to exactly that list.
//
// The newest report counts, by its date in the file name: an older one is history and stays.

const evidenceDir = "docs/evidence"

// coverageFile matches the report's name, and captures the date the newest one is picked by.
var coverageFile = regexp.MustCompile(`^COVERAGE-(\d{4}-\d{2}-\d{2})\.md$`)

// coverageRow matches a table row whose first cell is one backticked use case name.
var coverageRow = regexp.MustCompile("(?m)^\\| `([A-Z][A-Za-z]+)` \\|")

func checkCoverageReport(root string) []string {
	matches, err := filepath.Glob(filepath.Join(root, evidenceDir, "COVERAGE-*.md"))
	if err != nil {
		return []string{err.Error()}
	}
	var newest, newestDate string
	for _, match := range matches {
		name := filepath.Base(match)
		if m := coverageFile.FindStringSubmatch(name); m != nil && m[1] > newestDate {
			newest, newestDate = name, m[1]
		}
	}
	if newest == "" {
		return []string{evidenceDir + "/COVERAGE-<date>.md is missing - decision 16 holds the catalogue to it"}
	}
	relative := evidenceDir + "/" + newest
	report := read(root, relative)

	rows := map[string]int{}
	for _, m := range coverageRow.FindAllStringSubmatch(report, -1) {
		rows[m[1]]++
	}

	var problems []string
	served := map[string]bool{}
	for _, descriptor := range catalogue.Descriptors() {
		served[descriptor.Name] = true
		switch rows[descriptor.Name] {
		case 0:
			problems = append(problems, fmt.Sprintf("%s: no row for the use case %s - every use case the catalogue serves has a route or a reason", relative, descriptor.Name))
		case 1:
		default:
			problems = append(problems, fmt.Sprintf("%s: %d rows for the use case %s", relative, rows[descriptor.Name], descriptor.Name))
		}
	}
	names := make([]string, 0, len(rows))
	for name := range rows {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !served[name] {
			problems = append(problems, fmt.Sprintf("%s: the row %s names no use case the catalogue serves", relative, name))
		}
	}
	// An omission has a reason or an issue; "nobody built it" without a number is the one
	// sentence decision 16 refuses.
	for _, line := range strings.Split(report, "\n") {
		if strings.Contains(line, "omitted — nobody built it") && !strings.Contains(line, "| #") {
			problems = append(problems, fmt.Sprintf("%s: an omission that nobody built names no issue: %s", relative, strings.TrimSpace(line)))
		}
	}
	return problems
}
