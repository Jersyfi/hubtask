// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"regexp"
	"strings"
)

// An ADR records why and when a decision was taken; the rule itself lives in a subject document
// (docs/adr/README.md). Once it is no longer proposed its text is the record and stays as it was:
// a change of mind is a new ADR plus the change in the subject document. What may still move is
// what points elsewhere - the status line (accepted, superseded by …), the `Rule lives in` line,
// and where a link leads when the document it names moves. The ADRs that carry addenda from
// before the rule keep them; this reads only what a pull request changes.

// adrSince is when the rule took effect.
var adrSince = readinessSince

var (
	adrFile      = regexp.MustCompile(`^docs/adr/ADR-\d{4}-[a-z0-9-]+\.md$`)
	statusLine   = regexp.MustCompile(`(?m)^\*\*Status:\*\*\s*([A-Za-z]+).*$`)
	ruleLine     = regexp.MustCompile(`(?m)^\*\*Rule lives in:\*\*.*(?:\n[^\n*#][^\n]*)*`)
	linkTarget   = regexp.MustCompile(`\]\([^)]*\)`)
	proposedOnly = "proposed"
)

// adrRecord is what of an ADR may not change once it is settled.
func adrRecord(text string) string {
	text = statusLine.ReplaceAllString(text, "")
	text = ruleLine.ReplaceAllString(text, "")
	text = linkTarget.ReplaceAllString(text, "]()")
	return normalised(text)
}

// adrStatus is the first word of the status line: proposed, accepted, superseded, …
func adrStatus(text string) string {
	m := statusLine.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1])
}

// settledADRChanges names the ADRs the branch changes or deletes whose status at the merge base
// was past proposed, and whose record - the text without the status line, the Rule lives in line
// and link targets - differs.
func settledADRChanges(root, base, head, nameStatus string) ([]string, error) {
	var out []string
	mergeBase := ""
	for _, line := range strings.Split(strings.TrimSpace(nameStatus), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || (fields[0] != "M" && fields[0] != "D") || !adrFile.MatchString(fields[1]) {
			continue
		}
		if mergeBase == "" {
			found, err := git(root, "merge-base", base, head)
			if err != nil {
				return nil, err
			}
			mergeBase = strings.TrimSpace(found)
		}
		before, err := git(root, "show", mergeBase+":"+fields[1])
		if err != nil {
			return nil, err
		}
		if status := adrStatus(before); status == "" || status == proposedOnly {
			continue
		}
		after := ""
		if fields[0] == "M" {
			if after, err = git(root, "show", head+":"+fields[1]); err != nil {
				return nil, err
			}
		}
		if fields[0] == "D" || adrRecord(before) != adrRecord(after) {
			out = append(out, fields[1])
		}
	}
	return out, nil
}

func adrProblems(edited []string) []string {
	var problems []string
	for _, path := range edited {
		problems = append(problems, fmt.Sprintf("%s is settled and this branch changes its text - an ADR is a record: a change of mind is a new ADR that supersedes it, and the rule changes in its subject document; only the status line, the Rule lives in line and link targets move", path))
	}
	return problems
}
