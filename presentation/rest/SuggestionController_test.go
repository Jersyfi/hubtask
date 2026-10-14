// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Deciding a suggestion, and the body the route reads.
//
// `SuggestionAcceptance` carries what a person changed or added before accepting, and a proposal
// about a jumble entry *needs* one: accepting it converts the entry, and `ConvertJumbleEntry`
// requires a destination collection a model cannot name. The controller sent only the identifier,
// so every such acceptance answered `usecase.field_required` for a field the caller had sent - and
// a whole class of suggestion could not be accepted over REST at all.

const decidedSuggestion = "0192f000-0000-7000-8000-000000000f11"

func acceptedSuggestion() usecase.Output {
	return usecase.Output{
		"id": decidedSuggestion, "kind": "FIELDS", "status": "ACCEPTED",
		"target_type": "JUMBLE_ENTRY", "target_id": "0192f000-0000-7000-8000-000000000e11",
		"source": "AI", "model": "stub-1", "prompt_id": "suggest-fields", "prompt_version": "v1",
		"payload": map[string]any{"title": "Order 42"}, "version": 2,
	}
}

func suggestionRequest(
	t *testing.T, registry UseCaseRegistry, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})

	request := httptest.NewRequestWithContext(ctx, http.MethodPost, APIBasePath+path,
		strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestAcceptingCarriesTheOverridesToTheUseCase(t *testing.T) {
	registry := &catalogue{out: acceptedSuggestion()}

	recorder := suggestionRequest(t, registry, "/suggestions/"+decidedSuggestion+":accept",
		`{"overrides":{"collection_id":"0192f000-0000-7000-8000-00000000000c","title":"mine"}}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != acceptSuggestionUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	overrides, held := registry.in["overrides"].(map[string]any)
	if !held {
		t.Fatalf("the overrides did not reach the use case: %v", registry.in)
	}
	if overrides["collection_id"] != "0192f000-0000-7000-8000-00000000000c" {
		t.Errorf("the destination did not travel: %v", overrides)
	}
	if overrides["title"] != "mine" {
		t.Errorf("what the person changed did not travel: %v", overrides)
	}
}

// An acceptance with nothing to add sends no overrides rather than an empty object: the two are
// different requests to a use case that merges what it is given.
func TestAcceptingWithoutABodySendsNoOverrides(t *testing.T) {
	registry := &catalogue{out: acceptedSuggestion()}

	recorder := suggestionRequest(t, registry, "/suggestions/"+decidedSuggestion+":accept", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if _, held := registry.in["overrides"]; held {
		t.Errorf("an empty acceptance sent %v", registry.in["overrides"])
	}
}

// A dismissal carries none by construction, and must not be given one.
func TestDismissingCarriesNothingButTheIdentifier(t *testing.T) {
	registry := &catalogue{out: acceptedSuggestion()}

	recorder := suggestionRequest(t, registry, "/suggestions/"+decidedSuggestion+":dismiss", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != dismissSuggestionUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	if len(registry.in) != 1 || registry.in["suggestion_id"] != decidedSuggestion {
		t.Errorf("a dismissal sent %v", registry.in)
	}
}

// A body that is not the documented shape is a refusal rather than a silently ignored one.
func TestAMalformedAcceptanceIsRefused(t *testing.T) {
	registry := &catalogue{out: acceptedSuggestion()}

	recorder := suggestionRequest(t, registry, "/suggestions/"+decidedSuggestion+":accept",
		`{"overrides": "not an object"}`)

	if recorder.Code < http.StatusBadRequest {
		t.Fatalf("status %d, want a refusal: %s", recorder.Code, recorder.Body)
	}
	if registry.name != "" {
		t.Errorf("a malformed body reached the use case as %q", registry.name)
	}
}

// Every AI ask answers the job it queued: the provider has not been asked yet, and the job is
// where a refusal it meets later - a budget spent since, AI switched off - is said.
func TestEveryAiAskAnswersTheJobItQueued(t *testing.T) {
	const entity = "0192f000-0000-7000-8000-000000000e11"
	const queued = "0192f000-0000-7000-8000-00000000ac01"
	for _, ask := range []struct{ path, body string }{
		{"/items/" + entity + ":suggest-fields", ""},
		{"/items/" + entity + ":summarize", ""},
		{"/items/" + entity + ":classify", ""},
		{"/items/" + entity + ":summarize-thread", ""},
		{"/items/" + entity + ":decompose", ""},
		{"/containers/" + entity + ":summarize", ""},
		{"/jumble/entries/" + entity + ":suggest", ""},
		{"/templates:generate", `{"collection_id":"` + entity + `","description":"onboarding"}`},
	} {
		t.Run(ask.path, func(t *testing.T) {
			registry := &catalogue{out: usecase.Output{"job_id": queued}}

			recorder := suggestionRequest(t, registry, ask.path, ask.body)
			if recorder.Code != http.StatusAccepted {
				t.Fatalf("status %d, want 202: %s", recorder.Code, recorder.Body)
			}
			var body struct {
				JobID  string `json:"job_id"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decoding %q: %v", recorder.Body, err)
			}
			if body.JobID != queued || body.Status != "QUEUED" {
				t.Errorf("the answer is %+v, want the queued job", body)
			}
		})
	}
}

// What the narrowing dropped travels on the answer, so that a client can say a draft
// is not the whole of what the provider answered.
func TestTheAnswerCarriesWhatTheNarrowingDropped(t *testing.T) {
	out := acceptedSuggestion()
	out["kind"] = "TEMPLATE"
	out["dropped_nodes"] = 3
	registry := &catalogue{out: out}

	recorder := suggestionRequest(t, registry, "/suggestions/"+decidedSuggestion+":dismiss", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	var body struct {
		DroppedNodes *int `json:"dropped_nodes"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body.DroppedNodes == nil || *body.DroppedNodes != 3 {
		t.Errorf("dropped_nodes = %v, want 3", body.DroppedNodes)
	}
}
