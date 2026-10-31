// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// configuration.md §3: an agent changes the sandbox freely - the workspace's policy does not
// hold it back, because nothing runs - and the configuration in force stays as it is.
func TestAnAgentChangesTheSandboxAndNothingRuns(t *testing.T) {
	f := newFixture()
	out := invoke(t, WriteConfigSandbox{f.desk}.Descriptor(), agent, usecase.Input{"document": incident()})
	if out["exists"] != true || out["writes"] != 1 {
		t.Fatalf("sandbox %v", out)
	}
	if len(f.world.calls) != 0 || f.world.types["incident"] != nil || len(f.store.revisions) != 0 {
		t.Errorf("a sandbox write changed the configuration in force: %v", f.world.calls)
	}
	read := invoke(t, ReadConfigSandbox{f.desk}.Descriptor(), person, nil)
	promotion := read["promotion"].(usecase.Output)
	if changes := promotion["changes"].([]usecase.Output); len(changes) != 1 || changes[0]["path"] != "types.incident" {
		t.Errorf("the promotion does not plan what the sandbox changed: %v", promotion["changes"])
	}
	if f.trail.actions[len(f.trail.actions)-1] != SandboxAction {
		t.Errorf("trail %v", f.trail.actions)
	}
}

// A document the workspace would refuse is refused in the sandbox too, with the same refusal.
func TestTheSandboxRefusesWhatThePromotionWould(t *testing.T) {
	f := newFixture()
	_, err := WriteConfigSandbox{f.desk}.Descriptor().Handler.Invoke(t.Context(), agent, usecase.Input{
		"document": map[string]any{"types": map[string]any{"task": nil}},
	})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.not_removable" {
		t.Errorf("the sandbox took a shipped type's removal: %v", err)
	}
	if f.store.sandbox != nil {
		t.Error("a refused write kept a sandbox")
	}
}

// Promoting the sandbox is an ordinary plan and apply of what it changed since its copy: a change
// made in force meanwhile is kept, and an agent's promotion waits for a person as any apply does.
func TestPromotingTheSandboxAppliesOnlyWhatItChanged(t *testing.T) {
	f := newFixture()
	invoke(t, WriteConfigSandbox{f.desk}.Descriptor(), agent, usecase.Input{"document": incident()})
	// Meanwhile, in force: a type the sandbox never saw.
	f.world.types["risk"] = map[string]any{"key": "risk", "level": "TASK", "name": "Risk", "system": false}

	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"from_sandbox": true})
	changes := plan["changes"].([]usecase.Output)
	if len(changes) != 1 || changes[0]["path"] != "types.incident" || changes[0]["op"] != "ADD" {
		t.Fatalf("the promotion undid or missed something: %v", changes)
	}
	proposed := invoke(t, ApplyConfig{f.desk}.Descriptor(), agent, usecase.Input{"from_sandbox": true, "plan_hash": plan["plan_hash"]})
	if proposed["state"] != "PROPOSED" {
		t.Errorf("an agent's promotion did not wait for a person: %v", proposed)
	}
	applied := invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"from_sandbox": true, "plan_hash": plan["plan_hash"]})
	if applied["state"] != "APPLIED" || f.world.types["incident"] == nil || f.world.types["risk"] == nil {
		t.Fatalf("promotion %v, types %v", applied, f.world.types)
	}

	out := invoke(t, WriteConfigSandbox{f.desk}.Descriptor(), person, usecase.Input{"discard": true})
	if out["exists"] != false || f.store.sandbox != nil {
		t.Errorf("the sandbox was not discarded: %v", out)
	}
	_, err := PlanConfig{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{"from_sandbox": true})
	if !isDetail(err, "config.sandbox_none") {
		t.Errorf("a promotion without a sandbox: %v", err)
	}
}
