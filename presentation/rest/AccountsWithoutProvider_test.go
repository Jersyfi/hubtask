// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// The password switch's count (ADR-0078 §1): the route reaches its use case with nothing to send, and
// answers the number under the name the contract declares.
func TestTheCountOfAccountsWithoutAProviderIsAnswered(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"count": 3}}
	controller := NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		APIBasePath+"/tenant/accounts-without-provider", nil)
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != countAccountsWithoutProviderUseCase {
		t.Errorf("use case = %q", registry.name)
	}
	var answer struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil || answer.Count != 3 {
		t.Errorf("the answer is %s (%v), want the count", recorder.Body, err)
	}
}
