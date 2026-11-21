// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"strings"
	"time"
)

// The baseline of the browser walks' answers outside the contract only shrinks (ci-cd.md §3.2,
// `engines`): the test that reads it refuses an answer it does not hold, and this rule refuses
// the other way out - writing the answer into the baseline.
//
// Open: a suite renamed carries its entries to a new name, which reads as added; its answers are
// then fixed with the rename.

// walkBaselinePath is the baseline test/contract reads.
const walkBaselinePath = "test/contract/testdata/walks-baseline.txt"

// walkBaselineSince is when the rule took effect.
var walkBaselineSince = time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)

// baselineEntries are a baseline's lines that are neither empty nor a comment.
func baselineEntries(text string) map[string]bool {
	entries := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			entries[line] = true
		}
	}
	return entries
}

// addedBaselineEntries are the entries of after that before does not hold, in after's order.
func addedBaselineEntries(before, after string) []string {
	had := baselineEntries(before)
	var added []string
	for _, line := range strings.Split(after, "\n") {
		if line = strings.TrimSpace(line); baselineEntries(line)[line] && !had[line] {
			added = append(added, line)
		}
	}
	return added
}

// walkBaselineAdded reads the baseline at the merge base and at the head, where the branch changes
// it. A branch that creates the file adds nothing to a baseline: there was none.
func walkBaselineAdded(root, base, head, nameStatus string) ([]string, error) {
	for _, line := range strings.Split(strings.TrimSpace(nameStatus), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "M" || fields[1] != walkBaselinePath {
			continue
		}
		mergeBase, err := git(root, "merge-base", base, head)
		if err != nil {
			return nil, err
		}
		before, err := git(root, "show", strings.TrimSpace(mergeBase)+":"+walkBaselinePath)
		if err != nil {
			return nil, err
		}
		after, err := git(root, "show", head+":"+walkBaselinePath)
		if err != nil {
			return nil, err
		}
		return addedBaselineEntries(before, after), nil
	}
	return nil, nil
}

func walkBaselineProblems(added []string) []string {
	if len(added) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("%s gains %d entries (the first: %q) - an answer of a walk that the contract does not allow is corrected in the walk or its fixture, never added to the baseline", walkBaselinePath, len(added), added[0])}
}
