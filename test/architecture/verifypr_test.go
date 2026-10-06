// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Jersyfi/hubtask/tools/cilocal"
)

// `make verify-pr` is the pull request check on a laptop (ADR-0078), and it is only that while its
// job table says what `ci.yml` says. A job added to the pipeline without a local counterpart, or a
// filter changed in the workflow and not in the table, would make verify-pr green on a branch CI
// then turns red - the round trip the local check exists to save. So the table is held to the
// workflow from three sides: the same jobs, the same filters per job, the same filters per package.

var changesOutput = regexp.MustCompile(`needs\.changes\.outputs\.([a-z0-9_]+)`)

type pipelineJob struct {
	If       string    `yaml:"if"`
	Needs    yaml.Node `yaml:"needs"`
	Strategy struct {
		Matrix struct {
			Include []map[string]string `yaml:"include"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
}

func loadPipeline(t *testing.T) map[string]pipelineJob {
	t.Helper()

	raw, err := os.ReadFile("../../" + cilocal.WorkflowPath)
	if err != nil {
		t.Fatalf("ci.yml is not readable: %v", err)
	}
	var workflow struct {
		Jobs map[string]pipelineJob `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &workflow); err != nil {
		t.Fatalf("ci.yml does not parse: %v", err)
	}
	return workflow.Jobs
}

func needsOf(t *testing.T, job pipelineJob) []string {
	t.Helper()

	switch job.Needs.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		return []string{job.Needs.Value}
	default:
		var needs []string
		if err := job.Needs.Decode(&needs); err != nil {
			t.Fatalf("a job's needs do not decode: %v", err)
		}
		return needs
	}
}

func outputsIn(expression string) []string {
	seen := map[string]bool{}
	for _, match := range changesOutput.FindAllStringSubmatch(expression, -1) {
		seen[match[1]] = true
	}
	outputs := make([]string, 0, len(seen))
	for output := range seen {
		outputs = append(outputs, output)
	}
	sort.Strings(outputs)
	return outputs
}

func sorted(values []string) []string {
	out := slices.Clone(values)
	sort.Strings(out)
	return out
}

func TestVerifyPRKnowsEveryJobCIRequiredWaitsFor(t *testing.T) {
	pipeline := loadPipeline(t)

	var waitedFor []string
	for _, id := range needsOf(t, pipeline["ci-required"]) {
		if id != "changes" {
			waitedFor = append(waitedFor, id)
		}
	}
	if len(waitedFor) < 10 {
		t.Fatalf("ci-required waits for only %d jobs - the file has moved, not shrunk", len(waitedFor))
	}

	var known []string
	for _, job := range cilocal.Jobs {
		known = append(known, job.ID)

		kinds := 0
		if job.InVerify {
			kinds++
		}
		if len(job.Steps) > 0 {
			kinds++
		}
		if job.CIOnly != "" {
			kinds++
		}
		if kinds != 1 {
			t.Errorf("tools/cilocal: job %q must be exactly one of: run by make verify, run by its own "+
				"steps, or CI only with the reason", job.ID)
		}
	}

	if got, want := sorted(known), sorted(waitedFor); !slices.Equal(got, want) {
		t.Errorf("tools/cilocal knows the jobs\n  %v\nbut ci-required waits for\n  %v\n"+
			"a job verify-pr does not know is a job a branch can only learn about after Ready (ADR-0078)",
			got, want)
	}
}

// A job's filters are the outputs its own `if:` names. A job that names none and needs `quick`
// runs whenever `quick` does; a job that needs nothing runs always.
func TestVerifyPRSelectsJobsByTheWorkflowsFilters(t *testing.T) {
	pipeline := loadPipeline(t)
	quick := outputsIn(pipeline["quick"].If)

	for _, job := range cilocal.Jobs {
		workflowJob, ok := pipeline[job.ID]
		if !ok {
			continue // reported by the test above
		}
		want := outputsIn(workflowJob.If)
		if len(want) == 0 && slices.Contains(needsOf(t, workflowJob), "quick") {
			want = quick
		}
		if got := sorted(job.Filters); !slices.Equal(got, want) {
			t.Errorf("job %q: tools/cilocal selects it by %v, ci.yml by %v", job.ID, got, want)
		}
	}
}

func TestVerifyPRSelectsPackagesByTheWorkflowsMatrix(t *testing.T) {
	steps := map[string]cilocal.Step{}
	for _, job := range cilocal.Jobs {
		if job.ID == "node" {
			for _, step := range job.Steps {
				steps[step.Name] = step
			}
		}
	}

	include := loadPipeline(t)["node"].Strategy.Matrix.Include
	if len(include) < 5 {
		t.Fatalf("the node job's matrix has %d packages - the file has moved, not shrunk", len(include))
	}
	for _, entry := range include {
		name := "workspace (" + entry["package"] + ")"
		step, ok := steps[name]
		if !ok {
			t.Errorf("ci.yml builds the package %q and tools/cilocal has no step %q for it", entry["package"], name)
			continue
		}
		if got, want := sorted(step.Filters), outputsIn(entry["run"]); !slices.Equal(got, want) {
			t.Errorf("package %q: tools/cilocal runs it for %v, ci.yml for %v", entry["package"], got, want)
		}
		if !strings.Contains(step.Command, `"@hubtask/`+entry["package"]+`"`) {
			t.Errorf("package %q: the step's command does not name the package: %s", entry["package"], step.Command)
		}
	}
}
