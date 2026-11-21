// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/tools/losttests"
)

// A test the branch takes away is named in the description, where the reviewer and the owner read
// it with its reason (engineering-guidelines.md §3.20). What counts as taken away is the reading
// of tools/losttests, which the flow report shares.

// testsSince is when the rule took effect.
var testsSince = time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)

// lostTests reads the diff of the test files between the merge base and head.
func lostTests(root, base, head string) ([]losttests.Test, error) {
	found, err := git(root, "merge-base", base, head)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	return losttests.Between(ctx, root, strings.TrimSpace(found), head)
}

func lostTestProblems(body string, lost []losttests.Test) []string {
	body = htmlComment.ReplaceAllString(body, "")
	const reasons = "and say which applies: the behaviour it asserts was removed or changed on purpose, or another test asserts the same thing (engineering-guidelines.md §3.20)"
	var problems []string
	for _, l := range lost {
		if l.NamedIn(body) {
			continue
		}
		switch l.How {
		case "deleted":
			problems = append(problems, fmt.Sprintf("the test file %s is deleted - name %q in the description %s", l.Path, l.Name, reasons))
		case "skipped":
			problems = append(problems, fmt.Sprintf("%s adds a skip to %q - name it in the description %s", l.Path, l.Name, reasons))
		default:
			problems = append(problems, fmt.Sprintf("%s no longer has the test %q - name it in the description %s", l.Path, l.Name, reasons))
		}
	}
	return problems
}
