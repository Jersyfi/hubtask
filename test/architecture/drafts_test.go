// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A draft runs nothing, and the pipeline runs when the pull request is ready (ADR-0079, ADR-0089).
// Three things in the workflows make that true, and each one fails quietly when it goes missing:
// without `ready_for_review` leaving draft starts nothing, without the draft condition a draft runs
// everything again, and without the second name for `ci-required` a draft's commit carries a green
// `CI required` that no gate earned - a skipped job reports success under its name. Skipped on a
// draft, GitHub reports it under the expression unevaluated, which is not `CI required` either
// (seen on the first draft after ADR-0089). These tests are what notices.

const (
	draftCondition = "!github.event.pull_request.draft"

	// The exact expression, because the property that matters is a string comparison GitHub makes:
	// branch protection asks for a check called `CI required`, and only a pull request's run that
	// is not a draft's, or a push's, may produce one (ci-cd.md §3.2). A manual run checks the
	// branch's head - the commit a draft's pull request shows - without what reads a pull request
	// (ci-cd.md §3.4 lists it) and without the guard, so its summary has a name of its own.
	ciRequiredName = "${{ (github.event_name == 'pull_request' && github.event.pull_request.draft) && 'CI not run (draft)' || github.event_name == 'workflow_dispatch' && 'CI by hand' || 'CI required' }}"

	// Always, except on a draft: there it is skipped, so a draft's run starts no runner at all.
	ciRequiredIf = "always() && (github.event_name != 'pull_request' || !github.event.pull_request.draft)"

	// The one place a job reads what only a push has: the commit before it, which is empty on a
	// manual run. The step below it then takes where the branch left main instead.
	apiCompatBase = "API_BASE: ${{ github.event_name == 'pull_request' && github.event.pull_request.base.sha || github.event.before }}"
)

