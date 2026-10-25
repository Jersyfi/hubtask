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

// A place is not always a list things go *on*: a store is where one buys, a project what one
// works for, a folder what things go in - and the verb at the end is how it was said.
func TestAPlaceIsNamedTheWayItsKindIsSpokenOf(t *testing.T) {
	cases := []struct {
		sentence string
		language Language
		things   []string
		target   string
	}{
		{"Gemüsebrühe bei Metro", German, []string{"Gemüsebrühe"}, "Metro"},
		{"Gemüsebrühe und Reis bei Metro kaufen", German, []string{"Gemüsebrühe", "Reis"}, "Metro"},
		{"Angebot schreiben für das Website-Projekt", German, []string{"Angebot schreiben"}, "Website-Projekt"},
		{"Steuerbescheid in den Ordner Büro", German, []string{"Steuerbescheid"}, "Ordner Büro"},
		{"Reifenwechsel beim Autohaus", German, []string{"Reifenwechsel"}, "Autohaus"},
		{"Kunden anrufen in Akquise ein", German, []string{"Kunden anrufen"}, "Akquise"},
		{"stock cubes at Costco", English, []string{"Stock cubes"}, "Costco"},
		{"call the client for the website project please", English, []string{"Call the client"}, "website project"},
	}
	for _, c := range cases {
		got := ParseAddition(c.sentence, c.language)
		titles := make([]string, 0, len(got.Things))
		for _, thing := range got.Things {
			titles = append(titles, thing.Title)
		}
		if !reflect.DeepEqual(titles, c.things) || got.Target != c.target {
			t.Errorf("ParseAddition(%q) = %v / %q, want %v / %q", c.sentence, titles, got.Target, c.things, c.target)
		}
	}
	if got := TargetWithout("Metro kaufen", German); got != "metro" {
		t.Errorf("TargetWithout kept the verb: %q", got)
	}
}
