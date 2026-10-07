// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"regexp"
	"strings"
)

// A use case's Goal, How to check and Where it ends change only by the owner's decision, or as a
// correction of a wrong reference or number - what a person relies on is the owner's to change. The
// gate cannot tell which one a change is, and it cannot see the owner's word; what it holds is that
// the description says which - so the change is visible to whoever reads the pull request, instead
// of arriving as one more line in a large diff.

// ucTextSince is when the rule took effect.
var ucTextSince = readinessSince

var ucTextNamed = regexp.MustCompile(`(?i)\bcorrection\b|\bdecision #\d+`)

func ucTextProblems(body string, changes []ucTextChange) []string {
	section := sectionText(htmlComment.ReplaceAllString(body, ""), "Use cases")
	var problems []string
	for _, c := range changes {
		if c.added {
			continue
		}
		named := false
		for _, line := range strings.Split(section, "\n") {
			if strings.Contains(line, c.id) && ucTextNamed.MatchString(line) {
				named = true
				break
			}
		}
		if !named {
			problems = append(problems, fmt.Sprintf("%s changes the Goal, How to check or Where it ends of %s - say in Use cases on a line naming %s whether it is a `correction` (a wrong reference or number) or `decision #<issue>`", c.path, c.id, c.id))
		}
	}
	return problems
}
