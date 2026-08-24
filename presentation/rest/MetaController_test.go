// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	usecase "github.com/Jersyfi/hubtask/core/application/service/meta"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

type capabilities struct {
	result usecase.Capabilities
	err    error
	actor  appshared.ActorContext
}

func (c *capabilities) Execute(_ context.Context, actor appshared.ActorContext) (usecase.Capabilities, error) {
	c.actor = actor
	return c.result, c.err
}

func manifest() usecase.Capabilities {
	return usecase.Capabilities{
		ProductVersion: "1.2.3",
		APIVersion:     usecase.APIVersion,
		TenancyMode:    "single",
		ItemTypes: []work.CapabilityProfile{{
			Type:              work.ItemTask,
			Capabilities:      []work.Capability{work.CapabilityCompletion, work.CapabilityCover},
			AllowedChildTypes: []work.ItemType{work.ItemWorkPackage},
			MaxDepth:          3,
		}},
		QueryFields:   view.Fields(),
		TextLanguages: []string{"de", "en"},
		Limits:        map[string]int64{"max_body_bytes": 1 << 20},
		Features:      map[string]bool{"mail": false},
	}
}

// The manifest is what a client builds its filter editor from, so what it publishes has to be what
// the grammar accepts: a field in the list that the grammar refuses would send a client to build a
// query that cannot run (api-guidelines.md §3).
func TestTheManifestPublishesTheQueryGrammar(t *testing.T) {
	response := serveCapabilities(t, &capabilities{result: manifest()})
	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}

	var body struct {
		QueryFields []struct {
			Field     string   `json:"field"`
			Kind      string   `json:"kind"`
			Operators []string `json:"operators"`
			Values    []string `json:"values"`
			Nullable  bool     `json:"nullable"`
			Sortable  bool     `json:"sortable"`
			Groupable bool     `json:"groupable"`
		} `json:"query_fields"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("the manifest is not JSON: %v", err)
	}
	if len(body.QueryFields) != len(view.Fields()) {
		t.Fatalf("%d fields published, %d in the grammar", len(body.QueryFields), len(view.Fields()))
	}

	published := map[string]bool{}
	for _, field := range body.QueryFields {
		published[field.Field] = true

		declared, known := view.FieldByName(field.Field)
		if !known {
			t.Errorf("%s is published and the grammar refuses it", field.Field)
			continue
		}
		if field.Kind != string(declared.Kind) || len(field.Operators) != len(declared.Operators) {
			t.Errorf("%s is published as %+v", field.Field, field)
		}
		for _, operator := range field.Operators {
			if !declared.Permits(view.Operator(operator)) {
				t.Errorf("%s is published as accepting %s, and does not", field.Field, operator)
			}
		}
	}

	// The two shapes a client cannot guess: an enum's values, and a field that may only be ordered
	// by. Both are in the manifest so that a filter editor does not have to hard-code them.
	for _, field := range body.QueryFields {
		if field.Field == view.FieldType && len(field.Values) != 3 {
			t.Errorf("the item types are published as %v", field.Values)
		}
		if field.Field == view.FieldOrderKey && (len(field.Operators) != 0 || !field.Sortable) {
			t.Errorf("the manual order is published as %+v", field)
		}
	}
	if !published[view.FieldLabels] {
		t.Error("the labels are missing from the manifest")
	}
	// A client builds its filter editor from this list, so a field a use case now writes and the
	// manifest does not name is a field nobody can filter on (C-01).
	for _, field := range []string{view.FieldAssigneeID, view.FieldMembers} {
		if !published[field] {
			t.Errorf("%s is missing from the manifest", field)
		}
	}
}

func serveCapabilities(t *testing.T, reader CapabilityReader) *httptest.ResponseRecorder {
	t.Helper()
	controller := NewRestController()
	controller.Capabilities = reader

	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/meta/capabilities", nil))
	return response
}

func TestTheManifestIsAnsweredFromTheUseCase(t *testing.T) {
	response := serveCapabilities(t, &capabilities{result: manifest()})

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("content type %q", got)
	}

	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("the response is not JSON: %v", err)
	}
	if body["product_version"] != "1.2.3" || body["api_version"] != "v1" {
		t.Errorf("versions = %v / %v", body["product_version"], body["api_version"])
	}

	itemTypes, ok := body["item_types"].([]any)
	if !ok || len(itemTypes) != 1 {
		t.Fatalf("item_types = %v", body["item_types"])
	}
	first, _ := itemTypes[0].(map[string]any)
	if first["type"] != "TASK" || first["max_depth"] != float64(3) {
		t.Errorf("first item type = %v", first)
	}
	if children, _ := first["allowed_child_types"].([]any); len(children) != 1 || children[0] != "WORK_PACKAGE" {
		t.Errorf("allowed_child_types = %v", first["allowed_child_types"])
	}
	if caps, _ := first["capabilities"].([]any); len(caps) != 2 {
		t.Errorf("capabilities = %v", first["capabilities"])
	}
}

// An item type without children answers an empty list rather than a null: a client iterating over
// it should not have to special-case the absence.
func TestATypeWithoutChildrenAnswersAnEmptyList(t *testing.T) {
	result := manifest()
	result.ItemTypes = []work.CapabilityProfile{{
		Type:         work.ItemActivity,
		Capabilities: []work.Capability{work.CapabilityCompletion},
		MaxDepth:     1,
	}}

	response := serveCapabilities(t, &capabilities{result: result})

	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("the response is not JSON: %v", err)
	}
	itemTypes, _ := body["item_types"].([]any)
	first, _ := itemTypes[0].(map[string]any)

	children, ok := first["allowed_child_types"].([]any)
	if !ok {
		t.Fatalf("allowed_child_types = %v", first["allowed_child_types"])
	}
	if len(children) != 0 {
		t.Errorf("allowed_child_types = %v", children)
	}
}

// The endpoint is public, so the actor may be anonymous - and the use case has to be told which,
// because the answer differs.
func TestTheActorIsPassedToTheUseCase(t *testing.T) {
	reader := &capabilities{result: manifest()}
	controller := NewRestController()
	controller.Capabilities = reader

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/meta/capabilities", nil)
	actor := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.ID("018f2a1b-0000-7000-8000-0000000000ab"),
	}
	controller.Routes().ServeHTTP(httptest.NewRecorder(),
		r.WithContext(appshared.ContextWithActor(r.Context(), actor)))

	if reader.actor.TenantID != actor.TenantID {
		t.Errorf("the use case saw %+v", reader.actor)
	}
}

func TestAFailureBecomesAProblemDocument(t *testing.T) {
	response := serveCapabilities(t, &capabilities{err: shared.ErrUnavailable})

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != ProblemContentType {
		t.Errorf("content type %q", got)
	}
}

// A route whose use case the composition root forgot is a defect, not a request the client got
// wrong. It answers 500 with nothing in it but the code and the request ID - not a panic, and not
// an empty 200 that a client would treat as an answer.
func TestAnUnwiredUseCaseIsAnInternalError(t *testing.T) {
	response := httptest.NewRecorder()
	NewRestController().Routes().ServeHTTP(response,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/meta/capabilities", nil))

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", response.Code)
	}

	var problem Problem
	if err := json.NewDecoder(response.Body).Decode(&problem); err != nil {
		t.Fatalf("the body is not a problem document: %v", err)
	}
	if problem.Code != "internal" {
		t.Errorf("code %q", problem.Code)
	}
	// An internal error says nothing beyond its code: what it knows may name a query or a path
	// (security.md §9).
	if problem.DetailCode != "" || len(problem.Params) != 0 {
		t.Errorf("an internal error leaked detail: %+v", problem)
	}
}

// The language picker a client draws for `content_language` is this list, so it travels as an
// array even when the installation can index nothing: absent would read as "this server does not
// know about languages", which is a different statement and one a client acts on (C-08).
func TestTheManifestPublishesTheLanguagesThatCanBeIndexed(t *testing.T) {
	for _, test := range []struct {
		name  string
		given []string
		want  int
	}{
		{"what the installation has", []string{"de", "en"}, 2},
		{"an installation that has none", nil, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			answer := manifest()
			answer.TextLanguages = test.given

			response := serveCapabilities(t, &capabilities{result: answer})
			if response.Code != http.StatusOK {
				t.Fatalf("status %d, body %s", response.Code, response.Body.String())
			}

			var body struct {
				TextLanguages *[]string `json:"text_languages"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("the manifest is not JSON: %v", err)
			}
			if body.TextLanguages == nil {
				t.Fatal("the field is absent, which says the server knows nothing about languages")
			}
			if len(*body.TextLanguages) != test.want {
				t.Errorf("%d languages published, want %d", len(*body.TextLanguages), test.want)
			}
		})
	}
}
