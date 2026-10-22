// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

var threeLevels = []any{
	map[string]any{"type": "TASK"}, map[string]any{"type": "WORK_PACKAGE"}, map[string]any{"type": "ACTIVITY"},
}

func clippedFrom(out usecase.Output) (string, []any) {
	structure, _ := out["structure"].(map[string]any)
	settings, _ := structure["settings"].(map[string]any)
	counts, _ := settings["counts"].(map[string]any)
	clipped, _ := out["clipped_by_levels"].([]any)
	clip, _ := structure["counts_clipped"].(map[string]any)
	if clip != nil {
		from, _ := clip["from"].(map[string]any)
		if from["figure"] != "PROGRESS" || clip["reason"] != "ONE_LEVEL" {
			return "bad clip", clipped
		}
	}
	figure, _ := counts["figure"].(string)
	return figure, clipped
}

// UC-WRK-29 check 2: progress typed for a collection of one level is refused on every door, and
// nothing is written. The hub of the harness is a List: one level.
func TestProgressTypedForOneLevelIsRefusedOnEveryDoor(t *testing.T) {
	for _, kind := range []appshared.ActorKind{
		appshared.ActorUser,           // the web app and REST
		appshared.ActorServiceAccount, // hubctl with a token
		appshared.ActorAIAgent,        // MCP
		appshared.ActorAutomation,     // a rule, were one to write a structure
	} {
		t.Run(string(kind), func(t *testing.T) {
			h := newStructureHarness(t)
			caller := actor()
			caller.Kind = kind
			_, err := h.registry.Invoke(context.Background(), SetContainerStructureName, caller, usecase.Input{
				"container_id": collectionID.String(),
				"settings":     map[string]any{"counts": map[string]any{"figure": "PROGRESS"}},
			})
			refusal := shared.AsError(err)
			if refusal == nil || refusal.DetailCode != "structure.figure_not_offered" ||
				refusal.Category != shared.CategoryValidation {
				t.Fatalf("refused as %v", err)
			}
			if len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/settings/counts" {
				t.Errorf("fields %+v", refusal.Fields)
			}
			if len(h.store.written) != 0 || len(h.audit.entries) != 0 || len(h.events.appended) != 0 {
				t.Error("a refused change left something behind")
			}
		})
	}
}

// Progress typed together with the levels that carry it is accepted.
func TestProgressWithItsLevelsIsAccepted(t *testing.T) {
	h := newStructureHarness(t)
	out, err := h.invoke(t, usecase.Input{
		"container_id": collectionID.String(),
		"settings": map[string]any{
			"counts": map[string]any{"figure": "PROGRESS"}, "levels": threeLevels,
		},
	})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if figure, clipped := clippedFrom(out); figure != "PROGRESS" || len(clipped) != 0 {
		t.Errorf("in force %s, clipped %v", figure, clipped)
	}
}

// What a person did not type - a reset, a narrowing, a preset under a lock - is not refused: what
// counts is clipped to OPEN and the answer says so.
func TestAClipThePersonDidNotTypeIsReported(t *testing.T) {
	withOwnProgress := func(h *structureHarness) {
		collection := h.containers.stored[collectionID]
		collection.Structure.Values.Counts = &domain.Counts{Figure: domain.FigureProgress}
		collection.Structure.Values.Levels = []domain.Level{
			{Type: domain.ItemTask}, {Type: domain.ItemWorkPackage}, {Type: domain.ItemActivity},
		}
		h.containers.stored[collectionID] = collection
	}
	cases := map[string]struct {
		prepare func(*structureHarness)
		input   usecase.Input
	}{
		"a reset of the levels to the hub's one": {
			prepare: withOwnProgress,
			input:   usecase.Input{"container_id": collectionID.String(), "reset": []any{"levels"}},
		},
		"the levels narrowed to one": {
			prepare: withOwnProgress,
			input: usecase.Input{"container_id": collectionID.String(), "settings": map[string]any{
				"levels": []any{map[string]any{"type": "TASK"}},
			}},
		},
		"a preset whose levels a lock keeps at one": {
			prepare: func(h *structureHarness) {
				h.store.workspace = domain.WorkspaceStructure{
					Layer: domain.StructureLayer{Values: domain.StructureValues{Levels: []domain.Level{{Type: domain.ItemTask}}}},
					Locks: map[domain.LockKey]shared.ID{"levels": accountID},
				}
			},
			input: usecase.Input{"container_id": collectionID.String(), "preset": "PROJECT"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newStructureHarness(t)
			tc.prepare(h)
			out, err := h.invoke(t, tc.input)
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			figure, clipped := clippedFrom(out)
			if figure != "OPEN" || len(clipped) != 1 || clipped[0] != "counts" {
				t.Errorf("in force %s, clipped %v", figure, clipped)
			}
		})
	}
}

// The workspace refuses progress typed for its own single level, and accepts it with more.
func TestTheWorkspaceRefusesProgressForItsOwnSingleLevel(t *testing.T) {
	h, registry := newWorkspaceStructureHarness(t)
	h.store.workspace = domain.WorkspaceStructure{Layer: domain.StructureLayer{Values: domain.StructureValues{
		Levels: []domain.Level{{Type: domain.ItemTask}},
	}}}

	_, err := registry.Invoke(context.Background(), SetWorkspaceStructureName, actor(), usecase.Input{
		"settings": map[string]any{"counts": map[string]any{"figure": "PROGRESS"}},
	})
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "structure.figure_not_offered" {
		t.Fatalf("refused as %v", err)
	}
	if len(h.audit.entries) != 0 {
		t.Error("a refused change was audited")
	}

	_, err = registry.Invoke(context.Background(), SetWorkspaceStructureName, actor(), usecase.Input{
		"settings": map[string]any{"counts": map[string]any{"figure": "PROGRESS"}, "levels": threeLevels},
	})
	if err != nil {
		t.Fatalf("progress with three levels refused: %v", err)
	}
}
