// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"regexp"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/voice"
)

var slotReference = regexp.MustCompile(`\{([A-Za-z]+)\}`)

// The model is pasted into Amazon's console, which refuses it at Build for any of these - a
// refusal the administrator would meet, not a test.
func TestTheModelIsOneAlexaBuilds(t *testing.T) {
	for _, language := range []domain.Language{domain.German, domain.English} {
		model := AlexaModel(language, "")
		intents := model["interactionModel"].(map[string]any)["languageModel"].(map[string]any)["intents"].([]any)
		seen := map[string]string{}
		for _, raw := range intents {
			intent := raw.(map[string]any)
			name := intent["name"].(string)
			declared := map[string]bool{}
			if slots, ok := intent["slots"].([]any); ok {
				for _, each := range slots {
					declared[each.(map[string]any)["name"].(string)] = true
				}
			}
			for _, sample := range intent["samples"].([]any) {
				text := sample.(string)
				key := strings.ToLower(text)
				if other, twice := seen[key]; twice {
					t.Errorf("%s: %q is in %s and %s", language, text, other, name)
				}
				seen[key] = name
				if strings.Contains(text, "} {") {
					t.Errorf("%s: %q puts two slots side by side", language, text)
				}
				for _, match := range slotReference.FindAllStringSubmatch(text, -1) {
					if !declared[match[1]] {
						t.Errorf("%s: %q names slot %s, which %s does not declare", language, text, match[1], name)
					}
				}
				if strings.ContainsAny(text, ",;:!?\"") {
					t.Errorf("%s: %q carries punctuation Alexa refuses", language, text)
				}
			}
		}
	}
}

// A store is said with "bei", a project with "für": the sentences are not a shopping list's alone.
func TestTheModelSpeaksOfEveryKindOfPlace(t *testing.T) {
	model := AlexaModel(domain.German, "")
	intents := model["interactionModel"].(map[string]any)["languageModel"].(map[string]any)["intents"].([]any)
	samples := map[string]bool{}
	for _, raw := range intents {
		for _, sample := range raw.(map[string]any)["samples"].([]any) {
			samples[sample.(string)] = true
		}
	}
	for _, want := range []string{
		"{Thing} bei {List}", "{Thing} bei {List} kaufen", "{Thing} für {List}", "{Thing} in {List}",
		"trag {Thing} in die {List} ein", "was steht bei {List}", "{Thing} bei {List} ist erledigt",
	} {
		if !samples[want] {
			t.Errorf("the model has no %q", want)
		}
	}
}

// No workspace's names are in what is pasted into Amazon's console.
func TestThePastedModelNamesNoWorkspacesLists(t *testing.T) {
	german, english := AlexaModel(domain.German, ""), AlexaModel(domain.English, "")
	for _, model := range []map[string]any{german, english} {
		types := model["interactionModel"].(map[string]any)["languageModel"].(map[string]any)["types"].([]any)
		for _, value := range types[0].(map[string]any)["values"].([]any) {
			if _, hasID := value.(map[string]any)["id"]; hasID {
				t.Errorf("the static list type carries a workspace's place: %v", value)
			}
		}
	}
}
