// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The `actions-allowed` job in ci.yml asks GitHub's job set-up whether the repository setting
// allows every action the workflows use (ci-cd.md §4). It answers that only for the `uses:` it
// names, so it has to name exactly the others: one missing is an action whose refusal waits for
// the release or the nightly that uses it, one extra is a pin nothing runs. The pin is part of
// the comparison - set-up resolves a commit, and a bump applied to one side would probe another.

const probeJob = "actions-allowed"

type probeWorkflow struct {
	Jobs map[string]struct {
		Steps []struct {
			Uses string `yaml:"uses"`
			Run  string `yaml:"run"`
			If   string `yaml:"if"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// probeUses splits the workflows' `uses:` into the probe job's and everybody else's. A local
// (`./`) or `docker://` reference names no repository the setting governs.
func probeUses(t *testing.T, workflows map[string][]byte) (probe, others []string, problems []string) {
	t.Helper()

	seen := map[string]bool{}
	files := make([]string, 0, len(workflows))
	for file := range workflows {
		files = append(files, file)
	}
	sort.Strings(files)

	found := false
	for _, file := range files {
		var workflow probeWorkflow
		if err := yaml.Unmarshal(workflows[file], &workflow); err != nil {
			t.Fatalf("%s does not parse: %v", file, err)
		}
		for id, job := range workflow.Jobs {
			isProbe := file == "ci.yml" && id == probeJob
			if isProbe {
				found = true
			}
			runs := 0
			for _, step := range job.Steps {
				if step.Run != "" && isProbe {
					runs++
				}
				if step.Uses == "" || strings.HasPrefix(step.Uses, "./") || strings.HasPrefix(step.Uses, "docker://") {
					continue
				}
				if !isProbe {
					if !seen[step.Uses] {
						seen[step.Uses] = true
						others = append(others, step.Uses)
					}
					continue
				}
				if strings.TrimSpace(step.If) != "${{ false }}" {
					problems = append(problems, step.Uses+" in the probe must never run: `if: ${{ false }}`")
				}
				probe = append(probe, step.Uses)
			}
			if isProbe && runs != 1 {
				problems = append(problems, "the probe has one `run:` step and nothing else that runs")
			}
		}
	}
	if !found {
		problems = append(problems, "ci.yml has no job "+probeJob)
	}
	return probe, others, problems
}

func setDifference(a, b []string) []string {
	var out []string
	for _, v := range a {
		if !slices.Contains(b, v) {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func probeProblems(probe, others []string) []string {
	var problems []string
	for _, missing := range setDifference(others, probe) {
		problems = append(problems, missing+" is used by a workflow and missing from the probe")
	}
	for _, extra := range setDifference(probe, others) {
		problems = append(problems, extra+" is in the probe and no other job uses it")
	}
	counted := map[string]bool{}
	for _, u := range probe {
		if counted[u] {
			problems = append(problems, u+" is in the probe twice")
		}
		counted[u] = true
	}
	return problems
}

func TestTheActionsProbeNamesEveryActionTheWorkflowsUse(t *testing.T) {
	paths, err := filepath.Glob("../../.github/workflows/*.yml")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no workflows found under .github/workflows: %v", err)
	}
	workflows := map[string][]byte{}
	for _, path := range paths {
		workflows[filepath.Base(path)] = readFile(t, path)
	}

	probe, others, problems := probeUses(t, workflows)
	if len(others) < 10 {
		t.Fatalf("only %d actions found outside the probe - the workflows have moved, not stopped using actions", len(others))
	}
	problems = append(problems, probeProblems(probe, others)...)
	for _, p := range problems {
		t.Errorf("ci.yml job %s: %s", probeJob, p)
	}
}

func TestTheActionsProbeCheckRefusesWhatItShould(t *testing.T) {
	ci := []byte(`jobs:
  actions-allowed:
    steps:
      - run: echo
      - uses: acme/a@1111111111111111111111111111111111111111
        if: ${{ false }}
      - uses: acme/b@2222222222222222222222222222222222222222
  build:
    steps:
      - uses: ./local
      - uses: acme/a@0000000000000000000000000000000000000000
      - uses: acme/c@3333333333333333333333333333333333333333
`)
	probe, others, problems := probeUses(t, map[string][]byte{"ci.yml": ci})
	problems = append(problems, probeProblems(probe, others)...)

	for _, want := range []string{
		"acme/b@2222222222222222222222222222222222222222 in the probe must never run",
		"acme/a@0000000000000000000000000000000000000000 is used by a workflow and missing",
		"acme/c@3333333333333333333333333333333333333333 is used by a workflow and missing",
		"acme/a@1111111111111111111111111111111111111111 is in the probe and no other job uses it",
	} {
		found := false
		for _, p := range problems {
			found = found || strings.Contains(p, want)
		}
		if !found {
			t.Errorf("want a problem containing %q, got %q", want, problems)
		}
	}
	for _, p := range problems {
		if strings.Contains(p, "./local") {
			t.Errorf("a local action is none of the setting's business: %q", p)
		}
	}
}
