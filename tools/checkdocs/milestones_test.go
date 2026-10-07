// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
)

const milestoneSample = "# Milestone XX — a sample\n\n" +
	"**Delivers:** UC-PRV-01 (9, 10), UC-LIF-06\n**Released:** 2026-10-02\n\n## Decisions\n\n1. One.\n\n---\n\n" +
	"## XX-01 — The first\n\n**Use cases:** UC-PRV-01 (9)\n\nText.\n\n" +
	"## XX-02 — The second\n\n**Use cases:** UC-PRV-01 (10), UC-LIF-06 - all of it\n\nText.\n\n" +
	"## XX-03 — The walk\n\n**Use cases:** every check in `Delivers` — the walk confirms them.\n"

func sampleUseCases() map[string]ucChecks {
	return map[string]ucChecks{
		"UC-PRV-01": {state: "partial", checks: map[int]bool{1: true, 9: true, 10: true, 11: true}, unmet: map[int]bool{11: true}},
		"UC-LIF-06": {state: "built", checks: map[int]bool{1: true, 2: true}, unmet: map[int]bool{}},
		"UC-ID-16":  {state: "built", checks: map[int]bool{1: true}, unmet: map[int]bool{}},
	}
}

func TestMilestoneDelivers(t *testing.T) {
	known := sampleUseCases()
	if p := milestoneProblems("m.md", milestoneSample, known); len(p) != 0 {
		t.Fatalf("a milestone whose tasks stay inside Delivers was refused: %v", p)
	}
	closed := strings.Replace(milestoneSample, "**Released:** 2026-10-02", "**Released:** 2026-10-02\n**Closed:** 2026-10-20", 1)
	if p := milestoneProblems("m.md", closed, known); len(p) != 0 {
		t.Fatalf("a closed milestone whose checks are all met was refused: %v", p)
	}

	cases := []struct {
		name, doc, want string
		mutate          func(map[string]ucChecks)
	}{
		{"a check outside Delivers", strings.Replace(milestoneSample, "UC-PRV-01 (9)\n", "UC-PRV-01 (9, 11)\n", 1), "carries UC-PRV-01 (11), which Delivers does not name", nil},
		{"a use case outside Delivers", strings.Replace(milestoneSample, "UC-PRV-01 (9)\n", "UC-PRV-01 (9), UC-ID-16 (1)\n", 1), "carries UC-ID-16 (1), which Delivers does not name", nil},
		{"a use case that does not exist", strings.Replace(milestoneSample, "UC-PRV-01 (9)\n", "UC-PRV-01 (9), "+missingUC+"\n", 1), "names " + missingUC + ", which does not exist", nil},
		{"a whole use case Delivers names only partly", strings.Replace(milestoneSample, "UC-PRV-01 (9)\n", "UC-PRV-01\n", 1), "carries UC-PRV-01 (1, 11)", nil},
		{"a task without checks", strings.Replace(milestoneSample, "UC-PRV-01 (9)\n", "none - tooling\n", 1), "XX-01 carries no check from Delivers", nil},
		{"a task without the line", strings.Replace(milestoneSample, "**Use cases:** UC-PRV-01 (9)\n", "", 1), "XX-01 names no use cases", nil},
		{"a check the use case does not have", strings.Replace(milestoneSample, "UC-PRV-01 (9)\n", "UC-PRV-01 (9, 12)\n", 1), "UC-PRV-01 " + "check 12, which UC-PRV-01 does not have", nil},
		{"a range in Delivers", strings.Replace(milestoneSample, "(9, 10), UC-LIF-06\n", "(9–10), UC-LIF-06\n", 1), "", nil},
		{"closed with a check still unmet", closed, "still lists check 10 as not met",
			func(k map[string]ucChecks) { k["UC-PRV-01"].unmet[10] = true }},
		{"closed with a use case still specified", closed, "UC-LIF-06, which it delivers, is still specified",
			func(k map[string]ucChecks) { uc := k["UC-LIF-06"]; uc.state = "specified"; k["UC-LIF-06"] = uc }},
		{"an unmet check is fine while the milestone runs", milestoneSample, "",
			func(k map[string]ucChecks) { k["UC-PRV-01"].unmet[10] = true }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			known := sampleUseCases()
			if c.mutate != nil {
				c.mutate(known)
			}
			got := strings.Join(milestoneProblems("m.md", c.doc, known), "\n")
			if c.want == "" && got != "" {
				t.Fatalf("want nothing, got:\n%s", got)
			}
			if c.want != "" && !strings.Contains(got, c.want) {
				t.Fatalf("want a problem containing %q, got:\n%s", c.want, got)
			}
		})
	}
}

// missingUC is an id no use case carries, assembled so that the gate's own walk does not read it
// as a citation.
var missingUC = "UC-" + "LIF-" + "99"

func TestTodayChecks(t *testing.T) {
	got := todayChecks("\n* Check 4: not met — x\n* Check 11: not met in the web app — y\nprose\n")
	if !got[4] || !got[11] || len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}
