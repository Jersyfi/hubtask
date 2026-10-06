// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
)

const agentsSample = "# Working on Hubtask\n\n" +
	"## Rules that do not bend\n\n| # | Rule | Checked by |\n|---|---|---|\n" +
	"| 1 | Inwards. | `[gate: gate-architecture]` |\n| 2 | Auth. | `[partial: gate-security; open: an adapter]` |\n\n" +
	"## Working rules\n\n- A draft first.\n  `[partial: ci:ci-required; open: a skipped run]`\n- Merge on the word. `[unchecked: one identity]`\n\n" +
	"## Working with the owner\n\n- Fresh session. `[owner]`\n\n" +
	"## Code comments\n\n- Stable references. `[gate: gate-docs]`\n"

func TestRuleTags(t *testing.T) {
	targets := map[string]bool{"gate-architecture": true, "gate-security": true, "gate-docs": true}
	jobs := map[string]bool{"ci-required": true}

	if p := ruleTagProblems(agentsSample, targets, jobs); len(p) != 0 {
		t.Fatalf("a well-tagged file was refused: %v", p)
	}
	cases := []struct{ name, from, to, want string }{
		{"an untagged rule", " `[owner]`", "", "says nothing about what checks it"},
		{"a gate that does not exist", "`[gate: gate-docs]`", "`[gate: gate-nothing]`", "Makefile does not have"},
		{"a CI job that does not exist", "ci:ci-required", "ci:no-such-job", "ci.yml does not have"},
		{"partial without what stays open", "; open: an adapter", "", "does not say what stays open"},
		{"unchecked without a reason", "`[unchecked: one identity]`", "`[unchecked]`", "without saying why"},
		{"a section gone", "## Code comments", "## Comments", "is missing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := strings.Replace(agentsSample, c.from, c.to, 1)
			problems := strings.Join(ruleTagProblems(doc, targets, jobs), "\n")
			if !strings.Contains(problems, c.want) {
				t.Errorf("want a problem containing %q, got:\n%s", c.want, problems)
			}
		})
	}
}

func TestADRRuleLine(t *testing.T) {
	if !hasRuleLine("# ADR-0001\n\n**Status:** accepted\n\n**Rule lives in:** [x](y) §1\n") {
		t.Error("the line under the status was not found")
	}
	if hasRuleLine("# ADR-0001\n\n**Status:** accepted\n\n## Context\n") {
		t.Error("an ADR without the line passed")
	}
}
