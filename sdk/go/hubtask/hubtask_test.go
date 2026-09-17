// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package hubtask

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckReadsTheProblemDocument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer hbt_pat_x" {
			t.Errorf("bearer not sent: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Idempotency-Key") == "" {
			t.Error("idempotency key not sent")
		}
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"status":422,"code":"validation_failed","detail_code":"items.title_too_long",` +
			`"field_errors":[{"path":"title","code":"items.title_too_long"}]}`))
	}))
	defer server.Close()

	client, err := NewClientWithResponses(server.URL, WithRequestEditorFn(WithBearer("hbt_pat_x")))
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.CreateWorkItemWithResponse(context.Background(), &CreateWorkItemParams{},
		CreateWorkItemJSONRequestBody{Title: "x", Type: "TASK"}, WithIdempotencyKey("018f6e2a-2b7c-7c1e-9b1a-3d5e6f7a8b9c"))
	checked := Check(err, res)
	var problem *ProblemError
	if !errors.As(checked, &problem) {
		t.Fatalf("Check answered %v, want a ProblemError", checked)
	}
	if problem.Status != 422 || problem.Problem.Code != "validation_failed" {
		t.Errorf("problem = %+v", problem)
	}
	if problem.Problem.FieldErrors == nil || len(*problem.Problem.FieldErrors) != 1 {
		t.Errorf("field errors not decoded: %+v", problem.Problem)
	}
	if problem.Error() != "hubtask: 422 validation_failed (items.title_too_long)" {
		t.Errorf("Error() = %q", problem.Error())
	}
}

func TestCheckPassesASuccessAndWrapsABodyThatIsNotAProblem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/meta/health" {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html>upstream</html>"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"next_cursor":null,"has_more":false}`))
	}))
	defer server.Close()

	client, err := NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.ListContainersWithResponse(context.Background(), &ListContainersParams{})
	if err := Check(err, res); err != nil {
		t.Fatalf("a 200 is not an error: %v", err)
	}
	health, err := client.GetHealthReportWithResponse(context.Background())
	checked := Check(err, health)
	var problem *ProblemError
	if !errors.As(checked, &problem) || problem.Status != http.StatusBadGateway || problem.Problem.Code != "" {
		t.Fatalf("a 502 with HTML should be a ProblemError with the status and no code: %v", checked)
	}
	if err := Check(errors.New("dial"), nil); err == nil || err.Error() != "dial" {
		t.Errorf("a transport error is passed through: %v", err)
	}
	var nilResult *ListContainersResult
	if err := Check(nil, nilResult); err == nil {
		t.Error("a nil answer is an error")
	}
}
