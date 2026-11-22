// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"errors"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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

// UC-INS-21 check 13: an apply of the workspace's document that names existing hubs - the hub a
// template is taken for - names them in each switch of a module on, so the switch does not leave
// that hub without the module; the plan is the document's, with hubs named or not, and a hub's
// own document takes none.
func TestAnApplyNamesTheChosenHubsInItsSwitch(t *testing.T) {
	f := newFixture()
	f.world.workflowsOff = true
	document := workflowsOn(map[string]any{"card": map[string]any{"name": "Card", "shape": serviceShape("todo")}})
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": document, "plan_hash": plan["plan_hash"], "acknowledge": true, "module_hub_ids": []any{supportHub},
	})
	switches := f.world.inputs["SetModule"]
	if len(switches) != 1 || switches[0].String("switch") != "ON" || switches[0].String("hub_id") != "" {
		t.Fatalf("the workspace's document switched %v", switches)
	}
	named, err := switches[0].IDList("hub_ids")
	if err != nil || len(named) != 1 || named[0].String() != supportHub {
		t.Errorf("the switch names %v (%v), want the chosen hub", switches[0]["hub_ids"], err)
	}

	hub := hubFixture()
	own := map[string]any{"scheme": map[string]any{"every_type": map[string]any{"workflow": "service"}}}
	hub.world.preview = usecase.Output{"entries": 0, "moves": []usecase.Output{}, "removed": []usecase.Output{}}
	hubPlan := invoke(t, PlanConfig{hub.desk}.Descriptor(), person, usecase.Input{"document": own, "hub_id": supportHub})
	_, err = ApplyConfig{hub.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"document": own, "hub_id": supportHub, "plan_hash": hubPlan["plan_hash"], "acknowledge": true,
		"module_hub_ids": []any{supportHub},
	})
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "module.hubs_with_on_only" {
		t.Errorf("a hub's own document with chosen hubs answered %v", err)
	}
	if len(hub.world.calls) != 0 {
		t.Errorf("the refused apply ran %v", hub.world.calls)
	}
}

// An apply names at most as many hubs as the contract's bound: more are refused before anything
// runs, at the field that named them.
func TestAnApplyNamingTooManyHubsIsRefused(t *testing.T) {
	f := newFixture()
	f.world.workflowsOff = true
	document := workflowsOn(map[string]any{"card": map[string]any{"name": "Card", "shape": serviceShape("todo")}})
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})
	hubs := make([]any, module.MaxChosenHubs+1)
	for i := range hubs {
		hubs[i] = supportHub
	}
	_, err := ApplyConfig{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"document": document, "plan_hash": plan["plan_hash"], "acknowledge": true, "module_hub_ids": hubs,
	})
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != module.TooManyHubsCode ||
		len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/module_hub_ids" {
		t.Errorf("an apply naming %d hubs answered %v", len(hubs), err)
	}
	if len(f.world.calls) != 0 {
		t.Errorf("the refused apply ran %v", f.world.calls)
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
