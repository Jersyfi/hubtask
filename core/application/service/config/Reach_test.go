// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

func serviceShape(statuses ...string) map[string]any {
	rows := []any{}
	for _, key := range statuses {
		rows = append(rows, map[string]any{"key": key, "name": key, "category": "IN_PROGRESS"})
	}
	return map[string]any{"statuses": rows}
}

// config-document.md §2: a plan says what a workflow's new shape reaches - the entries it moves,
// taken from the publish preview of the shape the plan sets, before anything is saved.
func TestAPlanNamesTheEntriesAWorkflowsNewShapeMoves(t *testing.T) {
	f := newFixture()
	f.world.workflows = map[string]map[string]any{
		"service": {"key": "service", "name": "Service", "published": serviceShape("todo", "doing")},
	}
	f.world.preview = usecase.Output{
		"entries": 7, "removed": []usecase.Output{},
		"moves": []usecase.Output{{"collection_id": "0190a8f0-0000-7000-8000-0000000000c1", "entry_type": "task", "from": "doing", "to": "todo", "count": 7}},
	}
	shape := serviceShape("todo", "review")
	document := map[string]any{"workflows": map[string]any{"service": map[string]any{"shape": shape}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})

	reached := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["path"] != "workflows.service" || reached[0]["entries"] != 7 {
		t.Fatalf("reach %v", reached)
	}
	asked := f.world.inputs["PreviewWorkflowPublish"]
	if len(asked) != 1 || asked[0]["key"] != "service" || asked[0]["shape"] == nil {
		t.Fatalf("the preview was not asked for the plan's shape: %v", asked)
	}
	if plan["plan_hash"] == nil || len(f.world.calls) != 0 {
		t.Errorf("plan %v wrote %v", plan["plan_hash"], f.world.calls)
	}
}

// S-09's gap: a reader the preview refuses - an agent's token with `config:plan` but not the
// preview's scope - gets a plan that says the reach was not counted, rather than one silent on it.
func TestAPlanSaysWhenItsReachWasWithheld(t *testing.T) {
	f := newFixture()
	f.world.workflows = map[string]map[string]any{
		"service": {"key": "service", "name": "Service", "published": serviceShape("todo", "doing")},
	}
	f.world.previewWithheld = true
	document := map[string]any{"workflows": map[string]any{"service": map[string]any{"shape": serviceShape("todo", "review")}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})

	reached := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["path"] != "workflows.service" || reached[0]["withheld"] != true {
		t.Fatalf("reach %v", reached)
	}
	if _, counted := reached[0]["entries"]; counted {
		t.Errorf("a withheld reach was counted: %v", reached[0])
	}
	if plan["plan_hash"] == nil {
		t.Error("a withheld reach refused the plan")
	}
}

// A shape that removes a status entries are in is refused at the plan, with the status and its
// count, as its publish would refuse the apply.
func TestAPlanRefusesAShapeRemovingAStatusInUse(t *testing.T) {
	f := newFixture()
	f.world.workflows = map[string]map[string]any{
		"service": {"key": "service", "name": "Service", "published": serviceShape("todo", "waiting")},
	}
	f.world.preview = usecase.Output{"entries": 0, "moves": []usecase.Output{}, "removed": []usecase.Output{{"status": "waiting", "count": 12}}}
	document := map[string]any{"workflows": map[string]any{"service": map[string]any{"shape": serviceShape("todo")}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})

	refusals := plan["refusals"].([]usecase.Output)
	if len(refusals) != 1 || refusals[0]["code"] != "workflows.mapping_required" || plan["plan_hash"] != nil {
		t.Fatalf("refusals %v, hash %v", refusals, plan["plan_hash"])
	}
	if params := refusals[0]["params"].(map[string]string); params["status"] != "waiting" || params["count"] != "12" {
		t.Errorf("the refusal does not name the status and its count: %v", params)
	}
}

