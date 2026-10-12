// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A pull request merges without catching up with `main` (ADR-0083). What the pull request's own run
// did not see - the commits merged while it ran - only `main`'s run sees, so `main` must run every
// commit to the end. These tests hold the parts of ci.yml that make that true.

type concurrencyFile struct {
	Concurrency struct {
		Group            string `yaml:"group"`
		CancelInProgress string `yaml:"cancel-in-progress"`
	} `yaml:"concurrency"`
}

// The guard is the last step of `CI required`, so that auto-merge reads it as late as possible, and
// it runs only on a pull request: on the push to main it would read main's own red run and keep
// every later commit red, the revert included.
func TestTheGuardIsTheLastStepOfCIRequiredAndOnlyOnAPullRequest(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("ci.yml is not readable: %v", err)
	}
	var ci struct {
		Jobs map[string]struct {
			Permissions map[string]string `yaml:"permissions"`
			Steps       []struct {
				If  string `yaml:"if"`
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &ci); err != nil {
		t.Fatalf("ci.yml does not parse: %v", err)
	}
	job, ok := ci.Jobs["ci-required"]
	if !ok || len(job.Steps) == 0 {
		t.Fatal("ci.yml has no job `ci-required` with steps")
	}
	last := job.Steps[len(job.Steps)-1]
	if strings.TrimSpace(last.Run) != "scripts/ci/main-green.sh" {
		t.Fatalf("the last step of `CI required` runs %q; it must be the guard, scripts/ci/main-green.sh (ADR-0083)", last.Run)
	}
	if !strings.Contains(last.If, "github.event_name == 'pull_request'") || !strings.Contains(last.If, draftCondition) {
		t.Errorf("the guard's step runs `if: %s`; it must run only on a ready pull request - on the push to "+
			"main it would keep main red for ever", last.If)
	}
	for _, scope := range []string{"actions", "pull-requests"} {
		if job.Permissions[scope] != "read" {
			t.Errorf("`ci-required` must have %s: read for the guard, it has %q", scope, job.Permissions[scope])
		}
	}
}

func TestEveryCommitOnMainGetsItsOwnRun(t *testing.T) {
	raw, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("ci.yml is not readable: %v", err)
	}
	var ci concurrencyFile
	if err := yaml.Unmarshal(raw, &ci); err != nil {
		t.Fatalf("ci.yml does not parse: %v", err)
	}

	const (
		group  = "ci-${{ github.event_name == 'pull_request' && github.ref || github.sha }}"
		cancel = "${{ github.event_name == 'pull_request' }}"
	)
	if ci.Concurrency.Group != group {
		t.Errorf("ci.yml: the concurrency group is %q; it must be\n  %s\nso that every commit on main "+
			"has a group of its own - one group for all of main cancels a pending run when a third "+
			"arrives, and the commit that turned main red is then never known (ADR-0083)", ci.Concurrency.Group, group)
	}
	if ci.Concurrency.CancelInProgress != cancel {
		t.Errorf("ci.yml: cancel-in-progress is %q; it must be\n  %s\nso that only a pull request's "+
			"older run is cancelled, never a run on main (ADR-0083)", ci.Concurrency.CancelInProgress, cancel)
	}
}
