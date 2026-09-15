// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n

import (
	"strings"
	"testing"
	"testing/fstest"
)

func directory(t *testing.T, files map[string]string) map[string]Catalogue {
	t.Helper()
	tree := fstest.MapFS{}
	for name, content := range files {
		tree[name] = &fstest.MapFile{Data: []byte(content)}
	}
	catalogues, err := LoadDirectory(tree)
	if err != nil {
		t.Fatalf("loading the directory: %v", err)
	}
	return catalogues
}

func TestADirectoryIsReadOneCatalogueASFile(t *testing.T) {
	catalogues := directory(t, map[string]string{
		"en.json":    `{"_comment": "source", "a.one": "One"}`,
		"de-AT.json": `{"a.one": "Eins"}`,
		"README.md":  "not a catalogue",
	})

	if len(catalogues) != 2 {
		t.Fatalf("%d catalogues, want en and de-at", len(catalogues))
	}
	if _, present := catalogues["de-at"]; !present {
		t.Error("the tag is not keyed lower-cased")
	}
	if catalogues["en"].Has("_comment") {
		t.Error("the note to the translators is offered as a message")
	}
}

// A file nobody can ask for is a typo, and a typo that is skipped is a translation that goes
// missing without a trace.
func TestAFileWhoseNameIsNotATagIsRefused(t *testing.T) {
	for name, content := range map[string]string{
		"9.json":     `{}`,
		"de_DE.json": `{}`,
		"en.json":    `["not", "a", "map"]`,
		"de.json":    `{"a.one": 1}`,
	} {
		_, err := LoadDirectory(fstest.MapFS{name: &fstest.MapFile{Data: []byte(content)}})
		if err == nil {
			t.Errorf("%s %s: accepted, want a refusal", name, content)
			continue
		}
		if !strings.Contains(err.Error(), name) {
			t.Errorf("%s: the error does not name the file: %v", name, err)
		}
	}
}

// The fallback chain of i18n-l10n.md §3, proved with a half-translated locale: a translated key
// renders translated, a missing one renders the source language, and never the key.
func TestAHalfTranslatedCatalogueFallsBackKeyByKey(t *testing.T) {
	embedded := directory(t, map[string]string{
		"en.json": `{"a.one": "One", "a.two": "Two"}`,
		"de.json": `{"a.one": "Eins"}`,
	})
	renderer, err := newRenderer(embedded, nil)
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}

	for _, tc := range []struct{ locale, code, want string }{
		{"de", "a.one", "Eins"},
		{"de-AT", "a.one", "Eins"},
		{"de", "a.two", "Two"},
		{"de", "a.three", "a.three"},
		{"fr", "a.one", "One"},
	} {
		if got := renderer.Render(tc.locale, tc.code, nil); got != tc.want {
			t.Errorf("%s %s rendered %q, want %q", tc.locale, tc.code, got, tc.want)
		}
	}
}

// An operator's directory overrides key by key: a file for a tag the binary carries corrects the
// sentences it names and leaves the rest, and a file for a new tag is a new locale (§1).
func TestAnOverrideDirectoryIsLaidOverKeyByKey(t *testing.T) {
	embedded := directory(t, map[string]string{
		"en.json": `{"a.one": "One", "a.two": "Two"}`,
		"de.json": `{"a.one": "Eins", "a.two": "Zwei"}`,
	})
	overrides := directory(t, map[string]string{
		"de.json": `{"a.two": "Zwo"}`,
		"ar.json": `{"a.one": "واحد"}`,
	})
	renderer, err := newRenderer(embedded, overrides)
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}

	if got := renderer.Render("de", "a.one", nil); got != "Eins" {
		t.Errorf("an untouched key lost its embedded translation: %q", got)
	}
	if got := renderer.Render("de", "a.two", nil); got != "Zwo" {
		t.Errorf("the override did not win: %q", got)
	}
	if got := renderer.Render("ar", "a.one", nil); got != "واحد" {
		t.Errorf("the new locale did not arrive: %q", got)
	}
	if got := renderer.Render("ar", "a.two", nil); got != "Two" {
		t.Errorf("the new locale did not fall back to the source: %q", got)
	}

	want := []string{"en", "ar", "de"}
	if got := renderer.Locales(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Locales() = %v, want %v (the source first, the rest sorted)", got, want)
	}
}

// Without the source language there is nothing to fall back to, and a build that lacks it is
// refused rather than one that renders codes.
func TestTheSourceLanguageIsRequired(t *testing.T) {
	only := directory(t, map[string]string{"de.json": `{"a.one": "Eins"}`})
	if _, err := newRenderer(only, nil); err == nil {
		t.Error("a renderer without the source language was built")
	}
}

// One catalogue for one locale, as the CLI holds it: the locale's messages over the source's.
func TestForAnswersTheMergedCatalogue(t *testing.T) {
	embedded := directory(t, map[string]string{
		"en.json": `{"a.one": "One", "a.two": "Two"}`,
		"de.json": `{"a.one": "Eins"}`,
	})
	renderer, err := newRenderer(embedded, nil)
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}

	german := renderer.For("de-CH")
	if message, _ := german.Message("a.one", nil); message != "Eins" {
		t.Errorf("a.one = %q", message)
	}
	if message, _ := german.Message("a.two", nil); message != "Two" {
		t.Errorf("a.two = %q, want the source", message)
	}
	if !german.Has("a.two") {
		t.Error("the merged catalogue does not know a source key")
	}
	if unknown := renderer.For("xx"); unknown.Len() != 2 {
		t.Errorf("an unknown locale is not the source catalogue: %d messages", unknown.Len())
	}
}

// A key written twice is a message that says two things, and a map keeps the last without a
// word. Found in en.json by this very check: four codes, one of them with two different sentences.
func TestAKeyWrittenTwiceRefusesTheFile(t *testing.T) {
	_, err := LoadDirectory(fstest.MapFS{"en.json": &fstest.MapFile{
		Data: []byte(`{"a.one": "One", "a.two": "Two", "a.one": "Eins"}`),
	}})
	if err == nil || !strings.Contains(err.Error(), "a.one") {
		t.Errorf("a duplicate key was not refused by name: %v", err)
	}
}
