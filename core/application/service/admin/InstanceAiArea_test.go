// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"slices"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The AI area of the instance level (ADR-0072 §2): read from a document, refused outside its
// closed sets, answered whole, and named in the journal when it moves.

func TestTheAiAreaIsReadWithItsLocks(t *testing.T) {
	level, err := InstanceLevelOf(map[string]any{
		"ai": map[string]any{
			"sources":          map[string]any{"value": "OFFERED", "locked": true},
			"min_jurisdiction": map[string]any{"value": "EEA"},
			"a_key_from_later": map[string]any{"value": "x"},
		},
	})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if level.Ai.Sources == nil || *level.Ai.Sources != integration.AiSourcesOffered || !level.Ai.Locks[integration.AiSwitchSources] {
		t.Errorf("sources read as %v, locks %v", level.Ai.Sources, level.Ai.Locks)
	}
	if level.Ai.MinJurisdiction == nil || *level.Ai.MinJurisdiction != integration.AiMinEEA ||
		level.Ai.Locks[integration.AiSwitchMinJurisdiction] {
		t.Errorf("the minimum read as %v", level.Ai.MinJurisdiction)
	}
}

func TestAnAiValueOutsideItsSetIsRefusedNamingTheKey(t *testing.T) {
	for _, document := range []map[string]any{
		{"sources": map[string]any{"value": "SOMETIMES"}},
		{"min_jurisdiction": map[string]any{"value": "MOON"}},
		{"sources": map[string]any{"value": 3}},
		{"sources": "EITHER"},
	} {
		_, err := InstanceLevelOf(map[string]any{"ai": document})
		if got := shared.AsError(err); got == nil || got.DetailCode != "auth.policy_value_invalid" {
			t.Errorf("%v answered %v", document, err)
		}
	}
}

func TestTheProjectionAnswersEveryAiKey(t *testing.T) {
	sources := integration.AiSourcesEither
	out := instanceLevelOutput(identityrepo.InstanceLevel{
		Ai: integration.AiLayer{Sources: &sources, Locks: map[integration.AiSwitch]bool{integration.AiSwitchSources: true}},
	})
	area, _ := out["ai"].(usecase.Output)
	if len(area) != len(integration.AiSwitches()) {
		t.Fatalf("the projection answers %d AI keys, want %d", len(area), len(integration.AiSwitches()))
	}
	decided, _ := area["sources"].(usecase.Output)
	if decided["set"] != true || decided["value"] != "EITHER" || decided["locked"] != true {
		t.Errorf("sources read %v", decided)
	}
	undecided, _ := area["min_jurisdiction"].(usecase.Output)
	if undecided["set"] != false {
		t.Errorf("an undecided minimum read %v", undecided)
	}
}

func TestMovingTheAiAreaIsNamedInTheJournal(t *testing.T) {
	before := identityrepo.InstanceLevel{}
	sources := integration.AiSourcesOffered
	after := identityrepo.InstanceLevel{Ai: integration.AiLayer{
		Sources: &sources, Locks: map[integration.AiSwitch]bool{integration.AiSwitchMinJurisdiction: true},
	}}
	moved := movedSwitches(before, after)
	for _, want := range []any{"ai.sources", "ai.min_jurisdiction"} {
		if !slices.Contains(moved, want) {
			t.Errorf("the journal does not name %v: %v", want, moved)
		}
	}
}

// UC-PRV-05 check 11: the installation holds a workspace at OFFERED, with or without a lock, and
// can never name NOT_OFFERED - that needs a legal basis only the workspace can give.
func TestTheInstallationHoldsThePersonOptOutAndNeverTakesItAway(t *testing.T) {
	level, err := InstanceLevelOf(map[string]any{
		"ai": map[string]any{"person_opt_out": map[string]any{"value": "OFFERED", "locked": true}},
	})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if level.Ai.PersonOptOut == nil || *level.Ai.PersonOptOut != integration.AiOptOutOffered ||
		!level.Ai.Locks[integration.AiSwitchPersonOptOut] {
		t.Errorf("read as %v, locks %v", level.Ai.PersonOptOut, level.Ai.Locks)
	}

	for value, want := range map[string]string{
		"NOT_OFFERED": "ai.opt_out_workspace_only",
		"SOMETIMES":   "auth.policy_value_invalid",
	} {
		_, err := InstanceLevelOf(map[string]any{
			"ai": map[string]any{"person_opt_out": map[string]any{"value": value, "locked": true}},
		})
		if got := shared.AsError(err); got == nil || got.DetailCode != want {
			t.Errorf("%s answered %v, want %s", value, err, want)
		}
	}

	moved := movedSwitches(identityrepo.InstanceLevel{}, level)
	if !slices.Contains(moved, any("ai.person_opt_out")) {
		t.Errorf("the journal does not name the key: %v", moved)
	}
	area, _ := instanceLevelOutput(level)["ai"].(usecase.Output)
	position, _ := area["person_opt_out"].(usecase.Output)
	if position["value"] != "OFFERED" || position["locked"] != true {
		t.Errorf("answered %v", position)
	}
}
