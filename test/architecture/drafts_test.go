// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A draft is checked in the session that writes it, and the pipeline runs when the pull request is
// ready (ADR-0079). Three things in the workflows make that true, and each one fails quietly when
// it goes missing: without `ready_for_review` leaving draft starts nothing, without the draft
// condition a draft runs everything again, and without the second name for `ci-required` a draft's
// commit carries a green `CI required` that no gate earned. These tests are what notices.

const (
	draftCondition = "!github.event.pull_request.draft"

	// The exact expression, because the property that matters is a string comparison GitHub makes:
	// branch protection asks for a check called `CI required`, and only a run that is not a draft's
	// may produce one (ci-cd.md §3.2).
	ciRequiredName = "${{ (github.event_name == 'pull_request' && github.event.pull_request.draft) && 'CI not run (draft)' || 'CI required' }}"
)

type workflowFile struct {
	On   map[string]yaml.Node   `yaml:"on"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Name  string    `yaml:"name"`
	If    string    `yaml:"if"`
	Needs yaml.Node `yaml:"needs"`
}

func loadWorkflow(t *testing.T, file string) workflowFile {
	t.Helper()

	raw, err := os.ReadFile("../../.github/workflows/" + file)
	if err != nil {
		t.Fatalf("%s is not readable: %v", file, err)
	}
	var workflow workflowFile
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatalf("%s does not parse: %v", file, err)
	}
	return workflow
}

// pullRequestTypes reads `on.pull_request.types`. An empty list is GitHub's default, which is
// exactly the list without `ready_for_review`.
func pullRequestTypes(t *testing.T, workflow workflowFile, file string) []string {
	t.Helper()

	trigger, ok := workflow.On["pull_request"]
	if !ok {
		t.Fatalf("%s no longer triggers on pull_request", file)
	}
	var config struct {
		Types []string `yaml:"types"`
	}
	if err := trigger.Decode(&config); err != nil {
		t.Fatalf("%s: on.pull_request does not decode: %v", file, err)
	}
	return config.Types
}

func TestLeavingDraftStartsThePipeline(t *testing.T) {
	for _, file := range []string{"ci.yml", "codeql.yml"} {
		types := pullRequestTypes(t, loadWorkflow(t, file), file)
		for _, want := range []string{"opened", "synchronize", "reopened", "ready_for_review"} {
			if !slices.Contains(types, want) {
				t.Errorf("%s does not trigger on pull_request %q - leaving draft, or a push, would "+
					"start no run (ADR-0079)", file, want)
			}
		}
	}
}

// Every job that needs no other job carries the draft condition; every other job depends on one of
// them and is skipped with it (ci-cd.md §3.4). `ci-required` is the one exception: it always runs
// and is renamed instead.
func TestADraftRunsNothing(t *testing.T) {
	ci := loadWorkflow(t, "ci.yml")

	roots := 0
	for id, job := range ci.Jobs {
		if id == "ci-required" || !job.Needs.IsZero() {
			continue
		}
		roots++
		if !strings.Contains(job.If, draftCondition) {
			t.Errorf("ci.yml: job %q needs no other job and has no draft condition - it runs on every "+
				"push to a draft (ADR-0079). Its `if:` needs %q", id, draftCondition)
		}
	}
	if roots < 5 {
		t.Fatalf("only %d jobs without needs found in ci.yml - the file has moved, not shrunk", roots)
	}

	analyze, ok := loadWorkflow(t, "codeql.yml").Jobs["analyze"]
	if !ok {
		t.Fatal("codeql.yml has no job `analyze`")
	}
	if !strings.Contains(analyze.If, draftCondition) {
		t.Errorf("codeql.yml: `analyze` has no draft condition (ADR-0079)")
	}
}

func TestADraftNeverCarriesCIRequired(t *testing.T) {
	summary, ok := loadWorkflow(t, "ci.yml").Jobs["ci-required"]
	if !ok {
		t.Fatal("ci.yml has no job `ci-required` - branch protection names its check")
	}
	if summary.Name != ciRequiredName {
		t.Errorf("ci.yml: `ci-required` is named %q; it must be\n  %s\nso that a draft's commit carries "+
			"no check called `CI required` (ci-cd.md §3.2)", summary.Name, ciRequiredName)
	}
	if summary.If != "always()" {
		t.Errorf("ci.yml: `ci-required` must run `if: always()`, it says %q - a skipped required "+
			"check reports success", summary.If)
	}
}