type workflowFile struct {
	On   map[string]yaml.Node   `yaml:"on"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Name  string         `yaml:"name"`
	If    string         `yaml:"if"`
	Needs yaml.Node      `yaml:"needs"`
	Steps []workflowStep `yaml:"steps"`
}

type workflowStep struct {
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
	Env  map[string]string `yaml:"env"`
}

// wholeHistory says whether the job's checkout fetches every branch and the history behind them.
func (j workflowJob) wholeHistory() bool {
	for _, step := range j.Steps {
		if strings.HasPrefix(step.Uses, "actions/checkout@") {
			return step.With["fetch-depth"] == "0"
		}
	}
	return false
}

// stepRunning is the job's step whose script holds command.
func (j workflowJob) stepRunning(command string) (workflowStep, bool) {
	for _, step := range j.Steps {
		if strings.Contains(step.Run, command) {
			return step, true
		}
	}
	return workflowStep{}, false
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
// them and is skipped with it (ci-cd.md §3.4). `ci-required` is the one exception: it runs after
// every other job whatever they ended as, and carries its own draft condition.
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
	if summary.If != ciRequiredIf {
		t.Errorf("ci.yml: `ci-required` must run `if: %s`, it says %q - without always() a failed "+
			"job skips it, and a skipped required check reports success", ciRequiredIf, summary.If)
	}
}

// A worker may start one full run on a pushed branch by hand while the pull request stays a draft
// (ci-cd.md §3.4). That holds only while three things do: the manual trigger exists, a push starts
// a run on `main` alone, and no job tells a push from a manual run - every condition asks whether
// the event is a pull request, so whatever is none runs what `main` runs. The concurrency group
// (mergeflow_test.go) and the summary's name are the two places that name another event, and
// `api-compat`'s base the one that reads what a manual run lacks.
func TestAManualRunChecksABranchAndADraftsPushStillStartsNothing(t *testing.T) {
	ci := loadWorkflow(t, "ci.yml")

	if _, ok := ci.On["workflow_dispatch"]; !ok {
		t.Errorf("ci.yml does not trigger on workflow_dispatch - a concept's checkpoint could start no " +
			"run on its branch without leaving draft")
	}

	push, ok := ci.On["push"]
	if !ok {
		t.Fatal("ci.yml no longer triggers on push")
	}
	var config struct {
		Branches []string `yaml:"branches"`
	}
	if err := push.Decode(&config); err != nil {
		t.Fatalf("ci.yml: on.push does not decode: %v", err)
	}
	if !slices.Equal(config.Branches, []string{"main"}) {
		t.Errorf("ci.yml triggers on a push to %v; it must be [main] alone - a push to a draft's "+
			"branch would start a run the draft condition does not see (ADR-0079)", config.Branches)
	}

	raw, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("ci.yml is not readable: %v", err)
	}
	jobs := strings.Replace(string(raw), ciRequiredName, "", 1)
	if !strings.Contains(jobs, apiCompatBase) {
		t.Errorf("ci.yml: `api-compat` no longer sets\n  %s\n- the one known reading of "+
			"`github.event.before`; if the base is found another way, this test follows", apiCompatBase)
	}
	jobs = strings.Replace(jobs, apiCompatBase, "", 1)
	if at := strings.Index(jobs, "\njobs:\n"); at >= 0 {
		jobs = jobs[at:]
	} else {
		t.Fatal("ci.yml has no `jobs:` line")
	}
	for _, match := range regexp.MustCompile(`github\.event_name\s*[!=]=\s*'([a-z_]+)'`).FindAllStringSubmatch(jobs, -1) {
		if match[1] != "pull_request" {
			t.Errorf("ci.yml: a job asks `%s`; a job may only ask whether the event is a pull request, "+
				"so that a manual run runs what a push to main runs", match[0])
		}
	}
	// However it is written - `==`, `contains(…)`, `startsWith(…)`, as a value: the ref, and what
	// only a pull request or only a push carries, tell a branch's manual run from `main`'s.
	branch := regexp.MustCompile(`github\.(ref|ref_name|ref_type|ref_protected|base_ref|head_ref|event\.before|event\.after|event\.ref)\b`)
	for _, named := range branch.FindAllString(jobs, -1) {
		t.Errorf("ci.yml: a job reads `%s`; no job of ci.yml may depend on the branch or on what only "+
			"a push carries - a manual run on a branch would then run less, or more, than main's", named)
	}
	if named := regexp.MustCompile(`(contains|startsWith|endsWith)\(\s*github\.event_name`).FindString(jobs); named != "" {
		t.Errorf("ci.yml: a job asks `%s…`; a job may only ask `github.event_name == 'pull_request'` "+
			"or its negation", named)
	}
}

// A manual run has no pull request and no commit before it, and two jobs judge what a branch adds
// (ci-cd.md §3.4). Without a base `api-compat` judges every break since the tag against every
// commit since it, so one marked break on `main` carries the branch's unmarked one through - green
// by hand, red at Ready. Without the open pull requests `numbers` is green while another one takes
// the same migration number.
func TestAManualRunJudgesWhatItsBranchAdds(t *testing.T) {
	ci := loadWorkflow(t, "ci.yml")

	compat := ci.Jobs["api-compat"]
	gate, ok := compat.stepRunning("make gate-api-compat")
	if !ok {
		t.Fatal("ci.yml: `api-compat` has no step that runs make gate-api-compat")
	}
	if !strings.Contains(gate.Run, `if [ -z "${API_BASE}" ]`) || !strings.Contains(gate.Run, "git merge-base origin/main HEAD") {
		t.Errorf("ci.yml: `api-compat` does not fall back to `git merge-base origin/main HEAD` when " +
			"API_BASE is empty - a manual run would judge the branch's breaks with main's marks")
	}
	if !compat.wholeHistory() {
		t.Errorf("ci.yml: `api-compat` needs `fetch-depth: 0` - the tag, main and the merge base are behind HEAD")
	}

	numbers := ci.Jobs["numbers"]
	gate, ok = numbers.stepRunning("make gate-numbers")
	if !ok {
		t.Fatal("ci.yml: `numbers` has no step that runs make gate-numbers")
	}
	if _, set := gate.Env["OPEN"]; set {
		t.Errorf("ci.yml: `numbers` sets OPEN; the comparison with the open pull requests is never " +
			"switched off by the event - a commit on main adds nothing over main, so it asks nothing")
	}
	if want := "${{ github.event_name == 'pull_request' && 'HEAD^1' || 'origin/main' }}"; gate.Env["BASE"] != want {
		t.Errorf("ci.yml: `numbers` has BASE %q, it must be %q", gate.Env["BASE"], want)
	}
	if !numbers.wholeHistory() {
		t.Errorf("ci.yml: `numbers` needs `fetch-depth: 0` - a manual run finds where its branch left main")
	}
}
