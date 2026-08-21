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

// The history over REST (B-11). What this layer owes is the page shape, the code the client renders
// rather than a sentence, and a change set that reaches the wire as the domain built it.

const historyItemID = "0192f000-0000-7000-8000-00000000000e"

func activityPageOutput() usecase.Output {
	at := time.Date(2026, 8, 20, 9, 12, 0, 0, time.UTC)

	return usecase.Output{
		"data": []usecase.Output{
			{
				"id":      "0192f000-0000-7000-8000-0000000009e1",
				"item_id": historyItemID,
				"code":    "activity.item_updated",
				"actor": map[string]any{
					"type": "USER", "id": "0192f000-0000-7000-8000-00000000000d",
				},
				"occurred_at": at,
				"change_set": map[string]any{
					"title": map[string]any{"from": "Milk", "to": "Oat milk"},
					"notes": map[string]any{"changed": true},
				},
			},
			{
				"id":          "0192f000-0000-7000-8000-0000000009e2",
				"item_id":     historyItemID,
				"code":        "activity.item_created",
				"actor":       map[string]any{"type": "SYSTEM", "id": nil},
				"occurred_at": at.Add(-time.Hour),
				"change_set":  map[string]any{},
			},
		},
		"page": map[string]any{"next_cursor": "opaque", "has_more": true},
	}
}

func getActivity(t *testing.T, registry UseCaseRegistry, query string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(
		ctx, http.MethodGet, APIBasePath+"/items/"+historyItemID+"/activity"+query,
		strings.NewReader(""))

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestTheHistoryIsServedAsAPageOfCodes(t *testing.T) {
	registry := &catalogue{out: activityPageOutput()}

	recorder := getActivity(t, registry, "?size=25&cursor=opaque")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "ListActivity" {
		t.Errorf("use case = %q, want ListActivity", registry.name)
	}
	if registry.in["item_id"] != historyItemID {
		t.Errorf("the entry reached the catalogue as %v", registry.in["item_id"])
	}
	if registry.in["size"] != 25 || registry.in["cursor"] != "opaque" {
		t.Errorf("the query reached the catalogue as %v", registry.in)
	}

	var body struct {
		Data []map[string]any `json:"data"`
		Page map[string]any   `json:"page"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	if len(body.Data) != 2 {
		t.Fatalf("%d steps, want 2", len(body.Data))
	}

	// The code, not a sentence: the backend renders no display text (ADR-0011).
	if body.Data[0]["code"] != "activity.item_updated" {
		t.Errorf("the code is %v", body.Data[0]["code"])
	}
	changeSet, _ := body.Data[0]["change_set"].(map[string]any)
	title, _ := changeSet["title"].(map[string]any)
	if title["from"] != "Milk" || title["to"] != "Oat milk" {
		t.Errorf("the rename reads %v", title)
	}
	notes, _ := changeSet["notes"].(map[string]any)
	if notes["changed"] != true {
		t.Errorf("the note reads %v", notes)
	}

	// An actor without an account is null rather than absent: the field is required, and a system
	// step is one a client has to be able to read the same way as any other.
	actor, _ := body.Data[1]["actor"].(map[string]any)
	if actor["type"] != "SYSTEM" {
		t.Errorf("the actor reads %v", actor)
	}
	if id, present := actor["id"]; !present || id != nil {
		t.Errorf("the actor's id is %v, want null", id)
	}
	if body.Page["has_more"] != true || body.Page["next_cursor"] != "opaque" {
		t.Errorf("the walk's state is %v", body.Page)
	}
}

// An entry nothing has happened to renders as an empty array rather than a null: a client draws a
// list either way, and a null would make it check first.
func TestAnEmptyHistoryIsAnEmptyArray(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"data": []usecase.Output{},
		"page": map[string]any{"next_cursor": nil, "has_more": false},
	}}

	recorder := getActivity(t, registry, "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if !strings.Contains(recorder.Body.String(), `"data":[]`) {
		t.Errorf("an empty history renders as %s", recorder.Body)
	}
}

// A step that moved no field carries an empty object, and it has to survive the round trip as one:
// a client reading change_set unconditionally must not find a null there.
func TestAStepThatMovedNothingKeepsItsEmptyChangeSet(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"data": []usecase.Output{
			{
				"id":          "0192f000-0000-7000-8000-0000000009e3",
				"item_id":     historyItemID,
				"code":        "activity.item_archived",
				"actor":       map[string]any{"type": "USER", "id": nil},
				"occurred_at": time.Date(2026, 8, 20, 9, 12, 0, 0, time.UTC),
			},
		},
		"page": map[string]any{"next_cursor": nil, "has_more": false},
	}}

	recorder := getActivity(t, registry, "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if !strings.Contains(recorder.Body.String(), `"change_set":{}`) {
		t.Errorf("the step renders as %s", recorder.Body)
	}
}
