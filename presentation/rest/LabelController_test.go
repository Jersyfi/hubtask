// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const firstLabel = "0192f000-0000-7000-8000-0000000000c1"

func storedLabel(name string) usecase.Output {
	return usecase.Output{
		"id":            firstLabel,
		"collection_id": boardCollection,
		"name":          name,
		"color_token":   "accent.red",
		"description":   nil,
		"version":       1,
	}
}

func labelRequest(
	t *testing.T, registry UseCaseRegistry, method, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, method, APIBasePath+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestCreatingALabelAnswers201WithTheLabel(t *testing.T) {
	registry := &catalogue{out: storedLabel("Urgent")}

	recorder := labelRequest(t, registry, http.MethodPost,
		"/containers/"+boardCollection+"/labels", `{"name":"Urgent","color_token":"accent.red"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status %d, want 201: %s", recorder.Code, recorder.Body)
	}
	if registry.name != createLabelUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	// The collection comes from the path rather than from the body: the route already says which
	// vocabulary this is.
	if registry.in["collection_id"] != boardCollection {
		t.Errorf("collection_id is %v", registry.in["collection_id"])
	}

	want := APIBasePath + "/containers/" + boardCollection + "/labels/" + firstLabel
	if location := recorder.Header().Get("Location"); location != want {
		t.Errorf("Location is %q, want %q", location, want)
	}
	if tag := recorder.Header().Get("ETag"); tag != `"1"` {
		t.Errorf("ETag is %q", tag)
	}
}

func TestCreatingALabelRefusesAnUnknownField(t *testing.T) {
	registry := &catalogue{out: storedLabel("Urgent")}

	recorder := labelRequest(t, registry, http.MethodPost,
		"/containers/"+boardCollection+"/labels", `{"name":"Urgent","colour_token":"red"}`)

	if recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want a refusal: %s", recorder.Code, recorder.Body)
	}
	if registry.invoked {
		t.Error("the use case ran on a body the contract does not describe")
	}
}

// A plain array rather than a page, as the contract declares.
func TestListingAVocabularyAnswersAnArray(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"data": []usecase.Output{storedLabel("Urgent")}}}

	recorder := labelRequest(t, registry, http.MethodGet,
		"/containers/"+boardCollection+"/labels", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != listLabelsUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}

	var vocabulary []map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &vocabulary); err != nil {
		t.Fatalf("the body is not an array: %v (%s)", err, recorder.Body)
	}
	if len(vocabulary) != 1 {
		t.Fatalf("%d labels, want 1", len(vocabulary))
	}
	// The description is present as null rather than absent: a client renders the label from this.
	raw, present := vocabulary[0]["description"]
	if !present || string(raw) != "null" {
		t.Errorf("description is %s, want null", raw)
	}
}

func TestAnEmptyVocabularyIsAnEmptyArray(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"data": []usecase.Output{}}}

	recorder := labelRequest(t, registry, http.MethodGet,
		"/containers/"+boardCollection+"/labels", "")

	if body := strings.TrimSpace(recorder.Body.String()); body != "[]" {
		t.Errorf("the body is %q, want an empty array", body)
	}
}

// The handler holds no rules, so a refusal from the catalogue reaches the client unchanged.
func TestALabelRefusalIsReportedAsAProblem(t *testing.T) {
	registry := &catalogue{err: shared.ErrConflict.WithDetail("labels.name_taken")}

	recorder := labelRequest(t, registry, http.MethodPost,
		"/containers/"+boardCollection+"/labels", `{"name":"Urgent","color_token":"accent.red"}`)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409: %s", recorder.Code, recorder.Body)
	}
}
