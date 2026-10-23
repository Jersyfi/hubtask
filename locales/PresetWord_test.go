// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package locales

import (
	"encoding/json"
	"strings"
	"testing"
)

func catalogue(t *testing.T, name string) map[string]string {
	t.Helper()
	raw, err := Files.ReadFile(name)
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	var messages map[string]string
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	return messages
}

// A preset and a template are two concepts with two German words: *Voreinstellung* and *Vorlage*.
// Every message about a preset is translated - the first start's question with them, which offers
// hubs and collections on presets - and none calls one a *Vorlage*.
func TestAPresetHasItsOwnGermanWord(t *testing.T) {
	en, de := catalogue(t, "en.json"), catalogue(t, "de.json")
	for code, source := range en {
		aboutPreset := strings.HasPrefix(code, "presets.") || strings.HasPrefix(code, "app.presets.") ||
			strings.HasPrefix(code, "app.instance.presets_") || strings.HasPrefix(code, "app.structure.preset") ||
			strings.HasPrefix(code, "app.starter.")
		if !aboutPreset {
			continue
		}
		text, ok := de[code]
		if !ok {
			t.Errorf("de.json has no %s (%q)", code, source)
			continue
		}
		if strings.Contains(strings.ToLower(source), "preset") && strings.Contains(text, "Vorlage") {
			t.Errorf("de.json %s calls a preset a template: %s", code, text)
		}
	}
}
