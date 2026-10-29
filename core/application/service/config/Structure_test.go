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
