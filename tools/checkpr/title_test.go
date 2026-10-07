// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import "testing"

func TestTitle(t *testing.T) {
	for _, title := range []string{
		"feat(admin): an operator opens the password for one workspace",
		"docs: one place per kind of knowledge, AGENTS.md, readiness before code",
		"feat(api)!: remove the deprecated endpoint",
		"fix(webapp/shell): the menu closes on Escape",
		"build(deps): bump the actions group",
		`Revert "feat(admin): an operator opens the password"`,
	} {
		if p := titleProblems(title); len(p) != 0 {
			t.Errorf("%q was refused: %v", title, p)
		}
	}
	for _, title := range []string{
		"",
		"An operator opens the password",
		"feature(admin): a type that does not exist",
		"Feat: capitalised",
		"feat(Admin): a scope in capitals",
		"feat:no space",
		"feat(admin) : space before the colon",
		"WIP",
	} {
		if len(titleProblems(title)) == 0 {
			t.Errorf("%q passed", title)
		}
	}
}
