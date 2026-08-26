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

// Data subject rights over REST (E-10). What this layer owes is the fields reaching the catalogue
// under the names the descriptor declares, and the case coming back in the contract's shape.

func caseOutput() usecase.Output {
	return usecase.Output{
		"id":                 "0192f000-0000-7000-8000-00000000000b",
		"kind":               "ERASURE",
		"status":             "IN_PROGRESS",
		"scope":              "TENANT",
		"subject_account_id": "0192f000-0000-7000-8000-00000000000d",
		"subject_email":      "anna@example.org",
		"erasure_mode":       "ANONYMIZE",
		"received_at":        time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC),
		"due_at":             time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC),
		"handled_by":         "0192f000-0000-7000-8000-00000000000e",
		"notes":              "By email",
	}
}

func privacyRequest(t *testing.T, registry UseCaseRegistry, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, method, APIBasePath+path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestACaseIsRecordedAndComesBackInTheContractsShape(t *testing.T) {
	registry := &catalogue{out: caseOutput()}

	recorder := privacyRequest(t, registry, http.MethodPost, "/privacy/requests",
		`{"kind":"ERASURE","scope":"TENANT","subject_email":"anna@example.org",`+
			`"subject_account_id":"0192f000-0000-7000-8000-00000000000d",`+
			`"due_at":"2026-09-25T09:00:00Z","target_id":"0192f000-0000-7000-8000-0000000000f1",`+
			`"notes":"By email"}`)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("recording answered %d: %s", recorder.Code, recorder.Body)
	}

	var body struct {
		ID          string `json:"id"`
		Kind        string `json:"kind"`
		Status      string `json:"status"`
		ErasureMode string `json:"erasure_mode"`
		DueAt       string `json:"due_at"`
		Email       string `json:"subject_email"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not the contract's shape: %v", err)
	}
	if body.Kind != "ERASURE" || body.Status != "IN_PROGRESS" || body.ErasureMode != "ANONYMIZE" {
		t.Errorf("the case came back as %+v", body)
	}
	if body.Email != "anna@example.org" || body.DueAt != "2026-09-25T09:00:00Z" {
		t.Errorf("the case came back as %+v", body)
	}

	if registry.name != createDataSubjectRequestUseCase {
		t.Errorf("the request ran %q", registry.name)
	}
	for field, want := range map[string]any{
		"kind":               "ERASURE",
		"scope":              "TENANT",
		"subject_email":      "anna@example.org",
		"subject_account_id": "0192f000-0000-7000-8000-00000000000d",
		"target_id":          "0192f000-0000-7000-8000-0000000000f1",
		"notes":              "By email",
	} {
		if registry.in[field] != want {
			t.Errorf("%s reached the catalogue as %v, want %v", field, registry.in[field], want)
		}
	}
	if registry.in["due_at"] != "2026-09-25T09:00:00Z" {
		t.Errorf("the deadline reached the catalogue as %v", registry.in["due_at"])
	}
}

func TestEveryStepOfACaseReachesTheCatalogue(t *testing.T) {
	registry := &catalogue{out: caseOutput()}

	recorder := privacyRequest(t, registry, http.MethodPatch,
		"/privacy/requests/0192f000-0000-7000-8000-00000000000b",
		`{"status":"IN_PROGRESS","erasure_mode":"FULL_DELETE",`+
			`"handled_by":"0192f000-0000-7000-8000-00000000000e",`+
			`"rejection_reason":"","notes":"Started","target_id":"0192f000-0000-7000-8000-0000000000f1"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("the step answered %d: %s", recorder.Code, recorder.Body)
	}

	for field, want := range map[string]any{
		"request_id":       "0192f000-0000-7000-8000-00000000000b",
		"status":           "IN_PROGRESS",
		"erasure_mode":     "FULL_DELETE",
		"handled_by":       "0192f000-0000-7000-8000-00000000000e",
		"rejection_reason": "",
		"notes":            "Started",
		"target_id":        "0192f000-0000-7000-8000-0000000000f1",
	} {
		if registry.in[field] != want {
			t.Errorf("%s reached the catalogue as %v, want %v", field, registry.in[field], want)
		}
	}
	if registry.name != updateDataSubjectRequestUseCase {
		t.Errorf("the request ran %q", registry.name)
	}
}

func TestTheCasesAreServedAsAPage(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"data": []usecase.Output{caseOutput()},
		"page": map[string]any{"next_cursor": "opaque", "has_more": true},
	}}

	recorder := privacyRequest(t, registry, http.MethodGet,
		"/privacy/requests?status=RECEIVED&kind=ERASURE&due_within_days=7&include_closed=true&size=25", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("the list answered %d: %s", recorder.Code, recorder.Body)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, `"data":[`) || !strings.Contains(body, `"next_cursor":"opaque"`) {
		t.Errorf("the page came back as %s", body)
	}
	for field, want := range map[string]any{
		"status": "RECEIVED", "kind": "ERASURE", "due_within_days": 7,
		"include_closed": true, "size": 25,
	} {
		if registry.in[field] != want {
			t.Errorf("%s reached the catalogue as %v, want %v", field, registry.in[field], want)
		}
	}
}

