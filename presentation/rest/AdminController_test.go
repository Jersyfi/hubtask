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

// The control plane over REST: the listing answers every field the use case fills, and the
// lifecycle edges reach their use case with the workspace named in the path.

func callAdmin(t *testing.T, registry UseCaseRegistry, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry
	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		Scopes:    []string{"admin:tenants"},
	})
	request := httptest.NewRequestWithContext(ctx, method, APIBasePath+path, strings.NewReader(""))
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// A workspace under a legal hold says so in the listing: the console leaves *Delete* out for it on
// that field alone (P-05), so a listing that dropped it would offer a button the server refuses.
func TestTheInstallationListingAnswersTheLegalHold(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"data": []usecase.Output{{
		"id": "0192f000-0000-7000-8000-0000000000e1", "slug": "acme", "display_name": "Acme",
		"status": "ACTIVE", "created_at": time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC),
		"purge_after": nil, "legal_hold": true,
	}}}}

	recorder := callAdmin(t, registry, http.MethodGet, "/admin/tenants")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("the answer %s (%v)", recorder.Body, err)
	}
	if rows[0]["legal_hold"] != true {
		t.Errorf("legal_hold = %v, want true", rows[0]["legal_hold"])
	}
}

// Once the purge has begun the listing says when, which is what the console reads to stop offering
// *Cancel deletion*; before that the field is absent.
func TestTheInstallationListingAnswersWhenThePurgeBegan(t *testing.T) {
	began := time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)
	registry := &catalogue{out: usecase.Output{"data": []usecase.Output{
		{
			"id": "0192f000-0000-7000-8000-0000000000e1", "slug": "leaving", "display_name": "Leaving",
			"status": "PENDING_DELETION", "created_at": began, "purge_after": began,
			"legal_hold": false, "purge_started_at": began,
		},
		{
			"id": "0192f000-0000-7000-8000-0000000000e2", "slug": "waiting", "display_name": "Waiting",
			"status": "PENDING_DELETION", "created_at": began, "purge_after": began, "legal_hold": false,
		},
	}}}

	recorder := callAdmin(t, registry, http.MethodGet, "/admin/tenants")
	var rows []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &rows); err != nil || len(rows) != 2 {
		t.Fatalf("the answer %s (%v)", recorder.Body, err)
	}
	if rows[0]["purge_started_at"] != "2026-10-12T03:00:00Z" {
		t.Errorf("purge_started_at = %v", rows[0]["purge_started_at"])
	}
	if started, present := rows[1]["purge_started_at"]; present && started != nil {
		t.Errorf("a purge that has not begun answers %v", started)
	}
}

func TestCancellingADeletionReachesItsUseCase(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"tenant_id": "0192f000-0000-7000-8000-0000000000e1", "status": "ACTIVE",
	}}

	recorder := callAdmin(t, registry, http.MethodPost,
		"/admin/tenants/0192f000-0000-7000-8000-0000000000e1:cancel-deletion")
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "CancelTenantDeletion" ||
		registry.in["tenant_id"] != "0192f000-0000-7000-8000-0000000000e1" {
		t.Errorf("the call reached %q with %v", registry.name, registry.in)
	}
}
