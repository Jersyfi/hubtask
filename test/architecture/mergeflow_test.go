// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
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
