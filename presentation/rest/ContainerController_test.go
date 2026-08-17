// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
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

// catalogue is the registry as this layer sees it: it records what it was asked to run and
// answers with whatever the test set up.
type catalogue struct {
	name    string
	actor   appshared.ActorContext
	in      usecase.Input
	out     usecase.Output
	err     error
	invoked bool
}

func (c *catalogue) Invoke(_ context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	c.name, c.actor, c.in, c.invoked = name, actor, in, true
	return c.out, c.err
}

func createdContainer() usecase.Output {
	return usecase.Output{
		"id":         "0192f000-0000-7000-8000-00000000000b",
		"type":       "HUB",
		"parent_id":  nil,
		"name":       "Private",
		"order_key":  "a0",
		"created_at": time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC),
		"updated_at": time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC),
		"version":    1,
	}
}

func postContainer(t *testing.T, registry UseCaseRegistry, body string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, APIBasePath+"/containers",
		strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestCreatingAContainerAnswers201WithTheContainer(t *testing.T) {
	registry := &catalogue{out: createdContainer()}

	recorder := postContainer(t, registry, `{"type":"HUB","name":"Private"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status %d, want 201: %s", recorder.Code, recorder.Body)
	}
	if location := recorder.Header().Get("Location"); location != APIBasePath+"/containers/0192f000-0000-7000-8000-00000000000b" {
		t.Errorf("Location is %q", location)
	}
	// Strong rather than weak: If-Match compares strongly, and optimistic locking is built on it.
	if tag := recorder.Header().Get("ETag"); tag != `"1"` {
		t.Errorf("ETag is %q, want the version as a strong entity tag", tag)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the response is not JSON: %v", err)
	}
	if body["name"] != "Private" || body["type"] != "HUB" || body["version"] != float64(1) {
		t.Errorf("unexpected body: %v", body)
	}
	if _, present := body["parent_id"]; present {
		t.Errorf("a hub reports a parent: %v", body["parent_id"])
	}
}

// The handler holds no rules: it hands the request to the catalogue under the use case's own
// name, with the actor of the request.
func TestTheRequestReachesTheCatalogue(t *testing.T) {
	registry := &catalogue{out: createdContainer()}

	postContainer(t, registry, `{"type":"COLLECTION","name":"  Shopping  ","parent_id":"0192f000-0000-7000-8000-00000000000b","color_token":"blue"}`)

	if registry.name != "CreateContainer" {
		t.Errorf("the catalogue was asked for %q", registry.name)
	}
	if registry.actor.AccountID.IsZero() {
		t.Error("the actor of the request did not reach the use case")
	}
	if registry.in["type"] != "COLLECTION" || registry.in["parent_id"] != "0192f000-0000-7000-8000-00000000000b" {
		t.Errorf("unexpected input: %v", registry.in)
	}
	if registry.in["color_token"] != "blue" {
		t.Errorf("an optional field was dropped: %v", registry.in)
	}
	// An absent optional field arrives absent rather than empty, so the catalogue sees the same
	// thing whichever channel the call came through.
	if registry.in["icon"] != nil {
		t.Errorf("an absent field arrived as %v", registry.in["icon"])
	}
}

// A refusal from the application layer is rendered as the problem document it already is - the
// adapter adds a status, not a decision (api-guidelines.md §6).
func TestARefusalIsRenderedAsAProblem(t *testing.T) {
	registry := &catalogue{err: shared.ErrForbidden.
		WithDetail("access.not_permitted").
		WithParams(map[string]string{"permission": "STRUCTURE"})}

	recorder := postContainer(t, registry, `{"type":"HUB","name":"Private"}`)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", recorder.Code)
	}
	var problem map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the response is not JSON: %v", err)
	}
	if problem["code"] != "forbidden" || problem["detail_code"] != "access.not_permitted" {
		t.Errorf("unexpected problem: %v", problem)
	}
	if params, _ := problem["params"].(map[string]any); params["permission"] != "STRUCTURE" {
		t.Errorf("the parameters a client renders from are missing: %v", problem)
	}
}

// A misspelled field is refused rather than ignored - the same answer the catalogue gives an
// agent that misspells it in a tool call.
func TestAMisspelledFieldIsRefused(t *testing.T) {
	registry := &catalogue{out: createdContainer()}

	recorder := postContainer(t, registry, `{"type":"HUB","name":"Private","colour_token":"blue"}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422: %s", recorder.Code, recorder.Body)
	}
	if registry.invoked {
		t.Error("the use case ran on a body nobody declared")
	}

	var problem map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the response is not JSON: %v", err)
	}
	findings, _ := problem["field_errors"].([]any)
	if len(findings) != 1 {
		t.Fatalf("field errors = %v", problem["field_errors"])
	}
	finding, _ := findings[0].(map[string]any)
	if finding["path"] != "/colour_token" || finding["code"] != "usecase.field_unknown" {
		t.Errorf("the finding does not point at the field: %v", finding)
	}
}

func TestAMalformedBodyIsRefused(t *testing.T) {
	for name, body := range map[string]string{
		"broken JSON":   `{"type":"HUB",`,
		"an empty body": ``,
		"a wrong type":  `{"type":"HUB","name":7}`,
	} {
		t.Run(name, func(t *testing.T) {
			registry := &catalogue{out: createdContainer()}

			recorder := postContainer(t, registry, body)
			if recorder.Code != http.StatusBadRequest {
				t.Errorf("status %d, want 400: %s", recorder.Code, recorder.Body)
			}
			if registry.invoked {
				t.Error("the use case ran on a body that could not be read")
			}
		})
	}
}

// A controller without its catalogue is a misconfigured installation, not a bad request.
func TestAControllerWithoutTheCatalogueAnswers500(t *testing.T) {
	controller := NewRestController()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, APIBasePath+"/containers",
		strings.NewReader(`{"type":"HUB","name":"Private"}`))
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500", recorder.Code)
	}
}
