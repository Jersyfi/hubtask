// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"strings"
	"testing"
)

var agentsSample = "# Working on Hubtask\n\n" +
	"## Rules that do not bend\n\n| # | Rule | Checked by |\n|---|---|---|\n" +
	"| 1 | Inwards. | `[gate: gate-architecture]` |\n| 2 | Auth. | `[partial: gate-security; open: an adapter]` |\n" +
	moreRules(3, 15) + "\n" +
	"## Working rules\n\n- A draft first.\n  `[partial: ci:ci-required; open: a skipped run]`\n- Merge on the word. `[unchecked: one identity]`\n\n" +
	"## Working with the owner\n\n- Fresh session. `[owner]`\n\n" +
	"## The loop for every task\n\n1. **Understand.** Read.\n2. Plan.\n3. Specify.\n4. Build.\n5. Test.\n6. Check.\n7. Finish.\n\n" +
	"### Reading\n\n1. The use cases.\n2. The model.\n\n" +
	"## What you do not decide yourself\n\n- A new dependency.\n\n" +
	"## Which command checks what\n\n| Changed | Run |\n|---|---|\n| Anything | `make verify` |\n\n" +
	"## Code comments\n\n- Stable references. `[gate: gate-docs]`\n"

func moreRules(from, to int) string {
	var b strings.Builder
	for n := from; n <= to; n++ {
		fmt.Fprintf(&b, "| %d | Rule %d. | `[gate: gate-architecture]` |\n", n, n)
	}
	return b.String()
}

func TestRuleTags(t *testing.T) {
	targets := map[string]bool{"gate-architecture": true, "gate-security": true, "gate-docs": true}
	jobs := ciJobs("name: CI\non:\n  push:\n    branches: [main]\n  pull_request:\n\njobs:\n  quick:\n    runs-on: x\n  ci-required:\n    needs: [quick]\n")
	if !jobs["ci-required"] || !jobs["quick"] || jobs["push"] || jobs["pull_request"] {
		t.Fatalf("the jobs read from a workflow: %v", jobs)
	}

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
		{"a cited heading reworded", "## What you do not decide yourself", "## What the owner decides", `the heading "What you do not decide yourself" is gone`},
		{"the command table renamed", "## Which command checks what", "## Commands", `the heading "Which command checks what" is gone`},
		{"a rule renumbered", "| 7 | Rule 7.", "| 8 | Rule 7.", "rule 8 stands where rule 7 belongs"},
		{"a rule removed", "| 15 | Rule 15. | `[gate: gate-architecture]` |\n", "", "14 rules, not 15"},
		{"a loop step removed", "7. Finish.\n", "", "not 1 to 7"},
		{"a loop step added", "7. Finish.\n", "7. Finish.\n8. Celebrate.\n", "not 1 to 7"},
		{"a key under on: is no job", "ci:ci-required", "ci:push", "ci.yml does not have"},
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

func TestNestedTags(t *testing.T) {
	targets := map[string]bool{"gate-architecture": true}
	jobs := map[string]bool{"node": true, "tokens-drift": true}
	doc := "# packages/x\n\n## What must not happen here\n\n" +
		"* **No literal.** The lint fails on one.\n  `[gate: ci:node, ci:tokens-drift]`\n" +
		"* **No go file** (rule 14). `[gate: gate-architecture]`\n" +
		"* **No judgement left out.** `[owner]`\n\n" +
		"**The gates read comments.** A paragraph explains.\n\n" +
		"## How to check a change\n\n* `pnpm test` - explained, not a rule\n"
	if p := nestedTagProblems("packages/x/AGENTS.md", doc, targets, jobs); len(p) != 0 {
		t.Fatalf("a tagged file was refused: %v", p)
	}
	cases := []struct{ name, from, to, want string }{
		{"an untagged rule", " `[owner]`", "", "packages/x/AGENTS.md § What must not happen here: \"* **No judgement left out.**\" says nothing"},
		{"one of two targets missing", "ci:tokens-drift", "ci:no-such-job", "names the CI job ci:no-such-job"},
		{"a gate that does not exist", "gate-architecture]", "gate-nothing]", "names the gate gate-nothing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(nestedTagProblems("packages/x/AGENTS.md", strings.Replace(doc, c.from, c.to, 1), targets, jobs), "\n")
			if !strings.Contains(got, c.want) {
				t.Fatalf("want %q, got:\n%s", c.want, got)
			}
		})
	}
	claims := "## What the site may claim\n\n* Only what is true today.\n"
	if len(nestedTagProblems("apps/website/AGENTS.md", claims, targets, jobs)) != 1 {
		t.Error("an untagged claim rule passed")
	}
}

func TestNormativeWords(t *testing.T) {
	doc := agentsSample + "\n## When CI runs\n\nA draft is checked in the session.\n\n```text\nnever in a code block\n```\n\n" +
		"See § \"What you do not decide yourself\" and `never` as code.\n"
	if p := normativeProblems(doc); len(p) != 0 {
		t.Fatalf("explaining prose, a code block, inline code and a quoted heading were refused: %v", p)
	}
	for _, sentence := range []string{
		"Rework must go back to draft first.",
		"Never push a red gate.",
		"Always run make verify.",
		"Do not run two gates at once.",
		"Don't pipe a gate.",
	} {
		t.Run(sentence, func(t *testing.T) {
			got := strings.Join(normativeProblems(doc+"\n"+sentence+"\n"), "\n")
			if !strings.Contains(got, "outside the rule lists") {
				t.Fatalf("%q passed", sentence)
			}
		})
	}
	t.Run("prose under a rule list", func(t *testing.T) {
		withProse := strings.Replace(doc, "## Working rules\n\n", "## Working rules\n\nYou must read this.\n\n", 1)
		if len(normativeProblems(withProse)) == 0 {
			t.Fatal("a commanding paragraph in a rule section that is no item passed")
		}
	})
	t.Run("a rule item", func(t *testing.T) {
		withRule := strings.Replace(doc, "- Merge on the word.", "- Never merge without the word.", 1)
		if p := normativeProblems(withRule); len(p) != 0 {
			t.Fatalf("a tagged rule item was refused: %v", p)
		}
	})
}

func TestADRRuleLine(t *testing.T) {
	if !hasRuleLine("# ADR-0001\n\n**Status:** accepted\n\n**Rule lives in:** [x](y) §1\n") {
		t.Error("the line under the status was not found")
	}
	if hasRuleLine("# ADR-0001\n\n**Status:** accepted\n\n## Context\n") {
		t.Error("an ADR without the line passed")
	}
}
