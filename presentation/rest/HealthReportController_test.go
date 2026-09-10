// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	health "github.com/Jersyfi/hubtask/core/port/health"
)

// healthReader is the use case as this controller sees it. What it returns is already the answer
// the actor may read - the reduction is the application layer's, and a test that let this adapter
// trim would be testing the wrong layer (K-06).
type healthReader struct {
	report health.Report
	err    error
}

func (h healthReader) Execute(
	context.Context, appshared.ActorContext,
) (health.Report, error) {
	return h.report, h.err
}

func serveHealthReport(t *testing.T, reader HealthReportReader) *httptest.ResponseRecorder {
	t.Helper()
	controller := NewRestController()
	controller.HealthReport = reader

	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/meta/health", nil))
	return response
}

// The route the contract has declared since A-06 and `Pending.go` answered 404 to until K-06
// (#507). W-03's acceptance in milestone-0.3.5.md says it answers in a running container.
func TestTheHealthReportIsServedUnderTheApiPath(t *testing.T) {
	response := serveHealthReport(t, healthReader{report: health.Report{
		Status:  health.StatusOK,
		Version: "0.7.5",
		Dependencies: []health.DependencyReport{
			{Name: "postgres", Required: true, Result: health.Result{Status: health.StatusOK}},
		},
	}})

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}

	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v", body["status"])
	}
}

// The API route answers 200 even when the report says the installation is down, which the internal
// listener deliberately does not. The contract's own description is the reason: the HTTP status
// describes whether the endpoint is reachable, the `status` field describes the system.
func TestADownReportIsStillATwoHundredHere(t *testing.T) {
	response := serveHealthReport(t, healthReader{report: health.Report{
		Status:  health.StatusDown,
		Version: "0.7.5",
		Dependencies: []health.DependencyReport{
			{Name: "postgres", Required: true, Result: health.Result{Status: health.StatusDown}},
		},
	}})

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 with a body that says down", response.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if body["status"] != "down" {
		t.Errorf("status = %v, want down", body["status"])
	}
}

// The reduced answer is what a session receives, and `dependencies` must be absent rather than
// empty: a reader given `[]` would conclude that nothing is monitored, which is a different
// statement from "you may not see this".
func TestTheReducedAnswerOmitsWhatItDoesNotCarry(t *testing.T) {
	response := serveHealthReport(t, healthReader{report: health.Report{
		Status:  health.StatusDegraded,
		Version: "0.7.5",
		DegradedFeatures: []health.DegradedFeature{{
			Feature:    "media",
			ReasonCode: "dependency.unavailable",
			Since:      time.Unix(1_755_000_000, 0).UTC(),
		}},
	}})

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	for _, absent := range []string{"dependencies", "backlogs", "warnings", "migration", "role"} {
		if _, present := body[absent]; present {
			t.Errorf("%q is in an answer that is not the operator's", absent)
		}
	}
	features, ok := body["degraded_features"].([]any)
	if !ok || len(features) != 1 {
		t.Fatalf("degraded_features = %v, and it is the whole reason this answer exists", body["degraded_features"])
	}
}

// A refusal is the use case's answer and reaches the client as a problem document, not as an empty
// report: a banner that received `{}` would draw "everything is fine" from "you may not ask".
func TestARefusalIsAProblemAndNotAnEmptyReport(t *testing.T) {
	response := serveHealthReport(t, healthReader{err: shared.ErrForbidden.WithDetail("access.denied")})

	if response.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", response.Code)
	}
	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.DetailCode != "access.denied" {
		t.Errorf("detail code %q", problem.DetailCode)
	}
}