// config-document.md §2: a plan says which people a role's change gives or takes a permission -
// how many hold it directly and as a group, and the permissions gained and lost.
func TestAPlanNamesThePeopleARolesChangeReaches(t *testing.T) {
	f := newFixture()
	f.world.roles["desk_agent"] = map[string]any{"key": "desk_agent", "name": "Desk agent", "permissions": []any{"READ", "COMMENT"}}
	f.world.grants = []any{
		map[string]any{"role": "desk_agent", "account_id": "0190a8f0-0000-7000-8000-0000000000d1"},
		map[string]any{"role": "desk_agent", "account_id": "0190a8f0-0000-7000-8000-0000000000d2"},
		map[string]any{"role": "desk_agent", "group_id": "0190a8f0-0000-7000-8000-0000000000e1"},
		map[string]any{"role": "other", "account_id": "0190a8f0-0000-7000-8000-0000000000d3"},
	}
	document := map[string]any{"roles": map[string]any{"desk_agent": map[string]any{"permissions": []any{"READ", "APPROVE"}}}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": document})

	reached := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["accounts"] != 2 || reached[0]["groups"] != 1 {
		t.Fatalf("reach %v", reached)
	}
	gained, lost := reached[0]["gained"].([]string), reached[0]["lost"].([]string)
	if len(gained) != 1 || gained[0] != "APPROVE" || len(lost) != 1 || lost[0] != "COMMENT" {
		t.Errorf("gained %v, lost %v", gained, lost)
	}
}

// S-09's gap: a plan that removes a field, or hides it from readers, says how many entries hold a
// value under it - what would stop being shown - asked by the field's identifier.
func TestAPlanCountsTheEntriesAFieldItRemovesHolds(t *testing.T) {
	f := newFixture()
	f.world.fields = map[string]map[string]any{
		"severity": {"id": "0190a8f0-0000-7000-8000-0000000000f1", "key": "severity", "kind": "TEXT", "applies_to": []any{"TASK"}},
	}
	f.world.fieldReach = 4
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": map[string]any{"fields": map[string]any{}}, "mode": "REPLACE",
	})

	reached, _ := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["path"] != "fields.severity" || reached[0]["hidden_values"] != 4 {
		t.Fatalf("reach %v", reached)
	}
	asked := f.world.inputs["PreviewCustomFieldReach"]
	if len(asked) != 1 || asked[0]["field_id"] != "0190a8f0-0000-7000-8000-0000000000f1" {
		t.Fatalf("the count was asked %v", asked)
	}
}

// S-09's gap: a plan that changes the workspace's own structure says what it would stop showing in
// every hub and collection that follows the workspace, from the workspace's structure preview.
func TestAPlanCountsWhatTheWorkspacesStructureWouldStopShowing(t *testing.T) {
	f := newFixture()
	f.world.structure = map[string]any{"completion": "MANUAL"}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": map[string]any{"structure": map[string]any{"capture": map[string]any{"value": "INBOX"}}},
	})

	reached, _ := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["path"] != "structure" ||
		reached[0]["hidden_entries"] != 3 || reached[0]["hidden_values"] != 5 {
		t.Fatalf("reach %v", reached)
	}
	asked := f.world.inputs["PreviewWorkspaceStructure"]
	if len(asked) != 1 || asked[0]["container_id"] != nil {
		t.Fatalf("the workspace's preview was asked %v", asked)
	}
	if settings, _ := asked[0]["settings"].(map[string]any); settings["capture"] != "INBOX" {
		t.Errorf("the preview was asked for %v", asked[0])
	}
	if f.world.inputs["PreviewContainerStructure"] != nil {
		t.Errorf("a hub's preview was asked for the workspace: %v", f.world.inputs["PreviewContainerStructure"])
	}
}

// S-09's gap: a plan that switches a module off says how many people it leaves without it, asked
// at the plan's place; a switch on is not counted.
func TestAPlanCountsThePeopleAModuleSwitchedOffLeaves(t *testing.T) {
	f := newFixture()
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": map[string]any{"modules": map[string]any{"workflows": map[string]any{"switch": "OFF"}}},
	})

	reached, _ := plan["reach"].([]usecase.Output)
	if len(reached) != 1 || reached[0]["path"] != "modules.workflows" || reached[0]["people"] != 7 {
		t.Fatalf("reach %v", reached)
	}
	asked := f.world.inputs["PreviewModuleReach"]
	if len(asked) != 1 || asked[0]["module"] != "WORKFLOWS" || asked[0]["hub_id"] != nil {
		t.Fatalf("the count was asked %v", asked)
	}

	f.world.previewWithheld = true
	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": map[string]any{"modules": map[string]any{"workflows": map[string]any{"switch": "OFF"}}},
	})
	if reached, _ := plan["reach"].([]usecase.Output); len(reached) != 1 || reached[0]["withheld"] != true {
		t.Errorf("a withheld count was answered %v", reached)
	}
}
