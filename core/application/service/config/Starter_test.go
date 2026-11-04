// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
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
