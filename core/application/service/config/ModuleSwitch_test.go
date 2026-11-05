// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
)

func workflowsOn(blocks map[string]any) map[string]any {
	return map[string]any{
		"modules":   map[string]any{"workflows": map[string]any{"switch": "ON"}},
		"workflows": blocks,
	}
}

// user-experience.md §3.2, configuration.md §2: a document that switches a module on writes the
// blocks that module carries in the same plan and apply - the switch first - instead of refusing
// them as withheld; a template's part names its modules beside its types and workflows.
func TestADocumentThatSwitchesAModuleOnWritesItsSection(t *testing.T) {
	f := newFixture()
	f.world.workflowsOff = true
	document := workflowsOn(map[string]any{"card": map[string]any{"name": "Card", "shape": serviceShape("todo")}})

	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})
	if refusals, _ := plan["refusals"].([]usecase.Output); len(refusals) != 0 {
		t.Fatalf("refused %v", refusals)
	}
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": document, "plan_hash": plan["plan_hash"], "acknowledge": true,
	})
	if f.world.workflows["card"] == nil {
		t.Errorf("the workflow was not made: %v", f.world.workflows)
	}
	switched, created := slices.Index(f.world.calls, "SetModule"), slices.Index(f.world.calls, "CreateWorkflow")
	if switched < 0 || created < switched {
		t.Errorf("the calls ran %v, want the switch before the workflow", f.world.calls)
	}
}

// Without the switch, a section whose module is off stays refused by its module.
func TestASectionWhoseModuleStaysOffIsRefused(t *testing.T) {
	f := newFixture()
	f.world.workflowsOff = true
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": map[string]any{"workflows": map[string]any{"card": map[string]any{"name": "Card", "shape": serviceShape("todo")}}},
	})
	refusals, _ := plan["refusals"].([]usecase.Output)
	if !slices.ContainsFunc(refusals, func(r usecase.Output) bool { return r["code"] == module.OffCode }) {
		t.Fatalf("refusals %v, want %s", refusals, module.OffCode)
	}
}
