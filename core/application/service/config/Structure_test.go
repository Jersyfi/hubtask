// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

func TestTheWorkspacesStructureIsASectionOfItsOwnSettings(t *testing.T) {
	f := newFixture()
	f.world.structure = map[string]any{"completion": "MANUAL"}
	document := invoke(t, ExportConfig{f.desk}.Descriptor(), person, nil)["document"].(map[string]any)
	structure := document["structure"].(map[string]any)
	if structure["completion"].(map[string]any)["value"] != "MANUAL" {
		t.Errorf("the workspace's own setting is not in the document: %v", structure)
	}
	if _, held := structure["views"]; held {
		t.Error("a setting left to the default is in the document")
	}
}

func TestAStructureSettingIsSetAndResetThroughItsUseCase(t *testing.T) {
	f := newFixture()
	f.world.structure = map[string]any{"completion": "MANUAL"}
	desired := map[string]any{"structure": map[string]any{
		"completion":  nil,
		"celebration": map[string]any{"value": "OFF"},
	}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": desired})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"document": desired, "plan_hash": plan["plan_hash"]})
	if _, held := f.world.structure["completion"]; held {
		t.Error("a removed block did not reset the setting")
	}
	if f.world.structure["celebration"] != "OFF" {
		t.Errorf("an added block did not set the setting: %v", f.world.structure)
	}
}

func TestAnUnknownStructureSettingIsRefused(t *testing.T) {
	f := newFixture()
	f.world.structure = map[string]any{}
	desired := map[string]any{"structure": map[string]any{"colour": map[string]any{"value": "RED"}}}
	out := invoke(t, ValidateConfig{f.desk}.Descriptor(), person, usecase.Input{"document": desired})
	if out["valid"] != false {
		t.Errorf("an unknown setting is valid: %v", out)
	}
}

func TestTheWorkspacesLocksAndDefaultPresetAreBlocksOfItsStructure(t *testing.T) {
	f := newFixture()
	f.world.structure = map[string]any{}
	f.world.locks = map[string]any{
		"views":                map[string]any{"origin": "WORKSPACE"},
		"fields.TASK.DUE_DATE": map[string]any{"origin": "WORKSPACE"},
		"completion":           map[string]any{"origin": "INSTANCE"},
	}
	f.world.defaultPreset = "TASKS"
	structure := invoke(t, ExportConfig{f.desk}.Descriptor(), person, nil)["document"].(map[string]any)["structure"].(map[string]any)
	locks, _ := structure["locks"].(map[string]any)["value"].([]any)
	if len(locks) != 2 || locks[0] != "fields.TASK.DUE_DATE" || locks[1] != "views" {
		t.Errorf("the workspace's own locks are %v, want its two and not the installation's", locks)
	}
	if structure["default_preset"].(map[string]any)["value"] != "TASKS" {
		t.Errorf("the default preset is %v", structure["default_preset"])
	}

	desired := map[string]any{"structure": map[string]any{
		"locks":          map[string]any{"value": []any{"views", "capture"}},
		"default_preset": nil,
	}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": desired})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"document": desired, "plan_hash": plan["plan_hash"]})
	calls := f.world.inputs["SetWorkspaceStructure"]
	if len(calls) != 2 {
		t.Fatalf("SetWorkspaceStructure ran %d times: %v", len(calls), calls)
	}
	for _, in := range calls {
		if list, held := in["locks"].([]any); held && (len(list) != 2 || list[1] != "capture") {
			t.Errorf("the locks were set to %v", list)
		}
		if _, held := in["locks"]; !held && in["clear_default_preset"] != true {
			t.Errorf("a removed default preset did not clear it: %v", in)
		}
	}
}
