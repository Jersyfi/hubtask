// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const completionItemID = "0192f000-0000-7000-8000-00000000000e"

func completedItemOutput() usecase.Output {
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	completedAt := at.Add(time.Hour)

	return usecase.Output{
		"id":            completionItemID,
		"type":          "ACTIVITY",
		"collection_id": "0192f000-0000-7000-8000-00000000000b",
		"parent_id":     "0192f000-0000-7000-8000-00000000000f",
		"path":          "/0192f000-0000-7000-8000-00000000000f/" + completionItemID + "/",
		"depth":         2,
		"title":         "Order the cable",
		"completion": map[string]any{
			"is_completed": true,
			"completed_at": completedAt,
			"completed_by": "0192f000-0000-7000-8000-00000000000d",
		},
		"order_key":  "a0",
		"created_by": "0192f000-0000-7000-8000-00000000000d",
		"created_at": at,
		"updated_at": completedAt,
		"version":    4,
	}
}

// action issues one of the two completion POSTs through the router, so that the path parameter is bound the
// way production binds it.
func action(t *testing.T, registry UseCaseRegistry, suffix, body string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequestWithContext(
		ctx, http.MethodPost, APIBasePath+"/items/"+completionItemID+":"+suffix, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestBothCompletionActionsReachTheirUseCase(t *testing.T) {
	cases := map[string]struct {
		suffix string
		want   string
	}{
		"complete": {"complete", completeWorkItemUseCase},
		"reopen":   {"reopen", reopenWorkItemUseCase},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			registry := &catalogue{out: completedItemOutput()}

			response := action(t, registry, c.suffix, "")
			if response.Code != http.StatusOK {
				t.Fatalf("status %d: %s", response.Code, response.Body)
			}
			if registry.name != c.want {
				t.Errorf("the handler ran %q, want %q", registry.name, c.want)
			}
			if registry.in["item_id"] != completionItemID {
				t.Errorf("the catalogue was asked about %v", registry.in["item_id"])
			}
			// The version after the change, which is what a client needs to follow the completion with an
			// edit. A roll-up may have moved it by more than one, so it comes from the answer rather than
			// from anything the request knew.
			if got := response.Header().Get("ETag"); got != `"4"` {
				t.Errorf("ETag %q, want the version after the change", got)
			}
		})
	}
}

// The body is optional in the contract, and its absence is not an error - which is the distinction between
// tolerating a missing body and accepting a malformed one.
func TestCompletingWithNoBodySucceeds(t *testing.T) {
	registry := &catalogue{out: completedItemOutput()}

	if response := action(t, registry, "complete", ""); response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	// Absent rather than false: the catalogue refuses only what the client actually asked for.
	if _, present := registry.in["cascade_children"]; present {
		t.Error("an absent body reached the catalogue as a cascade_children entry")
	}
}

func TestCascadeChildrenIsPassedOnWhenSent(t *testing.T) {
	for _, sent := range []bool{true, false} {
		registry := &catalogue{out: completedItemOutput()}

		body := `{"cascade_children":true}`
		if !sent {
			body = `{"cascade_children":false}`
		}
		if response := action(t, registry, "complete", body); response.Code != http.StatusOK {
			t.Fatalf("status %d: %s", response.Code, response.Body)
		}
		if registry.in["cascade_children"] != sent {
			t.Errorf("cascade_children=%v reached the catalogue as %v", sent, registry.in["cascade_children"])
		}
	}
}

// A field the contract does not declare is refused rather than ignored, as everywhere else a body is read:
// a client that misspelled one and got a 200 has no way to find out.
func TestAnUnknownFieldInTheCompletionBodyIsRefused(t *testing.T) {
	registry := &catalogue{out: completedItemOutput()}

	response := action(t, registry, "complete", `{"cascade_parents":true}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422: %s", response.Code, response.Body)
	}
	if registry.invoked {
		t.Error("the catalogue was called with an unknown field in hand")
	}
}

func TestAMalformedCompletionBodyIsRefused(t *testing.T) {
	registry := &catalogue{out: completedItemOutput()}

	response := action(t, registry, "complete", `{"cascade_children":`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", response.Code, response.Body)
	}
	if registry.invoked {
		t.Error("the catalogue was called with a malformed body in hand")
	}
}

// The completed item comes back in full, so that a client learns the state it now has rather than assuming
// the one it asked for - a roll-up may have changed more than it knows about.
func TestTheCompletedItemComesBackInTheResponse(t *testing.T) {
	response := action(t, &catalogue{out: completedItemOutput()}, "complete", "")

	var body struct {
		ID         string `json:"id"`
		Version    int    `json:"version"`
		Completion struct {
			IsCompleted bool       `json:"is_completed"`
			CompletedAt *time.Time `json:"completed_at"`
			CompletedBy *string    `json:"completed_by"`
		} `json:"completion"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("the body is not an item: %v", err)
	}

	if body.ID != completionItemID || body.Version != 4 {
		t.Errorf("the body describes %s at version %d", body.ID, body.Version)
	}
	if !body.Completion.IsCompleted {
		t.Error("the response says the item is open")
	}
	if body.Completion.CompletedAt == nil || body.Completion.CompletedBy == nil {
		t.Errorf("the completion is %+v, want who and when", body.Completion)
	}
}

// A refusal from the application layer - a capability the type does not have, an archived collection, a
// version that moved - reaches the client as its own problem document, with no ETag on it.
func TestARefusedCompletionAnswersTheProblem(t *testing.T) {
	registry := &catalogue{
		err: shared.ErrCapabilityNotSupported.WithDetail("items.capability_not_supported"),
	}

	response := action(t, registry, "complete", "")
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422: %s", response.Code, response.Body)
	}
	if tag := response.Header().Get("ETag"); tag != "" {
		t.Errorf("a refused completion answered with the ETag %q", tag)
	}

	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.DetailCode != "items.capability_not_supported" {
		t.Errorf("detail code %q", problem.DetailCode)
	}
}

func TestAnUnwiredCompletionAnswers500(t *testing.T) {
	controller := NewRestController()

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, httptest.NewRequestWithContext(
		t.Context(), http.MethodPost, APIBasePath+"/items/"+completionItemID+":complete", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500: %s", recorder.Code, recorder.Body)
	}
}
