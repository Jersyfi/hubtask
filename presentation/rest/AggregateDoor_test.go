// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// An aggregate's scope reaches the use case as the flat inputs MCP also sends, and its filter and
// aggregate as the documents they are.
func TestAnAggregatesScopeReachesTheUseCaseFlat(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"value": 3}}
	controller := NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/items:aggregate",
		strings.NewReader(`{"scope": {"container_id": "0192f000-0000-7000-8000-0000000000a1", "include_descendants": false},
			"filter": {"field": "status", "op": "EQ", "value": "OPEN"}, "aggregate": {"function": "COUNT"}}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "AggregateItems" || registry.in["scope_container_id"] != "0192f000-0000-7000-8000-0000000000a1" ||
		registry.in["include_descendants"] != false {
		t.Fatalf("%s got %v", registry.name, registry.in)
	}
	if _, ok := registry.in["filter"].(map[string]any); !ok {
		t.Errorf("the filter reached the use case as %T", registry.in["filter"])
	}
}
