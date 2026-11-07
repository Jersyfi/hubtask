// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A path parameter in a form the contract does not allow is refused as every other request is:
// RFC 9457 with a code and the parameter named, never the generated validator's English text
// (rule 8), so a client can tell "no such goal" from "this build cannot read the answer".
func TestAMalformedPathParameterIsAProblemNamingIt(t *testing.T) {
	controller := NewRestController()
	controller.UseCases = &catalogue{}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/goals/dashboards", nil)
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if got := recorder.Header().Get("Content-Type"); got != ProblemContentType {
		t.Fatalf("content type = %q, body = %s", got, recorder.Body)
	}
	var problem Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("body %s: %v", recorder.Body, err)
	}
	if problem.Code != "malformed_request" || problem.DetailCode != "request.parameter_invalid" || problem.Params["parameter"] != "goalId" {
		t.Errorf("problem = %+v", problem)
	}
	if len(problem.FieldErrors) != 1 || problem.FieldErrors[0].Path != "goalId" {
		t.Errorf("field errors = %+v", problem.FieldErrors)
	}
}
