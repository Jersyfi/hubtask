// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"sort"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	"github.com/Jersyfi/hubtask/infrastructure/ai"
)

// The gate K-01 exists for: what a prompt asks a provider for, and what the code keeps, are the
// same set.
//
// The two halves are a markdown file and a Go map, and until this test nothing read both.
// `suggest-fields` asked for `subtasks` from J-06 onwards and the allow list dropped the key, so
// every jumble suggestion paid a provider for an answer no code read - for four milestones, with
// §2's Jumble row and J-06's own acceptance both naming a field that never arrived. No compiler
// sees that, and a review sees it only if somebody happens to open both files at once.
//
// It is a *gate* rather than a rule in a document because the defect is silent in both directions.
// A key asked for and dropped is money spent on nothing; a key kept and never asked for is an allow
// list that has outlived its prompt and would accept whatever a model volunteered under that name.
func TestEveryPromptAsksForExactlyWhatTheCodeKeeps(t *testing.T) {
	store, err := ai.NewStore()
	if err != nil {
		t.Fatalf("the prompt store does not build: %v", err)
	}
	allowed := suggestion.AnswerKeys()

	for _, id := range store.IDs() {
		prompt, err := store.Get(id)
		if err != nil {
			t.Fatalf("%s is listed and not gettable: %v", id, err)
		}
		keeps, asked := allowed[id]

		if prompt.Published() {
			// A prompt written for an agent's client to operate (J-12) is not asked by this
			// product's own code, answers prose, and has no allow list. One that acquired one
			// would be a prompt read two ways.
			if asked {
				t.Errorf("%s is published for an agent and also has an allow list", id)
			}
			continue
		}
		if !asked {
			t.Errorf("%s is a completion prompt no code can ask: promptFields in "+
				"core/application/service/suggestion/Producing.go does not name it, and "+
				"Produce.Execute refuses a prompt it does not know", id)
			continue
		}

		asks := append([]string(nil), prompt.Answers...)
		sort.Strings(asks)
		if strings.Join(asks, ",") != strings.Join(keeps, ",") {
			t.Errorf("%s asks a provider for [%s] and the code keeps [%s].\n"+
				"Both halves are deliberate, so fix the one that is wrong: the answer shape in "+
				"infrastructure/ai/prompts/%s.v*.md, or promptFields in "+
				"core/application/service/suggestion/Producing.go.",
				id, strings.Join(asks, ", "), strings.Join(keeps, ", "), id)
		}
	}

	// And the other direction: an allow list entry naming a prompt this build does not carry is a
	// question nobody can ask - the job would answer ai.prompt_unknown at run time.
	for id := range allowed {
		if _, err := store.Get(id); err != nil {
			t.Errorf("the allow list names %s and the prompt store carries no such prompt", id)
		}
	}
}
