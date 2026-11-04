// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package locales_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/config"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
)

// Every word a shipped template writes - the names of its types, fields, options, statuses,
// steps, roles, views and rules, its samples' and pages' titles, its pages' and replies' bodies -
// has its code in each catalogue, so a German workspace taking a template gets German words
// (user-experience.md §4). The English catalogue renders each exactly as shipped, and a reply's
// {{placeholders}} survive the rendering in every language.
func TestEveryShippedTemplateWordHasItsWordsInEachCatalogue(t *testing.T) {
	files, err := filepath.Glob("../core/application/service/config/shipped/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no shipped template found: %v", err)
	}
	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("de.json")
	if err != nil {
		t.Fatal(err)
	}
	var german map[string]string
	if err := json.Unmarshal(raw, &german); err != nil {
		t.Fatal(err)
	}
	words := 0
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var template map[string]any
		if err := json.Unmarshal(raw, &template); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		delete(template, "key")
		for _, text := range templateWords(template) {
			words++
			code := config.TemplateWordCode(text)
			if said := renderer.Render("en", code, nil); said != text {
				t.Errorf("%s: en renders %s as %q, want the shipped %q", filepath.Base(file), code, said, text)
			}
			said := renderer.Render("de", code, nil)
			if german[code] == "" {
				t.Errorf("%s: de has no word for %q (%s)", filepath.Base(file), text, code)
			}
			if strings.Count(said, "{{") != strings.Count(text, "{{") {
				t.Errorf("%s: de renders %s with other placeholders: %q", filepath.Base(file), code, said)
			}
		}
	}
	if words < 600 {
		t.Errorf("%d words read, the walk no longer finds the templates' names", words)
	}
}

// templateWords are the strings under name, title and body, at any depth.
func templateWords(value any) []string {
	var out []string
	switch typed := value.(type) {
	case map[string]any:
		for key, inner := range typed {
			if text, ok := inner.(string); ok && (key == "name" || key == "title" || key == "body") && text != "" {
				out = append(out, text)
				continue
			}
			out = append(out, templateWords(inner)...)
		}
	case []any:
		for _, inner := range typed {
			out = append(out, templateWords(inner)...)
		}
	}
	return out
}
