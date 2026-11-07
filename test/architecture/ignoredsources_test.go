// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os/exec"
	"strings"
	"testing"
)

// A .gitignore pattern meant for tool output must not swallow source: on a case-insensitive file
// system `coverage.*` also matched Coverage.go, the file stayed untracked, and a clean checkout
// did not compile. ignorecase is forced so the check judges a pattern alike on every machine.
func TestNoSourceFileIsIgnored(t *testing.T) {
	git := func(args ...string) []string {
		out, err := exec.CommandContext(t.Context(), "git", append([]string{"-C", "../..", "-c", "core.ignorecase=true"}, args...)...).Output()
		if err != nil {
			t.Skipf("no git here: %v", err)
		}
		return strings.Fields(string(out))
	}
	tracked := git("ls-files", "--cached", "--ignored", "--exclude-standard")
	untracked := git("ls-files", "--others", "--ignored", "--exclude-standard", "--", "*.go", "*.sql", "**/testdata/**")
	for _, path := range append(tracked, untracked...) {
		t.Errorf("%s is matched by a .gitignore pattern; narrow the pattern so source stays tracked", path)
	}
}