// An empty list is an empty array rather than a null: a client reading `data` unconditionally is
// what the shape promises.
func TestAnEmptyCaseListIsAnEmptyArray(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"data": []usecase.Output{},
		"page": map[string]any{"next_cursor": nil, "has_more": false},
	}}

	recorder := privacyRequest(t, registry, http.MethodGet, "/privacy/requests", "")
	if body := recorder.Body.String(); !strings.Contains(body, `"data":[]`) {
		t.Errorf("an empty list came back as %s", body)
	}
}

// The restriction and the withdrawal over REST.
func TestARestrictionIsAppliedAndAnswersTheAccountState(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"id": "0192f000-0000-7000-8000-00000000000d", "status": "RESTRICTED",
	}}

	recorder := privacyRequest(t, registry, http.MethodPost,
		"/accounts/0192f000-0000-7000-8000-00000000000d:restrict",
		`{"restricted":true,"reason":"Art. 18 request"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("the restriction answered %d: %s", recorder.Code, recorder.Body)
	}
	if !strings.Contains(recorder.Body.String(), `"status":"RESTRICTED"`) {
		t.Errorf("the account came back as %s", recorder.Body)
	}

	for field, want := range map[string]any{
		"account_id": "0192f000-0000-7000-8000-00000000000d",
		"restricted": true,
		"reason":     "Art. 18 request",
	} {
		if registry.in[field] != want {
			t.Errorf("%s reached the catalogue as %v, want %v", field, registry.in[field], want)
		}
	}
	if registry.name != restrictProcessingUseCase {
		t.Errorf("the request ran %q", registry.name)
	}
}

func TestAWithdrawalComesBackAsTheRecord(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"id":         "0192f000-0000-7000-8000-00000000000f",
		"account_id": "0192f000-0000-7000-8000-00000000000d",
		"purpose":    "ai_processing",
		"granted":    false,
		"granted_at": time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC),
		"revoked_at": time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC),
		"source":     "user",
	}}

	recorder := privacyRequest(t, registry, http.MethodPost, "/privacy/consents:withdraw",
		`{"purpose":"ai_processing","reason":"Objection"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("the withdrawal answered %d: %s", recorder.Code, recorder.Body)
	}

	body := recorder.Body.String()
	for _, wanted := range []string{
		`"purpose":"ai_processing"`, `"granted":false`, `"revoked_at":"2026-08-26T10:00:00Z"`,
		`"source":"user"`,
	} {
		if !strings.Contains(body, wanted) {
			t.Errorf("the record %s is missing %s", body, wanted)
		}
	}
	if registry.in["purpose"] != "ai_processing" || registry.name != withdrawConsentUseCase {
		t.Errorf("the request ran %q with %v", registry.name, registry.in)
	}
}
