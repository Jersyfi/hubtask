// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"regexp"
	"strings"
)

// A pull request's title becomes the squash commit's title, and release.yml builds the changelog
// from those: a title that is not a Conventional Commit is a release note in the wrong section or
// in none (versioning-release.md §3).

// titleSince is when the rule took effect.
var titleSince = readinessSince

var (
	conventionalTitle = regexp.MustCompile(`^(feat|fix|perf|refactor|docs|test|build|ci|chore|revert)(\([a-z0-9][a-z0-9 ,./_-]*\))?!?: \S`)
	// githubRevert is the title GitHub's own revert button writes; it names the reverted title.
	githubRevert = regexp.MustCompile(`^Revert ".+"$`)
)

func titleProblems(title string) []string {
	title = strings.TrimSpace(title)
	if conventionalTitle.MatchString(title) || githubRevert.MatchString(title) {
		return nil
	}
	return []string{fmt.Sprintf("the title %q is not a Conventional Commit - `type(scope): subject`, `!` before the colon for a breaking change, type one of feat, fix, perf, refactor, docs, test, build, ci, chore, revert (versioning-release.md §3)", title)}
}
