// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import "testing"

func TestThreeWayNeverOverwritesWhatThePlaceChangedWithoutShowingIt(t *testing.T) {
	base := Document{"types": Section{
		"incident": Block{"name": "Incident", "icon": "alert"},
		"problem":  Block{"name": "Problem"},
		"request":  Block{"name": "Request"},
	}}
	ours := Document{"types": Section{
		"incident": Block{"name": "Störung", "icon": "alert"},
		"problem":  Block{"name": "Problem"},
		"request":  Block{"name": "Anfrage"},
	}}
	theirs := Document{"types": Section{
		"incident": Block{"name": "Incident (P1-P4)", "icon": "siren"},
		"change":   Block{"name": "Change"},
	}}
	got := map[string]UpgradeState{}
	for _, change := range ThreeWay(base, ours, theirs) {
		got[change.Path()] = change.State
	}
	want := map[string]UpgradeState{
		"types.incident.icon": UpgradeTake,     // only the template changed it
		"types.incident.name": UpgradeConflict, // both did, differently
		"types.change.name":   UpgradeTake,     // new in the template
		"types.problem":       UpgradeTake,     // dropped, untouched here
		"types.request":       UpgradeConflict, // dropped, but changed here
	}
	if len(got) != len(want) {
		t.Fatalf("changes %v, want %v", got, want)
	}
	for path, state := range want {
		if got[path] != state {
			t.Errorf("%s: %s, want %s", path, got[path], state)
		}
	}

	byDefault := UpgradeDocument(ThreeWay(base, ours, theirs), nil)
	if _, taken := byDefault["types"]["incident"]["name"]; taken {
		t.Error("a conflict was taken without being chosen")
	}
	if byDefault["types"]["incident"]["icon"] != "siren" {
		t.Error("a take was left")
	}
	if block, named := byDefault["types"]["problem"]; !named || block != nil {
		t.Error("a dropped block is not removed")
	}

	chosen := UpgradeDocument(ThreeWay(base, ours, theirs), []string{"types.incident.name"})
	if chosen["types"]["incident"]["name"] != "Incident (P1-P4)" || len(chosen["types"]) != 1 {
		t.Errorf("chosen %v", chosen)
	}
}

func TestProvenance(t *testing.T) {
	if got := (Template{Key: "itil_desk", Version: 3}).Provenance(); got != "itil_desk@3" {
		t.Errorf("provenance %q", got)
	}
}
