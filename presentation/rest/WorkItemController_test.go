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

const (
	itemID       = "0192f000-0000-7000-8000-000000000201"
	collectionID = "0192f000-0000-7000-8000-00000000000c"
)

func createdItem() usecase.Output {
	return usecase.Output{
		"id":            itemID,
		"type":          "TASK",
		"collection_id": collectionID,
		"parent_id":     nil,
		"path":          "/" + itemID + "/",
		"depth":         1,
		"title":         "Buy milk",
		"completion": map[string]any{
			"is_completed": false, "completed_at": nil, "completed_by": nil,
		},
		"order_key":  "a0",
		"created_by": "0192f000-0000-7000-8000-00000000000d",
		"created_at": time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
		"updated_at": time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
		"version":    1,
	}
}

func postItem(t *testing.T, registry UseCaseRegistry, body string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, APIBasePath+"/items",
		strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestCreatingAnItemAnswers201WithTheItem(t *testing.T) {
	registry := &catalogue{out: createdItem()}

	recorder := postItem(t, registry,
		`{"type":"TASK","collection_id":"`+collectionID+`","title":"Buy milk"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "CreateWorkItem" {
		t.Errorf("use case = %q", registry.name)
	}
	// Where it is and which version to write against, so a client can follow up without guessing.
	if location := recorder.Header().Get("Location"); location != APIBasePath+"/items/"+itemID {
		t.Errorf("Location = %q", location)
	}
	if tag := recorder.Header().Get("ETag"); tag != `"1"` {
		t.Errorf("ETag = %q", tag)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the response is not JSON: %v", err)
	}
	if body["id"] != itemID || body["type"] != "TASK" || body["title"] != "Buy milk" {
		t.Errorf("body = %v", body)
	}
	if body["path"] != "/"+itemID+"/" || body["depth"] != float64(1) {
		t.Errorf("the placement is missing from the response: %v", body)
	}
	// Required by the schema, so it is there even where it is always the same answer.
	completion, _ := body["completion"].(map[string]any)
	if completion["is_completed"] != false {
		t.Errorf("completion = %v", body["completion"])
	}
	if _, present := body["notes"]; present {
		t.Error("an unset note was returned anyway")
	}
}

// The catalogue is what the request goes through, so the mapping into it is what this layer owes.
func TestTheRequestIsHandedToTheCatalogueAsItArrived(t *testing.T) {
	registry := &catalogue{out: createdItem()}

	postItem(t, registry, `{"type":"WORK_PACKAGE","parent_id":"`+itemID+`","title":"Dairy",`+
		`"notes":"Semi-skimmed"}`)

	if registry.in["type"] != "WORK_PACKAGE" || registry.in["title"] != "Dairy" {
		t.Errorf("input = %v", registry.in)
	}
	if registry.in["parent_id"] != itemID || registry.in["notes"] != "Semi-skimmed" {
		t.Errorf("input = %v", registry.in)
	}
	// An omitted collection stays omitted rather than becoming an empty identifier: the use case
	// takes it from the parent, and an empty string would be a different instruction.
	if registry.in["collection_id"] != nil {
		t.Errorf("collection_id = %v, want nothing", registry.in["collection_id"])
	}
}

// The specification is the whole of 1.0 and this installation serves a part of it. A field no use
// case writes yet is passed on so the catalogue refuses it by name - never dropped, which is how
// a client comes to believe there is a due date.
func TestAFieldNoUseCaseWritesYetIsPassedOnRatherThanDropped(t *testing.T) {
	cases := map[string]string{
		"bucket_id":     `"bucket_id":"` + collectionID + `"`,
		"assignee_id":   `"assignee_id":"` + collectionID + `"`,
		"due_at":        `"due_at":"2026-09-01T09:00:00Z"`,
		"label_ids":     `"label_ids":["` + collectionID + `"]`,
		"custom_fields": `"custom_fields":{"size":"L"}`,
		"auto_assign":   `"auto_assign":true`,
	}

	for field, fragment := range cases {
		t.Run(field, func(t *testing.T) {
			registry := &catalogue{out: createdItem()}

			postItem(t, registry, `{"type":"TASK","collection_id":"`+collectionID+`",`+
				`"title":"Buy milk",`+fragment+`}`)

			if _, passed := registry.in[field]; !passed {
				t.Errorf("%s was dropped rather than passed on to be refused: %v", field, registry.in)
			}
		})
	}
}

// The other half of the same rule: a field that was not sent is not invented, or every request
// would be refused for naming fields the catalogue does not declare.
func TestAFieldThatWasNotSentIsNotInvented(t *testing.T) {
	registry := &catalogue{out: createdItem()}

	postItem(t, registry, `{"type":"TASK","collection_id":"`+collectionID+`","title":"Buy milk"}`)

	// `bucket_id` is not in this list any more: it is served since B-09, and travels like
	// `parent_id` and `notes` - always sent, as null when the client named none.
	for _, absent := range []string{
		"before_item_id", "assignee_id", "auto_assign", "label_ids", "member_ids",
		"due_at", "due_date_only", "due_time_zone", "cover", "custom_fields",
	} {
		if _, present := registry.in[absent]; present {
			t.Errorf("%s was sent to the catalogue although the client omitted it", absent)
		}
	}
}

// A misspelled field is refused rather than accepted, for the reason it is on containers: a client
// that misspells `parent_id` and receives a 201 has created something in the wrong place and has
// no way to find out.
func TestAnUnknownFieldIsRefused(t *testing.T) {
	registry := &catalogue{out: createdItem()}

	recorder := postItem(t, registry,
		`{"type":"TASK","collection_id":"`+collectionID+`","title":"Buy milk","parent":"x"}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.invoked {
		t.Error("the use case ran on a request that was not understood")
	}
}

// A refusal from the application layer reaches the client as the problem document it is, with the
// code that says which of the reasons it was.
func TestARefusedPlacementIsReportedAsAProblem(t *testing.T) {
	registry := &catalogue{err: shared.ErrCapabilityNotSupported.
		WithDetail("items.capability_not_supported").
		WithParams(map[string]string{"item_type": "ACTIVITY", "capability": "NOTES"}).
		WithFields(shared.FieldError{Path: "/notes", Code: "items.capability_not_supported"})}

	recorder := postItem(t, registry,
		`{"type":"ACTIVITY","parent_id":"`+itemID+`","title":"Milk","notes":"Semi-skimmed"}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	var problem map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the problem is not JSON: %v", err)
	}
	if problem["code"] != "capability_not_supported" {
		t.Errorf("code = %v", problem["code"])
	}
	if problem["detail_code"] != "items.capability_not_supported" {
		t.Errorf("detail code = %v", problem["detail_code"])
	}
	// The params are what let a client render a sentence without the server writing one
	// (ADR-0011).
	params, _ := problem["params"].(map[string]any)
	if params["item_type"] != "ACTIVITY" || params["capability"] != "NOTES" {
		t.Errorf("params = %v", problem["params"])
	}
}

// The column an entry sits in travels like the title: sent when the client sent it, and null and
// the empty string alike take the entry off the board (B-09).
func TestTheColumnTravelsWithTheItemUpdate(t *testing.T) {
	for _, c := range []struct {
		name string
		body string
		want any
	}{
		{
			name: "a column named",
			body: `{"bucket_id":"0192f000-0000-7000-8000-0000000000b1"}`,
			want: "0192f000-0000-7000-8000-0000000000b1",
		},
		{name: "null takes it off the board", body: `{"bucket_id":null}`, want: ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			registry := &catalogue{out: createdItem()}

			patchItem(t, registry, c.body, "")

			if registry.in["bucket_id"] != c.want {
				t.Errorf("bucket_id reached the use case as %v, want %v",
					registry.in["bucket_id"], c.want)
			}
		})
	}
}
