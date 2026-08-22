// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// arc42 §9 repeats the decision list that docs/adr/README.md owns. A repetition nobody checks
// drifts, and this one did: it stood at 0023 while the index had reached 0026, so three accepted
// decisions were invisible to anybody who read the architecture document rather than the folder.
//
// The two tables are deliberately not required to agree on the title - arc42 abbreviates, and a
// summary that has to match a full title word for word is a summary nobody may improve. What has
// to agree is which decisions exist and what state each one is in, because those are the two
// things a reader takes away from either table.

// indexRow matches a row of the table in docs/adr/README.md: | [0027](./ADR-...) | Title | accepted | … |
var indexRow = regexp.MustCompile(`(?m)^\|\s*\[(\d{4})\]\([^)]*\)\s*\|[^|]*\|\s*([A-Za-z]+)\s*\|`)

// arc42Row matches a row of the table in arc42 §9: | 0027 | Title | accepted |
var arc42Row = regexp.MustCompile(`(?m)^\|\s*(\d{4})\s*\|[^|]*\|\s*([A-Za-z]+)\s*\|`)

// checkArc42ADRTable reconciles arc42 §9 with the ADR index by number and by status.
func checkArc42ADRTable(root string) []string {
	const document = "docs/architecture/arc42.md"

	index := map[string]string{}
	for _, match := range indexRow.FindAllStringSubmatch(read(root, filepath.Join("docs", "adr", "README.md")), -1) {
		index[match[1]] = strings.ToLower(match[2])
	}
	if len(index) == 0 {
		return []string{"docs/adr/README.md: no ADR rows found - has the index table changed shape?"}
	}

	listed := map[string]string{}
	for _, match := range arc42Row.FindAllStringSubmatch(read(root, document), -1) {
		listed[match[1]] = strings.ToLower(match[2])
	}

	var problems []string
	for number, status := range index {
		switch listed[number] {
		case "":
			problems = append(problems, fmt.Sprintf("%s: §9 does not list ADR-%s", document, number))
		case status:
		default:
			problems = append(problems, fmt.Sprintf(
				"%s: §9 calls ADR-%s %q, the index calls it %q", document, number, listed[number], status))
		}
	}
	for number := range listed {
		if _, exists := index[number]; !exists {
			problems = append(problems, fmt.Sprintf("%s: §9 lists ADR-%s, which is not in the index", document, number))
		}
	}

	sort.Strings(problems)
	return problems
}
