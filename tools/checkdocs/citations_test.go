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
		{"an issue in TypeScript", "apps/webapp/src/x.ts", "// see #1119", "issue or pull request"},
		{"an issue in Svelte", "apps/webapp/src/X.svelte", "<!-- (#1119) -->", "issue or pull request"},
		{"a colour in a style block", "apps/webapp/src/X.svelte", "    color: #359;", ""},
		{"a colour as an attribute", "apps/webapp/src/X.svelte", `<path fill="#000" />`, ""},
		{"a colour as a JSON value", "packages/design-system/tokens/tokens.json", `"ink": "#1234",`, ""},
		{"a task with a single letter", "core/x.go", "// measured in G-02", "cites the task G-02"},
		{"an alert, a threat, a principle", "core/x.go", "// A-14, T-07, P-05, C-03, R-09", ""},
		{"a single letter that is no task", "core/x.go", "// option B-99", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(citationProblems(c.file, c.line, sections, map[string]bool{"G-02": true}), "\n")
			if c.want == "" && got != "" {
				t.Fatalf("want nothing, got %s", got)
			}
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Fatalf("want %q, got %q", c.want, got)
			}
		})
	}
}

func TestADRCitations(t *testing.T) {
	sections := map[string]*docSections{
		"versioning-release.md": {headings: map[string]bool{"1": true, "2": true}, items: map[string]bool{}},
	}
	const adr = "docs/adr/ADR-0025-precondition-failures.md"
	cases := []struct {
		name, readme, line, want string
	}{
		{"a section that exists", "", "versioning-release.md §2 lists it", ""},
		{"a section that does not exist", "", "versioning-release.md §8 lists it", "cites versioning-release.md §8"},
		{"one through a link", "", "[versioning-release.md](../architecture/versioning-release.md) §8", "cites versioning-release.md §8"},
		{"one the errata table names", "| ADR-0025 | versioning-release.md §8 | versioning-release.md §2 |", "versioning-release.md §8", ""},
		{"an erratum for another ADR", "| ADR-0004 | versioning-release.md §8 | versioning-release.md §2 |", "versioning-release.md §8", "cites versioning-release.md §8"},
		{"an erratum meaning a section that does not exist", "| ADR-0025 | versioning-release.md §8 | versioning-release.md §9 |", "versioning-release.md §8", "the erratum means versioning-release.md §9"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			content := map[string]string{"docs/adr/README.md": c.readme, adr: c.line}
			got := strings.Join(adrCitationProblems([]string{"docs/adr/README.md", adr}, func(f string) string { return content[f] }, sections), "\n")
			if c.want == "" && got != "" {
				t.Fatalf("want nothing, got %s", got)
			}
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Fatalf("want %q, got %q", c.want, got)
			}
		})
	}
}

func TestCitationScope(t *testing.T) {
	for file, want := range map[string]bool{
		"core/x.go":                      true,
		"api/events/item.created.json":   true,
		"packages/sync-engine/README.md": true,
		"deploy/observability/runbooks/RB-A14-misconfiguration.md": true,
		"scripts/verify-tenant-export.py":                          true,
		"presentation/webui/dist/index.html":                       true,
		"apps/website/src/site.css":                                true,
		"README.md":                                                false,
		"CONTRIBUTING.md":                                          false,
		"docs/architecture/security.md":                            false,
		".github/PULL_REQUEST_TEMPLATE.md":                         false,
		"core/AGENTS.md":                                           false,
		"locales/en.json":                                          false,
		"api/openapi.json":                                         false,
	} {
		if got := citationScope(file); got != want {
			t.Errorf("%s: in scope %v, want %v", file, got, want)
		}
	}
	if p := citationProblems("packages/x/README.md", "The rules are in [AGENTS.md](./AGENTS.md).", nil, nil); len(p) != 0 {
		t.Errorf("a README pointing to its AGENTS.md was refused: %v", p)
	}
	if p := citationProblems("deploy/x/README.md", "Found in issue #310.", nil, nil); len(p) == 0 {
		t.Error("an issue number in a code document passed")
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
