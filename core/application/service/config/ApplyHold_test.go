// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
)

// otherApply is a second administrator's apply committing while this one waits for the hold: the
// incident type renamed meanwhile.
func otherApply(f fixture) func() {
	return func() {
		f.world.types["incident"] = map[string]any{"key": "incident", "level": "TASK", "name": "Störung", "position": 1}
	}
}

// Two applies planned on one configuration never both land: the apply holds the workspace's
// configuration before it reads it, so the later one judges the configuration as the earlier left
// it and is refused, instead of writing over its change and recording that change as a direct
// edit (configuration.md §2, all or nothing against the configuration it was planned on).
func TestAnApplyJudgesTheConfigurationAsItStandsOnceHeld(t *testing.T) {
	f := newFixture()
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": incident()})
	f.store.onHold = otherApply(f)
	_, err := ApplyConfig{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"document": incident(), "plan_hash": plan["plan_hash"], "base_hash": plan["base_hash"],
	})
	if !isDetail(err, "config.version_conflict") {
		t.Fatalf("an apply planned before another landed wrote over it: %v, types %v", err, f.world.types)
	}
}

func TestADecidedProposalJudgesTheConfigurationAsItStandsOnceHeld(t *testing.T) {
	f := newFixture()
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": incident()})
	proposed := invoke(t, ProposeConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": incident(), "plan_hash": plan["plan_hash"],
	})
	id := proposed["proposal"].(usecase.Output)["id"]
	f.store.onHold = otherApply(f)
	_, err := DecideConfigProposal{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"proposal_id": id, "decision": "APPLY",
	})
	if !isDetail(err, "config.version_conflict") {
		t.Fatalf("a proposal decided while another apply landed wrote over it: %v, types %v", err, f.world.types)
	}
}

// Two people writing the sandbox at once: the second write reads the sandbox the first left once
// it holds the configuration, so neither change is lost.
func TestASandboxWriteKeepsTheOneThatLandedMeanwhile(t *testing.T) {
	f := newFixture()
	invoke(t, WriteConfigSandbox{f.desk}.Descriptor(), person, usecase.Input{"document": incident()})
	f.store.onHold = func() {
		types := domain.Section{}
		for key, block := range f.store.sandbox.Document["types"] {
			types[key] = block
		}
		types["risk"] = domain.Block{"level": "TASK", "name": "Risk"}
		f.store.sandbox.Document["types"] = types
	}
	invoke(t, WriteConfigSandbox{f.desk}.Descriptor(), person, usecase.Input{
		"document": map[string]any{"types": map[string]any{"problem": map[string]any{"level": "TASK", "name": "Problem"}}},
	})
	types := f.store.sandbox.Document["types"]
	for _, key := range []string{"incident", "risk", "problem"} {
		if types[key] == nil {
			t.Errorf("the sandbox lost %s: %v", key, types)
		}
	}
}
