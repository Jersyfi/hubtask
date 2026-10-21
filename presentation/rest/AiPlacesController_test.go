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

// A read sent as a POST: the title reaches the use case, the answer is a list under `items` that
// nothing between the server and the person may store, and an idempotency key is neither needed
// nor read.
func TestLookalikesAreAReadNobodyStores(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"items": []usecase.Output{{
		"item_id": "0192f000-0000-7000-8000-0000000000a1", "collection_id": containerID,
		"title": "Call the tiler", "similarity": 0.91,
	}}}}

	controller := NewRestController()
	controller.UseCases = registry
	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, APIBasePath+"/items:duplicates",
		strings.NewReader(`{"title":"Book the tiler"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "not-read-at-all")
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "FindDuplicatesByTitle" || registry.in["title"] != "Book the tiler" {
		t.Errorf("invoked %q with %v", registry.name, registry.in)
	}
	if _, held := registry.in["idempotency_key"]; held {
		t.Error("the idempotency key reached the use case")
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || len(body.Items) != 1 {
		t.Fatalf("body = %s (%v)", recorder.Body, err)
	}
	if body.Items[0]["title"] != "Call the tiler" {
		t.Errorf("item = %v", body.Items[0])
	}
}

// Nothing near is an empty list, not an absent one.
func TestNoLookalikeIsAnEmptyList(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"items": []usecase.Output{}}}
	recorder := containerRequest(t, registry, http.MethodPost, "/items:duplicates", `{"title":"Book"}`, "")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"items":[]`) {
		t.Errorf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
}

// The budget's state in the contract's words: the state, and the moment or null.
func TestTheBudgetStateIsAWordAndAMoment(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"state": "NEARLY_SPENT", "renews_at": "2030-03-31T00:00:00Z"}}
	recorder := containerRequest(t, registry, http.MethodGet, "/ai-budget-state", "", "")
	if recorder.Code != http.StatusOK || registry.name != "ReadAiBudgetState" {
		t.Fatalf("status = %d, use case %q, body = %s", recorder.Code, registry.name, recorder.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["state"] != "NEARLY_SPENT" || body["renews_at"] != "2030-03-31T00:00:00Z" {
		t.Errorf("body = %v", body)
	}

	off := &catalogue{out: usecase.Output{"state": "OFF", "renews_at": nil}}
	recorder = containerRequest(t, off, http.MethodGet, "/ai-budget-state", "", "")
	if !strings.Contains(recorder.Body.String(), `"renews_at":null`) {
		t.Errorf("OFF answered %s", recorder.Body)
	}
}
