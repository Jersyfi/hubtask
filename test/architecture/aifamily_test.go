// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	usecases "github.com/Jersyfi/hubtask/core/application/catalogue"
)

// aiUseCasesOutsideTheSuggestionService are the use cases that ask or read AI from another
// package: the translation (work), the budget's state (integration) and the question about a
// jumble entry (jumble). Every use case of the suggestion service is one by being there.
var aiUseCasesOutsideTheSuggestionService = []string{"AiTranslate", "ReadAiBudgetState", "SuggestFromJumbleEntry"}

// TestTheAiFamilyIsTheCataloguesAiActions holds the web app's one declared list of AI action kinds
// - what the rule editor draws in the AI colours and leaves out where AI is off (UC-AI-01 check 1)
// - to the catalogue, both ways: every listed kind is an action the catalogue serves, and every
// action of an AI use case is listed. A new AI use case therefore lands in the family, and a renamed
// one cannot leave a kind behind that nothing serves.
func TestTheAiFamilyIsTheCataloguesAiActions(t *testing.T) {
	raw, err := os.ReadFile("../../apps/webapp/src/lib/automation/aiactions.json")
	if err != nil {
		t.Fatalf("reading the declared list: %v", err)
	}
	var listed []string
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatalf("the declared list is not a list of kinds: %v", err)
	}

	served := usecases.AutomationActions()
	ai := map[string]bool{}
	for _, descriptor := range usecases.Descriptors() {
		name := runtime.FuncForPC(reflect.ValueOf(descriptor.Handler).Pointer()).Name()
		if strings.Contains(name, "/core/application/service/suggestion.") ||
			slices.Contains(aiUseCasesOutsideTheSuggestionService, descriptor.Name) {
			ai[descriptor.AutomationAction()] = true
		}
	}

	for _, kind := range listed {
		if !slices.Contains(served, kind) {
			t.Errorf("%s is listed as AI and the catalogue serves no such action", kind)
		}
		if !ai[kind] {
			t.Errorf("%s is listed as AI and its use case is not one", kind)
		}
	}
	for kind := range ai {
		if !slices.Contains(listed, kind) {
			t.Errorf("%s is the action of an AI use case and missing from aiactions.json", kind)
		}
	}
	if !slices.IsSorted(listed) {
		t.Error("aiactions.json is not sorted: keep it in order so a change reads as one line")
	}
}
