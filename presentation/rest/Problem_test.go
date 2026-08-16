// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The status mapping from api-guidelines.md §6, read back category by category.
func TestEveryCategoryMapsToItsDocumentedStatus(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{shared.ErrValidation, http.StatusUnprocessableEntity, "validation_failed"},
		{shared.ErrMalformedRequest, http.StatusBadRequest, "malformed_request"},
		{shared.ErrUnauthenticated, http.StatusUnauthorized, "unauthenticated"},
		{shared.ErrForbidden, http.StatusForbidden, "forbidden"},
		{shared.ErrNotFound, http.StatusNotFound, "not_found"},
		{shared.ErrConflict, http.StatusConflict, "conflict"},
		{shared.ErrVersionConflict, http.StatusConflict, "version_conflict"},
		{shared.ErrGone, http.StatusGone, "gone"},
		{shared.ErrRateLimited, http.StatusTooManyRequests, "rate_limited"},
		{shared.ErrUnavailable, http.StatusServiceUnavailable, "dependency_unavailable"},
		{shared.ErrInternal, http.StatusInternalServerError, "internal"},
	}

	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			got := ProblemFrom(tc.err, "01J9")

			if got.Status != tc.status {
				t.Errorf("status = %d, want %d", got.Status, tc.status)
			}
			if got.Code != tc.code {
				t.Errorf("code = %q, want %q", got.Code, tc.code)
			}
			if got.Title != tc.code {
				t.Errorf("title = %q, want the code %q", got.Title, tc.code)
			}
			if got.Docs != "https://docs.hubtask.dev/errors/"+tc.code {
				t.Errorf("docs = %q", got.Docs)
			}
			if got.RequestID != "01J9" {
				t.Errorf("request_id = %q, want 01J9", got.RequestID)
			}
		})
	}
}

func TestDetailCodeAndParamsReachTheClient(t *testing.T) {
	err := shared.ErrValidation.
		WithDetail("items.cover_not_supported_for_type").
		WithParams(map[string]string{"item_type": "ACTIVITY", "capability": "COVER"})

	got := ProblemFrom(err, "01J9")

	if got.DetailCode != "items.cover_not_supported_for_type" {
		t.Errorf("detail_code = %q", got.DetailCode)
	}
	if got.Params["item_type"] != "ACTIVITY" || got.Params["capability"] != "COVER" {
		t.Errorf("params = %v", got.Params)
	}
}

func TestFieldErrorsArePassedThrough(t *testing.T) {
	err := shared.ErrValidation.WithFields(
		shared.FieldError{Path: "/title", Code: "too_long", Params: map[string]string{"max": "200"}},
		shared.FieldError{Path: "/cover", Code: "not_allowed"},
	)

	got := ProblemFrom(err, "01J9")

	if len(got.FieldErrors) != 2 {
		t.Fatalf("field_errors = %+v, want 2 entries", got.FieldErrors)
	}
	if got.FieldErrors[0].Path != "/title" || got.FieldErrors[0].Params["max"] != "200" {
		t.Errorf("first field error = %+v", got.FieldErrors[0])
	}
	if got.FieldErrors[1].Code != "not_allowed" {
		t.Errorf("second field error = %+v", got.FieldErrors[1])
	}
}

// T-18 and security.md §9: a response carries no stack trace, no query fragment, no path, and no
// connection string. An unknown error is where that leaks if anywhere does.
func TestAnUnknownErrorLeaksNothingIntoTheResponse(t *testing.T) {
	leaky := errors.New(
		`pq: password authentication failed for user "hubtask" on postgres://hubtask:hunter2@db:5432/hubtask`)

	problem := ProblemFrom(fmt.Errorf("loading the container: %w", leaky), "01J9")

	body, err := json.Marshal(problem)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	for _, forbidden := range []string{"hunter2", "postgres://", "password", "hubtask:", "5432", "pq:"} {
		if strings.Contains(string(body), forbidden) {
			t.Errorf("the response contains %q: %s", forbidden, body)
		}
	}
	if problem.Status != http.StatusInternalServerError || problem.Code != "internal" {
		t.Errorf("got %d/%s, want 500/internal", problem.Status, problem.Code)
	}
}

// An internal error may have been raised deep inside an adapter. Whatever it attached as
// parameters was written for a log, so it does not go out.
func TestAnInternalErrorDropsParametersAndDetailCode(t *testing.T) {
	err := shared.ErrInternal.
		WithDetail("postgres.query_failed").
		WithParams(map[string]string{"query": "SELECT * FROM work_item WHERE tenant_id = $1"}).
		WithFields(shared.FieldError{Path: "/title", Code: "too_long"})

	got := ProblemFrom(err, "01J9")

	if got.Params != nil {
		t.Errorf("params = %v, want none on a 500", got.Params)
	}
	if got.DetailCode != "" {
		t.Errorf("detail_code = %q, want empty on a 500", got.DetailCode)
	}
	if got.FieldErrors != nil {
		t.Errorf("field_errors = %+v, want none on a 500", got.FieldErrors)
	}
}

func TestNilErrorBecomesInternal(t *testing.T) {
	got := ProblemFrom(nil, "01J9")

	if got.Status != http.StatusInternalServerError || got.Code != "internal" {
		t.Errorf("got %d/%s, want 500/internal", got.Status, got.Code)
	}
}

func TestWriteProblemSendsTheDocumentedMediaType(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteProblem(rec, shared.ErrNotFound.WithDetail("containers.not_found"), "01J9")

	if got := rec.Header().Get("Content-Type"); got != ProblemContentType {
		t.Errorf("Content-Type = %q, want %q", got, ProblemContentType)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}

	var problem Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the body is not valid JSON: %v", err)
	}
	if problem.Code != "not_found" || problem.DetailCode != "containers.not_found" {
		t.Errorf("body = %+v", problem)
	}
}

// The field names are the contract (api/openapi.yaml, schema Problem). A rename here is a
// breaking change, so the test names them explicitly rather than round-tripping the struct.
func TestTheJSONFieldNamesMatchTheSpecification(t *testing.T) {
	problem := ProblemFrom(
		shared.ErrValidation.
			WithDetail("items.title_too_long").
			WithParams(map[string]string{"max": "200"}).
			WithFields(shared.FieldError{Path: "/title", Code: "too_long"}),
		"01J9")

	body, err := json.Marshal(problem)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	for _, field := range []string{
		"type", "title", "status", "code", "detail_code", "params", "field_errors",
		"request_id", "docs",
	} {
		if _, ok := raw[field]; !ok {
			t.Errorf("the field %q is missing from the response: %s", field, body)
		}
	}
	// No free text for a client to parse (api-guidelines.md §6).
	if _, ok := raw["detail"]; ok {
		t.Errorf("the response carries a free-text detail field: %s", body)
	}
}

// Optional fields disappear when empty, so a minimal error stays a minimal response.
func TestEmptyOptionalFieldsAreOmitted(t *testing.T) {
	body, err := json.Marshal(ProblemFrom(shared.ErrNotFound, ""))
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	for _, field := range []string{"detail_code", "params", "field_errors", "request_id"} {
		if strings.Contains(string(body), `"`+field+`"`) {
			t.Errorf("the empty field %q is present: %s", field, body)
		}
	}
}
