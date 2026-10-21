// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// structureOf is a container's structure as its read answers it: the levels, and per level the
// capabilities offered.
func structureOf(levels []string, fields map[string][]string) map[string]any {
	listed := make([]any, 0, len(levels))
	for _, level := range levels {
		listed = append(listed, map[string]any{"type": level, "name": nil})
	}
	offered := map[string]any{}
	for itemType, capabilities := range fields {
		values := make([]any, 0, len(capabilities))
		for _, capability := range capabilities {
			values = append(values, capability)
		}
		offered[itemType] = map[string]any{"capabilities": values}
	}
	return map[string]any{"settings": map[string]any{"levels": listed, "fields": offered}}
}

// Tasks: tasks and activities, an activity without notes.
func tasksStructure() map[string]any {
	return structureOf([]string{"TASK", "ACTIVITY"}, map[string][]string{
		"TASK":     {"COMPLETION", "NOTES"},
		"ACTIVITY": {"COMPLETION", "DUE_DATE"},
	})
}

func TestABreakdownKeepsToTheLevelsAndNotesTheCollectionOffers(t *testing.T) {
	shape := shapeFrom(usecase.Output{"structure": tasksStructure()})
	answered := map[string]any{"children": []any{
		map[string]any{"type": "WORK_PACKAGE", "title": "Draft", "children": []any{
			map[string]any{"type": "ACTIVITY", "title": "Outline"},
		}},
		map[string]any{"type": "ACTIVITY", "title": "Call", "notes": "Before noon"},
	}}

	kept, ok := keptTree(answered, nil, shape)
	if !ok {
		t.Fatal("the breakdown was refused")
	}
	children, _ := kept["children"].([]any)
	if len(children) != 1 {
		t.Fatalf("kept %v, want the activity alone: Tasks hides work packages", children)
	}
	piece, _ := children[0].(map[string]any)
	if piece["type"] != "ACTIVITY" {
		t.Errorf("kept %v", piece)
	}
	if _, held := piece["notes"]; held {
		t.Error("an activity kept notes its collection does not offer")
	}
}

func TestATemplateDraftKeepsToTheLevelsTheCollectionOffers(t *testing.T) {
	shape := shapeFrom(usecase.Output{"structure": tasksStructure()})
	text := `{"name": "Move", "nodes": [{"type": "TASK", "title": "Move", "children": [
		{"type": "WORK_PACKAGE", "title": "Pack", "children": [{"type": "ACTIVITY", "title": "Tape"}]},
		{"type": "ACTIVITY", "title": "Call"}]}]}`

	payload, dropped, ok := templateFrom(text, hierarchyFixture(t), shape, shared.MustParseID("0192f000-0000-7000-8000-0000000000fc"))
	if !ok {
		t.Fatal("the draft was refused")
	}
	if dropped != 2 {
		t.Errorf("dropped %d, want the work package with its activity", dropped)
	}
	nodes, _ := payload["nodes"].([]any)
	root, _ := nodes[0].(map[string]any)
	if children, _ := root["children"].([]any); len(children) != 1 {
		t.Errorf("kept %v, want the activity alone", children)
	}
}

func TestAProposalFillsNothingTheCollectionHides(t *testing.T) {
	payload := map[string]any{"title": "Shop", notesKey: "Bags", dueKey: "2026-09-07", labelsKey: []any{"x"}}
	kept := withoutHidden(payload, map[work.Capability]bool{
		work.CapabilityNotes: true, work.CapabilityDueDate: true, work.CapabilityLabels: true,
	})
	if len(kept) != 1 || kept["title"] != "Shop" {
		t.Errorf("kept %v, want the title alone", kept)
	}
}

// A conversion's titles land at the first level under the entry the destination offers.
func TestAConversionsTitlesLandAtALevelTheDestinationOffers(t *testing.T) {
	for _, tc := range []struct {
		name      string
		structure map[string]any
		want      string
		creates   int
	}{
		{"everything offered", nil, "WORK_PACKAGE", 2},
		{"Tasks", tasksStructure(), "ACTIVITY", 2},
		{"List", structureOf([]string{"TASK"}, map[string][]string{"TASK": {"COMPLETION"}}), "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cases, world := newWorld()
			world.structure = tc.structure
			world.store.proposals[proposalID] = implied()
			if _, err := (AcceptSuggestion{Cases: cases}).Execute(context.Background(), person(),
				proposalID, map[string]any{"collection_id": containerTargetID.String()}); err != nil {
				t.Fatalf("accepting: %v", err)
			}
			if world.creates != tc.creates {
				t.Fatalf("%d creates, want %d", world.creates, tc.creates)
			}
			for _, call := range world.performed {
				if call.name == "CreateWorkItem" && tc.want != "" {
					if nodeType, _ := call.in["type"].(string); nodeType != tc.want {
						t.Errorf("a title landed as %s, want %s", nodeType, tc.want)
					}
				}
			}
		})
	}
}

// hierarchyFixture is the installation's topology: a task holds work packages and activities, a
// work package activities.
func hierarchyFixture(t *testing.T) *service.Hierarchy {
	t.Helper()
	profiles := []work.CapabilityProfile{
		{Type: work.ItemTask, Capabilities: []work.Capability{work.CapabilityCompletion, work.CapabilityNotes},
			AllowedChildTypes: []work.ItemType{work.ItemWorkPackage, work.ItemActivity}, MaxDepth: 3},
		{Type: work.ItemWorkPackage, Capabilities: []work.Capability{work.CapabilityCompletion},
			AllowedChildTypes: []work.ItemType{work.ItemActivity}, MaxDepth: 2},
		{Type: work.ItemActivity, Capabilities: []work.Capability{work.CapabilityCompletion}, MaxDepth: 1},
	}
	hierarchy, err := service.NewHierarchy(profiles, profiles)
	if err != nil {
		t.Fatal(err)
	}
	return &hierarchy
}

// hiddenNotes answers an entry whose collection hides its notes.
type hiddenNotes struct{}

func (hiddenNotes) Invoke(_ context.Context, name string, _ appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
	if name != "GetWorkItem" {
		return usecase.Output{}, nil
	}
	return usecase.Output{
		"title": "Shop", "notes": "The code for the door is 4711",
		"hidden": map[string]any{"level": false, "capabilities": []string{"NOTES", "LABELS"}},
	}, nil
}

// Notes the collection hides are kept, and not sent to a provider.
func TestHiddenNotesAreNotSentToTheProvider(t *testing.T) {
	material, err := CatalogueSources{Catalogue: hiddenNotes{}}.Material(
		context.Background(), person(), Question{TargetType: domain.TargetWorkItem, TargetID: targetID, PromptID: "suggest-item-fields"})
	if err != nil {
		t.Fatalf("reading the material: %v", err)
	}
	if strings.Contains(material.Content, "4711") {
		t.Errorf("hidden notes went to the provider: %q", material.Content)
	}
	if !material.Hidden[work.CapabilityNotes] {
		t.Error("the material does not know the notes are hidden")
	}
}
