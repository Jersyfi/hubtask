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

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The routes C-07 adds, at the mapping layer - the catalogue's projection into the contract's
// schema - because that is the layer where a field quietly goes missing.

var (
	definitionID  = shared.MustParseID("0192f000-0000-7000-8000-0000000000c5")
	fieldItemID   = shared.MustParseID("0192f000-0000-7000-8000-0000000000c6")
	fieldTenantID = shared.MustParseID("0192f000-0000-7000-8000-0000000000c7")
	definedAt     = time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
)

func definitionProjection() usecase.Output {
	return usecase.Output{
		"id":            definitionID.String(),
		"collection_id": nil,
		"key":           "priority",
		"kind":          "SELECT",
		"options":       []string{"high", "low"},
		"is_required":   false,
		"applies_to":    []string{"TASK"},
		"created_at":    definedAt,
		"updated_at":    definedAt,
		"version":       1,
	}
}

func fieldController(cat *catalogue) *RestController {
	controller := NewRestController()
	controller.UseCases = cat
	return controller
}

func TestDefiningAFieldAnswersWithItsLocationAndVersion(t *testing.T) {
	cat := &catalogue{out: definitionProjection()}
	controller := fieldController(cat)

	body := `{"key":"priority","kind":"SELECT","options":["high","low"]}`
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/custom-fields", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != defineCustomFieldUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}
	if got := response.Header().Get("Location"); got != APIBasePath+"/custom-fields/"+definitionID.String() {
		t.Errorf("Location is %q", got)
	}
	if got := response.Header().Get("ETag"); got != `"1"` {
		t.Errorf("ETag is %q", got)
	}

	var answered struct {
		Key          string   `json:"key"`
		Kind         string   `json:"kind"`
		Options      []string `json:"options"`
		CollectionID *string  `json:"collection_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answered); err != nil {
		t.Fatalf("the answer is not the schema's shape: %v", err)
	}
	if answered.Key != "priority" || answered.Kind != "SELECT" || len(answered.Options) != 2 {
		t.Errorf("the answer is %+v", answered)
	}
	// Present and null for a workspace-wide field: absent would say this server does not know
	// about scopes, which is a different statement from "this one is everywhere".
	if !strings.Contains(response.Body.String(), `"collection_id":null`) {
		t.Errorf("the scope is missing or absent: %s", response.Body.String())
	}
}

func TestListingDefinitionsAnswersAnArrayAndThreadsTheScope(t *testing.T) {
	cat := &catalogue{out: usecase.Output{"data": []usecase.Output{definitionProjection()}}}
	controller := fieldController(cat)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		APIBasePath+"/custom-fields?collection_id="+fieldTenantID.String(), nil)
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != listCustomFieldsUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}
	if cat.in["collection_id"] != fieldTenantID.String() {
		t.Errorf("the scope asked about is %v", cat.in["collection_id"])
	}
	// An array rather than a page: a workspace's vocabulary is not something a client walks.
	if !strings.HasPrefix(strings.TrimSpace(response.Body.String()), "[") {
		t.Errorf("the answer is not an array: %s", response.Body.String())
	}
}

func TestUpdatingADefinitionIsAMergePatch(t *testing.T) {
	cat := &catalogue{out: definitionProjection()}
	controller := fieldController(cat)

	// Only the options travel: what the caller did not send must not reach the catalogue, so that
	// "do not touch it" and "set it" stay two different requests.
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPatch,
		APIBasePath+"/custom-fields/"+definitionID.String(),
		strings.NewReader(`{"options":["high","low","medium"]}`))
	request.Header.Set("Content-Type", "application/merge-patch+json")
	request.Header.Set("If-Match", `"1"`)

	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != updateCustomFieldUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}
	if cat.in["expected_version"] != 1 {
		t.Errorf("the expected version is %v", cat.in["expected_version"])
	}
	if _, sent := cat.in["is_required"]; sent {
		t.Error("a field the caller did not send reached the catalogue")
	}
	if _, sent := cat.in["applies_to"]; sent {
		t.Error("a field the caller did not send reached the catalogue")
	}
}

func TestDeletingADefinitionAnswersWithoutABody(t *testing.T) {
	cat := &catalogue{out: usecase.Output{}}
	controller := fieldController(cat)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodDelete,
		APIBasePath+"/custom-fields/"+definitionID.String(), nil)
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != deleteCustomFieldUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}
	if response.Body.Len() != 0 {
		t.Errorf("the answer carries %q", response.Body.String())
	}
}

func TestSettingAValueThreadsTheKeyAndTheRawValue(t *testing.T) {
	projection := coveredProjection()
	projection["id"] = fieldItemID.String()
	projection["custom_fields"] = map[string]any{"priority": "high"}
	cat := &catalogue{out: projection}
	controller := fieldController(cat)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		APIBasePath+"/items/"+fieldItemID.String()+"/custom-fields/priority",
		strings.NewReader(`{"value":"high"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", `"3"`)

	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != setCustomFieldUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}
	if cat.in["key"] != "priority" || cat.in["expected_version"] != 3 {
		t.Errorf("the input is %+v", cat.in)
	}
	// The value travels as it arrived: what shape it may have is the definition's answer, and an
	// adapter that coerced it would be deciding a rule.
	if cat.in["value"] != "high" {
		t.Errorf("the value reached the catalogue as %#v", cat.in["value"])
	}
	// The ETag is the version after the change, from the entry the catalogue answered with.
	if got := response.Header().Get("ETag"); got != `"4"` {
		t.Errorf("ETag is %q", got)
	}
	if !strings.Contains(response.Body.String(), `"custom_fields":{"priority":"high"}`) {
		t.Errorf("the entry does not carry the value: %s", response.Body.String())
	}
}

// Clearing travels as an explicit null value, which the adapter must not turn into "absent".
func TestClearingAValueSendsAnExplicitNull(t *testing.T) {
	cat := &catalogue{out: coveredProjection()}
	controller := fieldController(cat)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		APIBasePath+"/items/"+fieldItemID.String()+"/custom-fields/priority",
		strings.NewReader(`{"value":null}`))
	request.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	value, present := cat.in["value"]
	if !present && value != nil {
		t.Errorf("the clearing reached the catalogue as %#v (present=%v)", value, present)
	}
	if value != nil {
		t.Errorf("the value is %#v, want nil", value)
	}
}
