// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import "testing"

func TestASpokenPlaceIsFound(t *testing.T) {
	einkauf := Place{CollectionID: "c1", Name: "Einkauf"}
	metro := Place{CollectionID: "c1", LabelID: "l1", Name: "Einkauf, Metro"}
	buero := Place{CollectionID: "c2", Name: "Büro"}
	baumarkt := Place{CollectionID: "c3", Name: "Baumarkt"}
	known := []Known{
		{Place: einkauf, Name: "Einkauf"},
		{Place: metro, Name: "Metro"},
		{Place: buero, Name: "Büro"},
		{Place: baumarkt, Name: "Baumarkt"},
		{Place: einkauf, Name: "Kühlschrank", IsAlias: true},
	}
	cases := map[string]Place{
		"die Einkaufsliste": einkauf,
		"Einkauf":           einkauf,
		"Metro-Liste":       metro,
		"die Metro":         metro,
		"Büro":              buero,
		"buero":             buero,
		"den Kühlschrank":   einkauf,
		"Baum":              baumarkt,
	}
	for spoken, want := range cases {
		got := Resolve(spoken, German, known)
		if !got.Found || got.Place != want {
			t.Errorf("Resolve(%q) = %+v, want %+v", spoken, got, want)
		}
	}
	if got := Resolve("Oma", German, known); got.Found || len(got.Ambiguous) != 0 {
		t.Errorf("an unknown place was found: %+v", got)
	}
	if got := Resolve("B", German, known); got.Found || len(got.Ambiguous) != 2 {
		t.Errorf("two places starting alike are not ambiguous: %+v", got)
	}
}
