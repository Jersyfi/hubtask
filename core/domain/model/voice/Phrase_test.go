// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"reflect"
	"testing"
)

func TestASentenceBecomesThingsAndAPlace(t *testing.T) {
	cases := []struct {
		sentence string
		language Language
		want     Addition
	}{
		{"Butter auf die Einkaufsliste", German, Addition{Things: []Thing{{Title: "Butter"}}, Target: "Einkaufsliste"}},
		{"Reis auf die Metro-Liste", German, Addition{Things: []Thing{{Title: "Reis"}}, Target: "Metro-Liste"}},
		{"zwei Liter Milch, Eier und Brot auf Einkauf", German, Addition{Things: []Thing{{Title: "Milch", Amount: "2 l"}, {Title: "Eier"}, {Title: "Brot"}}, Target: "Einkauf"}},
		{"Brot für Oma auf die Einkaufsliste", German, Addition{Things: []Thing{{Title: "Brot für Oma"}}, Target: "Einkaufsliste"}},
		{"eine Gurke", German, Addition{Things: []Thing{{Title: "Gurke"}}}},
		{"3 Äpfel", German, Addition{Things: []Thing{{Title: "Äpfel", Amount: "3"}}}},
		{"a bag of rice and milk to the shopping list", English, Addition{Things: []Thing{{Title: "Rice", Amount: "1 bag"}, {Title: "Milk"}}, Target: "shopping list"}},
		{"Butter.", German, Addition{Things: []Thing{{Title: "Butter"}}}},
	}
	for _, each := range cases {
		if got := ParseAddition(each.sentence, each.language); !reflect.DeepEqual(got, each.want) {
			t.Errorf("ParseAddition(%q) = %+v, want %+v", each.sentence, got, each.want)
		}
	}
}

func TestAPlaceIsNamedWithoutItsListWord(t *testing.T) {
	cases := map[string]string{
		"die Einkaufsliste": "einkauf",
		"Metro-Liste":       "metro",
		"der Metro Liste":   "metro",
		"Büro":              "buero",
		"Liste":             "liste",
	}
	for spoken, want := range cases {
		if got := TargetWithout(spoken, German); got != want {
			t.Errorf("TargetWithout(%q) = %q, want %q", spoken, got, want)
		}
	}
	if got := TargetWithout("the shopping list", English); got != "shopping" {
		t.Errorf("English: %q", got)
	}
}

func TestAPickNamesTheThingAndMaybeItsPlace(t *testing.T) {
	if got := ParsePick("Butter von der Einkaufsliste", German); got != (Pick{Title: "Butter", Target: "Einkaufsliste"}) {
		t.Errorf("got %+v", got)
	}
	if got := ParsePick("Butter", German); got != (Pick{Title: "Butter"}) {
		t.Errorf("got %+v", got)
	}
	if got := ParsePick("milk off the shopping list", English); got != (Pick{Title: "milk", Target: "shopping list"}) {
		t.Errorf("got %+v", got)
	}
}

func TestLanguageAndFolding(t *testing.T) {
	if LanguageOf("de-DE") != German || LanguageOf("en-GB") != English || LanguageOf("fr-FR") != English {
		t.Error("the language of a locale")
	}
	if Fold("Groß-Einkauf!") != "gross einkauf" {
		t.Errorf("Fold: %q", Fold("Groß-Einkauf!"))
	}
}
