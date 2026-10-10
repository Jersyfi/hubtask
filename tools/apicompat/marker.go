// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"regexp"
	"strings"
)

var (
	// bangTitle is a Conventional Commit title marked as breaking: `type(scope)!: subject`.
	bangTitle = regexp.MustCompile(`^[a-z]+(\([^)\n]*\))?!: \S`)
	// breakingFooter is the footer that says what breaks. Case matters: the template's own
	// "**Breaking change:** yes / no" line is a question, not a marker.
	breakingFooter = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE: \S`)
)

// markedBy answers what marks the change as breaking, or "" when nothing does: a commit message
// whose title carries `!` or whose body carries the footer, or the pull request's title and
// description the same way - its title becomes the squash commit's on main.
func markedBy(messages []string, title, body string) string {
	for _, m := range messages {
		first, _, _ := strings.Cut(m, "\n")
		if bangTitle.MatchString(first) || breakingFooter.MatchString(m) {
			return "the commit " + quote(first)
		}
	}
	if bangTitle.MatchString(strings.TrimSpace(title)) || breakingFooter.MatchString(body) {
		return "the pull request " + quote(strings.TrimSpace(title))
	}
	return ""
}

func quote(s string) string { return "\"" + s + "\"" }

// added is what the branch adds: the breaks against the release that the base did not have yet.
// oasdiff's fingerprint names a change by its rule, operation, path and text, not by the file it
// was read from, so the same break compared from two revisions has one fingerprint.
func added(now, before []change) []change {
	seen := make(map[string]bool, len(before))
	for _, c := range before {
		seen[c.Fingerprint] = true
	}
	var out []change
	for _, c := range now {
		if !seen[c.Fingerprint] {
			out = append(out, c)
		}
	}
	return out
}
