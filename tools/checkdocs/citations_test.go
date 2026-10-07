// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
)

func TestCitations(t *testing.T) {
	sections := map[string]*docSections{
		"security.md": {headings: map[string]bool{"9": true, "11": true, "5": true, "5.1": true}, items: map[string]bool{"5.3": true}},
	}
	cases := []struct {
		name, file, line, want string
	}{
		{"a section that exists", "core/x.go", "// (security.md §9)", ""},
		{"a numbered item inside a section", "core/x.go", "// (security.md §5.3)", ""},
		{"a section whose first subsection exists", "core/x.go", "// security.md §5", ""},
		{"a section that does not exist", "core/x.go", "// (security.md §14)", "does not have"},
		{"a document that does not exist", "core/x.go", "// (gone.md §1)", "no document under docs/"},
		{"a task", "core/x.go", "// added in SC-24", "cites the task SC-24"},
		{"a task in test data", "core/x_test.go", `name := "SC-33 A"`, ""},
		{"an issue", "core/x.go", "// see #1119", "issue or pull request"},
		{"an issue written out", "core/x.go", "// issue 818 found it", "issue or pull request"},
		{"a milestone file", "core/x.go", "// milestone-F8.md decision 5", "a milestone's decisions"},
		{"an instruction file", "core/x.go", "// rule 7 of AGENTS.md", "cite the rule"},
		{"a rule by number", "core/x.go", "// (rule 7, ADR-0016)", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(citationProblems(c.file, c.line, sections), "\n")
			if c.want == "" && got != "" {
				t.Fatalf("want nothing, got %s", got)
			}
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Fatalf("want %q, got %q", c.want, got)
			}
		})
	}
}

func TestPublicText(t *testing.T) {
	if p := publicTextProblems("info:\n  # ADR-0080 is cited in a comment\n  description: A task."); len(p) != 0 {
		t.Fatalf("a comment line was read as public text: %v", p)
	}
	for _, line := range []string{"description: see ADR-0045", "description: see security.md", "description: as §3 says", "description: built in F8-12"} {
		if len(publicTextProblems(line)) == 0 {
			t.Errorf("%q passed", line)
		}
	}
}
