// SPDX-License-Identifier: Apache-2.0
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

func callPlans(
	t *testing.T, registry UseCaseRegistry, method, path, body string, headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	controller := NewRestController()
	controller.UseCases = registry
	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		Scopes:    []string{"admin:tenants"},
	})
	request := httptest.NewRequestWithContext(ctx, method, APIBasePath+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func answeredPlan() usecase.Output {
	at := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	return usecase.Output{
		"name": "family", "display_name": "Family", "version": 3, "workspaces": int64(2),
		"created_at": at, "updated_at": at, "left_out": []any{"WEBHOOKS"},
		"quotas": usecase.Output{"items": usecase.Output{
			"set": true, "value": int64(5000), "locked": true, "kind": "COUNT", "position": 0,
		}},
	}
}

// The values travel in the installation level's own shape, both ways, and the version is the tag.
func TestAPlanIsCreatedOverREST(t *testing.T) {
	registry := &catalogue{out: answeredPlan()}
	recorder := callPlans(t, registry, http.MethodPost, "/admin/plans",
		`{"name": "family", "display_name": "Family",
		  "quotas": {"items": {"value": 5000, "locked": true, "set": true}},
		  "left_out": ["WEBHOOKS"]}`, nil)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if recorder.Header().Get("ETag") != `"3"` {
		t.Errorf("ETag = %q", recorder.Header().Get("ETag"))
	}
	if registry.name != "CreatePlan" || registry.in["name"] != "family" {
		t.Errorf("reached %q with %v", registry.name, registry.in)
	}
	quotas, _ := registry.in["quotas"].(map[string]any)
	if items, _ := quotas["items"].(map[string]any); items["locked"] != true {
		t.Errorf("the limit reached the use case as %v", quotas)
	}
	var answer map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the answer %s (%v)", recorder.Body, err)
	}
	if answer["workspaces"] != float64(2) || answer["left_out"].([]any)[0] != "WEBHOOKS" {
		t.Errorf("answered %v", answer)
	}
}

// If-Match carries the version read to the use case; a stale one is the use case's to refuse, and
// its refusal is a 409 version_conflict.
func TestAPlanChangeCarriesTheVersionRead(t *testing.T) {
	registry := &catalogue{out: answeredPlan()}
	recorder := callPlans(t, registry, http.MethodPut, "/admin/plans/family",
		`{"display_name": "Family plus"}`, map[string]string{"If-Match": `"2"`})
	if recorder.Code != http.StatusOK || registry.in["expected_version"] != 2 || registry.in["name"] != "family" {
		t.Fatalf("status %d, reached %q with %v", recorder.Code, registry.name, registry.in)
	}

	registry = &catalogue{err: shared.ErrVersionConflict.WithDetail("admin.plan_version_moved")}
	recorder = callPlans(t, registry, http.MethodPut, "/admin/plans/family",
		`{"display_name": "Family plus"}`, map[string]string{"If-Match": `"1"`})
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "version_conflict") {
		t.Errorf("a stale version answered %d %s", recorder.Code, recorder.Body)
	}
}

func TestAPlanIsRemovedOverREST(t *testing.T) {
	registry := &catalogue{out: usecase.Output{}}
	recorder := callPlans(t, registry, http.MethodDelete, "/admin/plans/family", "", nil)
	if recorder.Code != http.StatusNoContent || registry.name != "RemovePlan" || registry.in["name"] != "family" {
		t.Errorf("status %d, reached %q with %v", recorder.Code, registry.name, registry.in)
	}
}
