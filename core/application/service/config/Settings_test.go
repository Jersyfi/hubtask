// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// signedInWorkspace is a workspace that tightened the password's length and set its imprint,
// left the rest of its sign-in rule to the installation, and chose its own language.
func signedInWorkspace(f fixture) {
	f.world.structure = map[string]any{}
	cell := func(value any, source string) map[string]any {
		return map[string]any{"value": value, "source": source, "lock": nil}
	}
	f.world.workspace = map[string]any{
		"default_locale": "de-DE",
		"sign_in_policy": map[string]any{
			"password": map[string]any{
				"min_length": cell(float64(14), "WORKSPACE"),
				"min_digits": cell(float64(1), "INSTANCE"),
			},
			"mfa_required_for": cell("ADMINS", "DEFAULT"),
			"legal": map[string]any{
				"imprint_url": cell("https://example.org/imprint", "WORKSPACE"),
			},
		},
	}
	f.world.ai = map[string]any{
		"source": "OWN", "sources": "OWN_ONLY", "sources_origin": "WORKSPACE",
		"person_opt_out": "OFFERED", "person_opt_out_origin": "DEFAULT",
	}
}

func TestTheCatalogueSettingsTheWorkspaceDecidedAreInTheDocument(t *testing.T) {
	f := newFixture()
	signedInWorkspace(f)
	document := invoke(t, ExportConfig{f.desk}.Descriptor(), person, nil)["document"].(map[string]any)
	settings := document["settings"].(map[string]any)
	for key, want := range map[string]any{
		"sign_in.min_length": float64(14), "legal.imprint_url": "https://example.org/imprint",
		"localisation.locale": "de-DE", "ai.sources": "OWN_ONLY",
	} {
		block, held := settings[key].(map[string]any)
		if !held || block["value"] != want {
			t.Errorf("%s = %v, want %v", key, settings[key], want)
		}
	}
	for _, key := range []string{"sign_in.min_digits", "sign_in.mfa_required_for", "ai.person_opt_out", "localisation.time_zone"} {
		if _, held := settings[key]; held {
			t.Errorf("%s is left to the level above but is in the document", key)
		}
	}
}

func TestSignInChangesAreOneCallCarryingTheProof(t *testing.T) {
	f := newFixture()
	signedInWorkspace(f)
	desired := map[string]any{"settings": map[string]any{
		"sign_in.min_length":       map[string]any{"value": float64(16)},
		"sign_in.mfa_required_for": map[string]any{"value": "EVERYONE"},
		"legal.privacy_url":        map[string]any{"value": "https://example.org/privacy"},
		"localisation.time_zone":   map[string]any{"value": "Europe/Berlin"},
	}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": desired})
	if plan["step_up"] != true {
		t.Fatalf("a plan changing the sign-in rule does not say it needs a fresh proof: %v", plan)
	}
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{
		"document": desired, "plan_hash": plan["plan_hash"], "step_up_token": "proof",
	})
	calls := f.world.inputs["UpdateWorkspace"]
	if len(calls) != 1 {
		t.Fatalf("UpdateWorkspace ran %d times, want once: %v", len(calls), calls)
	}
	in := calls[0]
	if in["step_up_token"] != "proof" || in["default_time_zone"] != "Europe/Berlin" {
		t.Errorf("the call lacks the proof or the zone: %v", in)
	}
	patch := in["sign_in_policy"].(map[string]any)
	if patch["min_length"] != float64(16) || patch["mfa_required_for"] != "EVERYONE" || patch["privacy_url"] != "https://example.org/privacy" {
		t.Errorf("the sign-in patch is incomplete: %v", patch)
	}
}

func TestASignInSwitchIsNotHandedBackAndAnUnknownSettingIsRefused(t *testing.T) {
	f := newFixture()
	signedInWorkspace(f)
	for _, desired := range []map[string]any{
		{"settings": map[string]any{"sign_in.min_length": nil}},
		{"settings": map[string]any{"sign_in.rotation_from": map[string]any{"value": "now"}}},
		{"settings": map[string]any{"ai.sources": map[string]any{"value": "OWN_ONLY", "colour": "red"}}},
	} {
		out := invoke(t, ValidateConfig{f.desk}.Descriptor(), person, usecase.Input{"document": desired})
		if out["valid"] != false {
			t.Errorf("%v is valid", desired)
		}
	}
}

func TestRemovingTheAiNarrowingTakesWhatTheInstallationAllows(t *testing.T) {
	f := newFixture()
	signedInWorkspace(f)
	desired := map[string]any{"settings": map[string]any{"ai.sources": nil}}
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": desired})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"document": desired, "plan_hash": plan["plan_hash"]})
	calls := f.world.inputs["ChooseAiSource"]
	if len(calls) != 1 || calls[0]["sources"] != "INHERIT" || calls[0]["source"] != "OWN" {
		t.Errorf("ChooseAiSource got %v", calls)
	}
}

func TestASettingLeftToTheLevelAboveIsExplained(t *testing.T) {
	f := newFixture()
	signedInWorkspace(f)
	out := invoke(t, ExplainConfig{f.desk}.Descriptor(), person, usecase.Input{"key": "settings.sign_in.min_digits"})
	if out["source"] != "INSTANCE" || out["operation"] != "UpdateWorkspace" || out["permission"] != "MANAGE_MEMBERS" {
		t.Errorf("explained as %v", out)
	}
}
