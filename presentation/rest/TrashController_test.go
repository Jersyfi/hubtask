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

// The trash over REST (B-10). What this layer owes is the page shape and the explicit nulls: the
// list mixes containers and entries by design, so a field that appeared only for some rows is one a
// client cannot read unconditionally.

func trashPageOutput() usecase.Output {
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)

	return usecase.Output{
		"data": []usecase.Output{
			{
				"kind":           "CONTAINER",
				"id":             "0192f000-0000-7000-8000-00000000000b",
				"trash_batch_id": "0192f000-0000-7000-8000-0000000000b1",
				"deleted_at":     at,
				"title":          "Private",
				"subtype":        "HUB",
				"hub_id":         nil,
				"collection_id":  nil,
				"parent_id":      nil,
				"version":        3,
			},
			{
				"kind":           "ITEM",
				"id":             "0192f000-0000-7000-8000-00000000000e",
				"trash_batch_id": "0192f000-0000-7000-8000-0000000000b2",
				"deleted_at":     at.Add(-time.Hour),
				"title":          "Weekly shop",
				"subtype":        "TASK",
				"hub_id":         "0192f000-0000-7000-8000-00000000000c",
				"collection_id":  "0192f000-0000-7000-8000-00000000000f",
				"parent_id":      nil,
				"version":        2,
			},
		},
		"page": map[string]any{"next_cursor": "opaque", "has_more": true},
	}
}

func getTrash(t *testing.T, registry UseCaseRegistry, query string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(
		ctx, http.MethodGet, APIBasePath+"/trash"+query, strings.NewReader(""))

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestTheTrashIsServedAsAPageOfBothKinds(t *testing.T) {
	registry := &catalogue{out: trashPageOutput()}

	recorder := getTrash(t, registry, "?size=25&cursor=opaque")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "ListTrash" {
		t.Errorf("use case = %q, want ListTrash", registry.name)
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
		t.Fatalf("%d rows, want 2", len(body.Data))
	}
	if body.Data[0]["kind"] != "CONTAINER" || body.Data[1]["kind"] != "ITEM" {
		t.Errorf("the kinds are %v and %v", body.Data[0]["kind"], body.Data[1]["kind"])
	}
	// The optional identifiers are present as null rather than absent, so a client reads them
	// unconditionally across a list that mixes the two kinds.
	for _, field := range []string{"hub_id", "collection_id", "parent_id"} {
		value, present := body.Data[0][field]
		if !present {
			t.Errorf("the hub's row omits %s rather than sending null", field)
		}
		if value != nil {
			t.Errorf("the hub's %s is %v, want null", field, value)
		}
	}
	if body.Page["has_more"] != true || body.Page["next_cursor"] != "opaque" {
		t.Errorf("the walk's state is %v", body.Page)
	}
}

// An empty trash is an empty array rather than a null: a client renders a list either way, and a
// null would make it check first.
func TestAnEmptyTrashIsAnEmptyArray(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"data": []usecase.Output{},
		"page": map[string]any{"next_cursor": nil, "has_more": false},
	}}

	recorder := getTrash(t, registry, "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if !strings.Contains(recorder.Body.String(), `"data":[]`) {
		t.Errorf("an empty trash renders as %s", recorder.Body)
	}
}

// The named purge answers 204: the caller asked for one entry to go, and how many rows hung off it
// is not a fact about their request (api-guidelines.md §2).
func TestPurgingAnEntryAnswers204(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"removed": 3}}

	controller := NewRestController()
	controller.UseCases = registry
	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	itemID := "0192f000-0000-7000-8000-00000000000e"
	request := httptest.NewRequestWithContext(
		ctx, http.MethodPost, APIBasePath+"/items/"+itemID+":purge", strings.NewReader(""))

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "PurgeWorkItem" || registry.in["item_id"] != itemID {
		t.Errorf("the call reached %q with %v", registry.name, registry.in)
	}
}

// Emptying answers a summary rather than 204: a pass is not necessarily the whole of it, and a
// client that got no answer could not tell that it should call again - nor that rows stayed behind
// under a legal hold.
func TestEmptyingTheTrashAnswersWithTheSummary(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"matched": 12, "removed": 10,
		"blocked": map[string]any{"legal_hold": 1, "tombstone_window": 1},
	}}

	controller := NewRestController()
	controller.UseCases = registry
	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(
		ctx, http.MethodPost, APIBasePath+"/trash:empty", strings.NewReader(""))

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "EmptyTrash" {
		t.Errorf("use case = %q, want EmptyTrash", registry.name)
	}

	var summary struct {
		Matched int            `json:"matched"`
		Removed int            `json:"removed"`
		Blocked map[string]int `json:"blocked"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &summary); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	if summary.Matched != 12 || summary.Removed != 10 {
		t.Errorf("the summary reads %+v", summary)
	}
	if summary.Blocked["legal_hold"] != 1 || summary.Blocked["tombstone_window"] != 1 {
		t.Errorf("the reasons read %v", summary.Blocked)
	}
}

// `blocked` is always an object, empty when nothing was kept: a client rendering "why some stayed"
// should not have to tell an absent map from an empty one.
func TestAnUnblockedPassStillCarriesAnEmptyReasonMap(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"matched": 3, "removed": 3, "blocked": map[string]any{},
	}}

	controller := NewRestController()
	controller.UseCases = registry
	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(
		ctx, http.MethodPost, APIBasePath+"/trash:empty", strings.NewReader(""))

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if !strings.Contains(recorder.Body.String(), `"blocked":{}`) {
		t.Errorf("an unblocked pass renders as %s", recorder.Body)
	}
}
