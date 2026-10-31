// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// configuration.md §6, a hub template: a template's hub part is taken by a hub once the workspace took
// the template's own part, applied to the hub through the hub's use cases, and recorded as that
// hub's provenance - not the workspace's, and the base of the hub's own upgrade.
func TestAHubTakesATemplatesHubPart(t *testing.T) {
	f := hubFixture()
	workspacePart := map[string]any{"types": map[string]any{"incident": map[string]any{"level": "TASK", "name": "Incident"}}}
	published := invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "desk", "name": "Desk", "document": workspacePart,
		"hub_document": map[string]any{"scheme": map[string]any{"every_type": map[string]any{"workflow": "service"}}},
	})
	if published["hub_document"] == nil {
		t.Fatalf("the hub part was not kept: %v", published)
	}

	_, err := PlanConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{"key": "desk", "hub_id": supportHub})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.template_workspace_first" {
		t.Fatalf("a hub took the template before the workspace: %v", err)
	}
	take := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "desk"})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": take["document"], "plan_hash": take["plan_hash"], "template": take["template"],
	})

	f.world.preview = usecase.Output{"entries": 2, "moves": []usecase.Output{}, "removed": []usecase.Output{}}
	hubPlan := invoke(t, PlanConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{"key": "desk", "hub_id": supportHub})
	if hubPlan["hub_id"] != supportHub || hubPlan["plan_hash"] == nil {
		t.Fatalf("the hub's plan %v", hubPlan)
	}
	applied := invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": hubPlan["document"], "plan_hash": hubPlan["plan_hash"], "template": hubPlan["template"],
		"mode": hubPlan["mode"], "hub_id": supportHub, "acknowledge": true,
	})
	if applied["state"] != "APPLIED" {
		t.Fatalf("not applied: %v", applied)
	}
	schemes := f.world.inputs["SetWorkflowScheme"]
	if len(schemes) != 1 || schemes[0]["container_id"] != supportHub || schemes[0].String("entry_type") != "" {
		t.Errorf("the hub part ran %v", schemes)
	}
	var hubUses, workspaceUses int
	for _, use := range f.store.uses {
		switch use.HubID {
		case shared.ID(supportHub):
			hubUses++
			if use.Document["scheme"] == nil || use.Document["types"] != nil {
				t.Errorf("the hub's base is not the hub part: %v", use.Document)
			}
		case "":
			workspaceUses++
		}
	}
	if hubUses != 1 || workspaceUses != 1 {
		t.Errorf("the uses recorded: %+v", f.store.uses)
	}

	listed := invoke(t, ListConfigTemplates{f.desk}.Descriptor(), person, usecase.Input{"hub_id": supportHub})["data"].([]usecase.Output)
	byKey := map[string]usecase.Output{}
	for _, row := range listed {
		byKey[row["key"].(string)] = row
	}
	if byKey["desk"]["taken_version"] != 1 || byKey["list"] != nil {
		t.Errorf("a hub's templates are %v", listed)
	}
	if _, offered := byKey["itil_service_desk"]; !offered {
		t.Errorf("the ITIL template is not offered to a hub: %v", listed)
	}

	_, err = PlanConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{"key": "list", "hub_id": supportHub})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.template_not_for_hubs" {
		t.Errorf("a template of the workspace only was planned for a hub: %v", err)
	}
	_, err = ApplyConfig{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"document": map[string]any{}, "plan_hash": "x", "template": "list@1", "hub_id": supportHub,
	})
	if err == nil {
		t.Error("a hub recorded a template of the workspace only")
	}
}

// A hub part that does not pass is not published, named under hub_document.
func TestABrokenHubPartIsNotPublished(t *testing.T) {
	f := hubFixture()
	_, err := PublishConfigTemplate{f.desk}.Descriptor().Handler.Invoke(t.Context(), person, usecase.Input{
		"key": "desk", "name": "Desk", "document": map[string]any{},
		"hub_document": map[string]any{"types": map[string]any{"incident": map[string]any{"level": "TASK", "name": "Incident"}}},
	})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Path != "/hub_document/types" {
		t.Errorf("a hub part with a workspace section was published: %v", err)
	}
}
