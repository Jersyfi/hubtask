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

// The job resource over REST (E-01). What this layer owes is the explicit nulls and, above all,
// what it does *not* write: the response schema is narrow, and a field the queue keeps must not
// reach a client because a use case happened to answer it.

const jobUUID = "0192f000-0000-7000-8000-0000000000a1"

func runningJobOutput() usecase.Output {
	return usecase.Output{
		"job_id":     jobUUID,
		"status":     "RUNNING",
		"created_at": time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC),
	}
}

func callJob(
	t *testing.T, registry UseCaseRegistry, method, path string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, method, APIBasePath+path, strings.NewReader(""))

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestAJobIsServedWithItsUnknownsAsExplicitNulls(t *testing.T) {
	registry := &catalogue{out: runningJobOutput()}

	recorder := callJob(t, registry, http.MethodGet, "/jobs/"+jobUUID)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "GetJob" {
		t.Errorf("use case = %q, want GetJob", registry.name)
	}
	if registry.in["job_id"] != jobUUID {
		t.Errorf("the path reached the catalogue as %v", registry.in)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	if body["status"] != "RUNNING" || body["job_id"] != jobUUID {
		t.Errorf("the job reads as %v", body)
	}

	// Present as null rather than absent. A client reads progress unconditionally, and "this job
	// cannot say" has to be tellable from "nothing done yet".
	for _, field := range []string{"progress", "result_url", "error_code", "finished_at"} {
		value, present := body[field]
		if !present {
			t.Errorf("the job omits %s rather than sending null", field)
		}
		if value != nil {
			t.Errorf("%s is %v, want null", field, value)
		}
	}
}

// The narrow reading of the row, enforced at the boundary: a use case that grew a field the
// contract does not declare must not be able to leak it through this mapper.
func TestNothingOfTheQueuesBookkeepingReachesTheResponse(t *testing.T) {
	out := runningJobOutput()
	out["payload"] = map[string]any{"target_id": "0192f000-0000-7000-8000-0000000000c1"}
	out["attempts"] = 3
	out["dedupe_key"] = "backup:0192f000"
	out["locked_until"] = time.Date(2026, 8, 26, 8, 5, 0, 0, time.UTC)

	recorder := callJob(t, &catalogue{out: out}, http.MethodGet, "/jobs/"+jobUUID)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	for _, field := range []string{"payload", "attempts", "dedupe_key", "locked_until"} {
		if _, present := body[field]; present {
			t.Errorf("%s reached the client", field)
		}
	}
}

func TestAFinishedJobCarriesItsCodeAndItsTimes(t *testing.T) {
	finished := time.Date(2026, 8, 26, 8, 30, 0, 0, time.UTC)
	registry := &catalogue{out: usecase.Output{
		"job_id":      jobUUID,
		"status":      "FAILED",
		"created_at":  time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC),
		"finished_at": finished,
		"error_code":  "backup.target_unreachable",
		"progress":    0.5,
	}}

	recorder := callJob(t, registry, http.MethodGet, "/jobs/"+jobUUID)

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	switch {
	case body["status"] != "FAILED":
		t.Errorf("status %v", body["status"])
	case body["error_code"] != "backup.target_unreachable":
		t.Errorf("error code %v", body["error_code"])
	case body["progress"] != 0.5:
		t.Errorf("progress %v", body["progress"])
	case body["finished_at"] != finished.Format(time.RFC3339):
		t.Errorf("finished at %v", body["finished_at"])
	}
}

func TestCancellingAJobAnswersTheJobItNowIs(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"job_id":      jobUUID,
		"status":      "CANCELLED",
		"created_at":  time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC),
		"finished_at": time.Date(2026, 8, 26, 8, 40, 0, 0, time.UTC),
	}}

	recorder := callJob(t, registry, http.MethodPost, "/jobs/"+jobUUID+":cancel")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "CancelJob" {
		t.Errorf("use case = %q, want CancelJob", registry.name)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	if body["status"] != "CANCELLED" {
		t.Errorf("the answer says %v", body["status"])
	}
}

// A conflict from the catalogue reaches the client as one, with the code that says which kind.
func TestCancellingAFinishedJobIsAConflict(t *testing.T) {
	registry := &catalogue{err: shared.ErrConflict.
		WithDetail("jobs.already_finished").
		WithParams(map[string]string{"job_id": jobUUID, "status": "SUCCEEDED"})}

	recorder := callJob(t, registry, http.MethodPost, "/jobs/"+jobUUID+":cancel")

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var problem map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the answer is not a problem document: %v", err)
	}
	if problem["detail_code"] != "jobs.already_finished" {
		t.Errorf("the problem says %v", problem["code"])
	}
}

// A job in another tenant and a job that never existed answer the same 404, and this is the edge
// that must not add anything distinguishing to either.
func TestAnUnknownJobIsNotFound(t *testing.T) {
	registry := &catalogue{err: shared.ErrNotFound.
		WithDetail("jobs.not_found").
		WithParams(map[string]string{"job_id": jobUUID})}

	recorder := callJob(t, registry, http.MethodGet, "/jobs/"+jobUUID)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
}
