// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-WRK-30 check 5, user-experience.md §3.2: the first start built on a template takes its
// workspace part and then its hub part into the hub just made, each as ApplyConfig would, and
// records both as taken.
func TestAStarterTakesATemplatesWorkspaceAndHubParts(t *testing.T) {
	f := hubFixture()
	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "desk", "name": "Desk",
		"document":     map[string]any{"types": map[string]any{"incident": map[string]any{"level": "TASK", "name": "Incident"}}},
		"hub_document": map[string]any{"scheme": map[string]any{"every_type": map[string]any{"workflow": "service"}}},
	})
	f.world.preview = usecase.Output{"entries": 0, "moves": []usecase.Output{}, "removed": []usecase.Output{}}
	starter := StarterTemplates{Desk: f.desk}
	take := work.StarterTemplate{Key: "desk", Samples: true, Acknowledge: true}

	if err := starter.TakeForWorkspace(t.Context(), person, take); err != nil {
		t.Fatalf("the workspace part: %v", err)
	}
	if _, made := f.world.types["incident"]; !made {
		t.Errorf("the workspace part did not make its type: %v", f.world.types)
	}
	if err := starter.TakeForHub(t.Context(), person, take, shared.ID(supportHub)); err != nil {
		t.Fatalf("the hub part: %v", err)
	}
	if schemes := f.world.inputs["SetWorkflowScheme"]; len(schemes) != 1 || schemes[0]["container_id"] != supportHub {
		t.Errorf("the hub part ran %v", schemes)
	}
	var hubUses, workspaceUses int
	for _, use := range f.store.uses {
		switch use.HubID {
		case shared.ID(supportHub):
			hubUses++
		case "":
			workspaceUses++
		}
	}
	if hubUses != 1 || workspaceUses != 1 {
		t.Errorf("the uses recorded: %+v", f.store.uses)
	}
}

// A template the catalogue does not hold refuses the set rather than making a bare hub, and one
// without a hub part leaves the hub as its preset made it.
func TestAStarterOnAnUnknownOrWorkspaceOnlyTemplate(t *testing.T) {
	f := hubFixture()
	starter := StarterTemplates{Desk: f.desk}
	if err := starter.TakeForWorkspace(t.Context(), person, work.StarterTemplate{Key: "nothing_like_it"}); err == nil {
		t.Error("an unknown template was taken")
	}
	if err := starter.TakeForHub(t.Context(), person, work.StarterTemplate{Key: "nothing_like_it"}, shared.ID(supportHub)); err == nil {
		t.Error("an unknown template's hub part was taken")
	}
	if err := starter.TakeForHub(t.Context(), person, work.StarterTemplate{Key: "list"}, shared.ID(supportHub)); err != nil {
		t.Errorf("a template without a hub part refused the hub: %v", err)
	}
	if len(f.store.uses) != 0 {
		t.Errorf("a template without a hub part recorded %+v", f.store.uses)
	}
}

// A first start that took no template, or was skipped, still leaves a version of the workspace as
// it started: a change made on a screen later is compared with it and can be rolled back to it,
// although nothing was ever applied (configuration.md §5).
func TestAFirstStartWithoutATemplateLeavesAVersionToRollBackTo(t *testing.T) {
	f := newFixture()
	starter := StarterTemplates{Desk: f.desk}
	if err := starter.Baseline(t.Context(), person, ""); err != nil {
		t.Fatalf("the baseline: %v", err)
	}
	if len(f.store.revisions) != 1 || f.store.revisions[0].Version != 1 || f.store.revisions[0].ActorKind != domain.ActorDirect {
		t.Fatalf("versions %+v, want the workspace as it started", f.store.revisions)
	}
	if err := starter.Baseline(t.Context(), person, ""); err != nil || len(f.store.revisions) != 1 {
		t.Errorf("a second baseline over an unchanged workspace: %v, %d versions", err, len(f.store.revisions))
	}

	// On a screen: a type made directly, then the person goes back to the start.
	f.world.types["problem"] = map[string]any{"key": "problem", "level": "TASK", "name": "Problem", "system": false}
	back := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"to_version": 1})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"to_version": 1, "plan_hash": back["plan_hash"], "acknowledge": true})
	if f.world.types["problem"] != nil {
		t.Error("the screen's change was not rolled back to the start")
	}
}

// §3.2: a new workspace starts with no module on, and the starter switches on what the template
// needs - its workspace part, which names the module beside its workflows, is taken whole.
func TestAStarterSwitchesOnTheModulesItsTemplateNeeds(t *testing.T) {
	f := newFixture()
	f.world.workflowsOff = true
	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "board", "name": "Board",
		"document": workflowsOn(map[string]any{"card": map[string]any{"name": "Card", "shape": map[string]any{
			"initial": "todo",
			"statuses": []any{
				map[string]any{"key": "todo", "name": "To do", "category": "TODO"},
				map[string]any{"key": "done", "name": "Done", "category": "DONE"},
			},
			"steps": []any{map[string]any{"key": "finish", "name": "Finish", "from": []any{"todo"}, "to": "done"}},
		}}}),
	})
	take := work.StarterTemplate{Key: "board", Acknowledge: true}
	if err := (StarterTemplates{Desk: f.desk}).TakeForWorkspace(t.Context(), person, take); err != nil {
		t.Fatalf("the workspace part: %v", err)
	}
	if f.world.workflowsOff || f.world.workflows["card"] == nil {
		t.Errorf("the module is off %v, the workflows %v", f.world.workflowsOff, f.world.workflows)
	}
}

// user-experience.md §5.2: a hub made from a template takes the template's workspace part only
// where the workspace has not taken it - a second hub from it is no upgrade -, and a template
// without a hub part is refused before anything is made.
func TestAHubFromATemplateTakesItsWorkspacePartOnce(t *testing.T) {
	f := hubFixture()
	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "desk", "name": "Desk",
		"document":     map[string]any{"types": map[string]any{"incident": map[string]any{"level": "TASK", "name": "Incident"}}},
		"hub_document": map[string]any{"scheme": map[string]any{"every_type": map[string]any{"workflow": "service"}}},
	})
	starter := StarterTemplates{Desk: f.desk}
	take := work.StarterTemplate{Key: "desk", Acknowledge: true}
	for range 2 {
		if err := starter.TakeWorkspacePartOnce(t.Context(), person, take); err != nil {
			t.Fatalf("the workspace part: %v", err)
		}
	}
	var workspaceUses int
	for _, use := range f.store.uses {
		if use.HubID == "" {
			workspaceUses++
		}
	}
	if workspaceUses != 1 || f.world.types["incident"] == nil {
		t.Errorf("uses %+v, types %v: want the workspace part taken once", f.store.uses, f.world.types)
	}

	invoke(t, PublishConfigTemplate{f.desk}.Descriptor(), person, usecase.Input{
		"key": "register", "name": "Register",
		"document": map[string]any{"types": map[string]any{"vendor": map[string]any{"level": "TASK", "name": "Vendor"}}},
	})
	err := starter.TakeWorkspacePartOnce(t.Context(), person, work.StarterTemplate{Key: "register"})
	if refusals := refusalsIn(err); len(refusals) != 1 || refusals[0].Code != "config.template_not_for_hubs" {
		t.Errorf("a template without a hub part: %v", err)
	}
	if f.world.types["vendor"] != nil {
		t.Error("a template without a hub part took its workspace part")
	}
}
