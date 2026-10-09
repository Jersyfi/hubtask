// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A job without `timeout-minutes` runs for GitHub's default of six hours. A hung step - an apt
// mirror that stops answering - then holds `CI required`, and with it every merge, without a
// single red mark (rule 7 in the pipeline's terms, ci-cd.md §4). The limit is set from the job's
// measured duration; this test only holds that there is one, and that it is a number a reader can
// compare against a run, not an expression.

// longestJob is the ceiling: the nightly's fuzzing job. A limit above it is not a limit.
const longestJob = 120

type timedWorkflow struct {
	Jobs map[string]timedJob `yaml:"jobs"`
}

type timedJob struct {
	Uses    string      `yaml:"uses"`
	Timeout yaml.Node   `yaml:"timeout-minutes"`
	Steps   []timedStep `yaml:"steps"`
}

type timedStep struct {
	Run     string    `yaml:"run"`
	Timeout yaml.Node `yaml:"timeout-minutes"`
}

func minutes(node yaml.Node) (int, bool) {
	if node.IsZero() || node.Tag != "!!int" {
		return 0, false
	}
	var n int
	if err := node.Decode(&n); err != nil {
		return 0, false
	}
	return n, true
}

func loadTimedWorkflow(t *testing.T, path string) timedWorkflow {
	t.Helper()

	var workflow timedWorkflow
	if err := yaml.Unmarshal(readFile(t, path), &workflow); err != nil {
		t.Fatalf("%s does not parse: %v", filepath.Base(path), err)
	}
	return workflow
}

func TestEveryJobHasATimeLimit(t *testing.T) {
	files, err := filepath.Glob("../../.github/workflows/*.yml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no workflow found: %v", err)
	}
	jobs := 0
	for _, file := range files {
		for id, job := range loadTimedWorkflow(t, file).Jobs {
			// A job that calls a reusable workflow cannot carry the key; the called workflow's own
			// jobs do, and they are in this directory.
			if job.Uses != "" {
				continue
			}
			jobs++
			limit, ok := minutes(job.Timeout)
			switch {
			case !ok:
				t.Errorf("%s: job %q has no `timeout-minutes` as a number - it would run for six "+
					"hours (ci-cd.md §4)", filepath.Base(file), id)
			case limit < 1 || limit > longestJob:
				t.Errorf("%s: job %q has `timeout-minutes: %d`; a limit lies between 1 and %d",
					filepath.Base(file), id, limit, longestJob)
			}
		}
	}
	if jobs < 30 {
		t.Fatalf("only %d jobs found under .github/workflows - the files have moved, not shrunk", jobs)
	}
}

// The step that hung: `playwright install-deps` runs apt against the runner's mirror. It is given
// a limit of its own, shorter than the job's, so a hung mirror reads as that step in the log and
// not as a slow browser test.
func TestTheEnginesSystemPackagesHaveTheirOwnLimit(t *testing.T) {
	engines, ok := loadTimedWorkflow(t, "../../.github/workflows/ci.yml").Jobs["engines"]
	if !ok {
		t.Fatal("ci.yml has no job `engines`")
	}
	job, _ := minutes(engines.Timeout)
	found := 0
	for _, step := range engines.Steps {
		if !strings.Contains(step.Run, "playwright install") {
			continue
		}
		found++
		limit, ok := minutes(step.Timeout)
		if !ok || limit >= job {
			t.Errorf("ci.yml: the engines step %q needs a `timeout-minutes` below the job's %d, "+
				"it has %q", step.Run, job, step.Timeout.Value)
		}
	}
	if found == 0 {
		t.Fatal("ci.yml: no `playwright install` step found in `engines` - the step has moved")
	}
}
