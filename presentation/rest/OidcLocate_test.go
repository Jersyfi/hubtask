// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func locateRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/auth/oidc:locate", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

// The state reaches the lookup under the descriptor's name, and the answer is the address alone.
func TestTheLocatedFlowAnswersItsAddress(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"base_url": "https://anna.hub.example"}}
	controller := NewRestController()
	controller.UseCases = registry
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, locateRequest(t, `{"state":"the-state"}`))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != locateOidcFlowUseCase || registry.in["state"] != "the-state" {
		t.Errorf("invoked %q with %v", registry.name, registry.in)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(body) != 1 || body["base_url"] != "https://anna.hub.example" {
		t.Errorf("body %v, want the address alone", body)
	}
}

// A state that names no live flow is a 404 with the one detail code.
func TestAFlowNotFoundIsA404(t *testing.T) {
	controller := NewRestController()
	controller.UseCases = &catalogue{err: shared.ErrNotFound.WithDetail("auth.oidc_flow_not_found")}
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, locateRequest(t, `{"state":"gone"}`))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "auth.oidc_flow_not_found") {
		t.Errorf("body %s, want the one detail code", recorder.Body)
	}
}

// The lookup is in the auth bucket with the other anonymous credential routes: keyed per address.
func TestTheLookupSpendsTheAuthBudget(t *testing.T) {
	bucket := AuthBucket(10, 5)(locateRequest(t, `{"state":"s"}`))
	if bucket.Key == "" || bucket.Limit != 10 || bucket.Burst != 5 {
		t.Errorf("bucket %+v, want the auth budget", bucket)
	}
}
