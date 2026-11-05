// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"regexp"
	"strings"
	"testing"
)

// The catalogue's rows, read without a database: PG-7 proves every table with personal content
// has a row, and this proves each row says what a row has to say (UC-PRV-08 checks 1 and 3).
func TestEveryCatalogueRowSaysItsClassificationPurposeRetentionAndDeletionPath(t *testing.T) {
	classes := map[string]bool{
		"NON_PERSONAL": true, "PERSONAL_BASIC": true, "PERSONAL_CONTENT": true,
		"PERSONAL_TECHNICAL": true, "SPECIAL_CATEGORY_RISK": true, "SECRET": true,
	}
	// A backticked word in the classification cell that is a class's spelling, as opposed to a
	// column name the cell explains.
	token := regexp.MustCompile("`([A-Z][A-Z_]+)`")

	rows := 0
	var header []string
	for _, line := range strings.Split(readFile(t, "../../docs/privacy/data-catalog.md"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			header = nil
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		switch {
		case cells[0] == "Data category":
			header = cells
			continue
		case header == nil || strings.HasPrefix(cells[0], "---"):
			continue
		}
		if len(cells) != len(header) {
			t.Errorf("%q has %d cells under a header of %d", cells[0], len(cells), len(header))
			continue
		}
		rows++
		for i, name := range header {
			if cells[i] == "" {
				t.Errorf("%q says nothing under %q", cells[0], name)
			}
			if name != "Classification" {
				continue
			}
			named := 0
			for _, m := range token.FindAllStringSubmatch(cells[i], -1) {
				if classes[m[1]] {
					named++
				}
			}
			if named == 0 && cells[i] != "Every class" {
				t.Errorf("%q is classified as %q, none of the six classes", cells[0], cells[i])
			}
		}
	}
	if rows < 50 {
		t.Fatalf("read %d rows - the catalogue's shape has changed", rows)
	}
}

// UC-PRV-08 check 4: the catalogue says plainly that it describes the software and not an
// operator's record, and names the backups, the logs and the infrastructure as the operator's.
func TestTheCatalogueSaysWhatIsTheOperators(t *testing.T) {
	content := readFile(t, "../../docs/privacy/data-catalog.md")
	for _, said := range []string{
		"This document describes the software. It is **not** an operator's record of processing activities",
		"| Operational logs | stdout / the operator's aggregator |",
		"| Backups | The operator's infrastructure |",
	} {
		if !strings.Contains(content, said) {
			t.Errorf("the catalogue no longer says %q", said)
		}
	}
}
