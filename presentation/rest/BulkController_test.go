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

	repository "github.com/Jersyfi/hubtask/core/application/repository/idempotency"
	"github.com/Jersyfi/hubtask/core/application/service/idempotency"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The bulk route at the mapping layer (C-11), which owns exactly one thing worth testing here: the
// status each operation would have answered on its own.

func bulkAnswer() usecase.Output {
	return usecase.Output{
		"applied": 2,
		"failed":  1,
		"results": []usecase.Output{
			{"index": 0, "op": "COMPLETE_ITEM", "output": createdItem()},
			{"index": 1, "op": "CREATE_ITEM", "output": createdItem()},
			{"index": 2, "op": "MOVE_ITEM", "problem": usecase.Output{
				"category":    string(shared.CategoryConflict),
				"code":        shared.ErrVersionConflict.Code,
				"detail_code": "items.version_conflict",
				"params":      map[string]string{"item_id": itemID},
			}},
		},
	}
}

func postBulk(t *testing.T, registry UseCaseRegistry, body string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, APIBasePath+"/items:bulk",
		strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

type bulkResponse struct {
	Applied int `json:"applied"`
	Failed  int `json:"failed"`
	Results []struct {
		Index   int  `json:"index"`
		Status  int  `json:"status"`
		Item    *any `json:"item"`
		Problem *struct {
			Status     int    `json:"status"`
			Code       string `json:"code"`
			DetailCode string `json:"detail_code"`
		} `json:"problem"`
	} `json:"results"`
}

// A bulk that half succeeded is not a failed request: HTTP 200, and what happened is per operation.
func TestABulkAnswers200AndTheStatusPerOperation(t *testing.T) {
	cat := &catalogue{out: bulkAnswer()}

	recorder := postBulk(t, cat, `{"operations":[
		{"op":"COMPLETE_ITEM","item_id":"`+itemID+`"},
		{"op":"CREATE_ITEM","payload":{"type":"TASK","title":"Buy milk"}},
		{"op":"MOVE_ITEM","item_id":"`+itemID+`","payload":{"target_parent_id":null}}
	]}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}

	var body bulkResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer does not decode: %v", err)
	}
	if body.Applied != 2 || body.Failed != 1 || len(body.Results) != 3 {
		t.Fatalf("the answer is %+v", body)
	}
	// The status each operation would have answered on its own: a creation answers 201, everything
	// else answers 200, and a refusal answers what api-guidelines.md §6 maps its category to.
	if body.Results[0].Status != http.StatusOK || body.Results[1].Status != http.StatusCreated {
		t.Errorf("the applied operations answered %d and %d",
			body.Results[0].Status, body.Results[1].Status)
	}
	refused := body.Results[2]
	if refused.Status != http.StatusConflict || refused.Problem == nil {
		t.Fatalf("the refused operation is %+v", refused)
	}
	if refused.Problem.Status != http.StatusConflict ||
		refused.Problem.DetailCode != "items.version_conflict" {
		t.Errorf("the problem is %+v", refused.Problem)
	}
	if refused.Item != nil {
		t.Error("a refused operation carries an entry")
	}
	if body.Results[0].Item == nil {
		t.Error("an applied operation carries no entry")
	}
}

// The operations travel to the catalogue as they arrived, payloads and all: what a field means is
// the operation's own declaration.
func TestABulkPassesTheOperationsOnUntouched(t *testing.T) {
	cat := &catalogue{out: bulkAnswer()}

	postBulk(t, cat, `{"atomic":true,"operations":[
		{"op":"UPDATE_ITEM","item_id":"`+itemID+`","payload":{"title":"Oat milk"}}
	]}`)

	if cat.name != "BulkUpdateWorkItems" || cat.in["atomic"] != true {
		t.Fatalf("the catalogue was asked for %s with %v", cat.name, cat.in)
	}
	operations, ok := cat.in["operations"].([]any)
	if !ok || len(operations) != 1 {
		t.Fatalf("the operations arrived as %v", cat.in["operations"])
	}
	operation, _ := operations[0].(map[string]any)
	payload, _ := operation["payload"].(map[string]any)
	if operation["op"] != "UPDATE_ITEM" || operation["item_id"] != itemID || payload["title"] != "Oat milk" {
		t.Errorf("the operation arrived as %v", operation)
	}
}

// A bulk the use case refused whole - over the cap, unreadable - is a refused request, not an answer
// with results in it.
func TestABulkTheUseCaseRefusesIsAProblem(t *testing.T) {
	cat := &catalogue{err: shared.ErrValidation.WithDetail("bulk.too_many_operations")}

	recorder := postBulk(t, cat, `{"operations":[{"op":"COMPLETE_ITEM","item_id":"`+itemID+`"}]}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != ProblemContentType {
		t.Errorf("content type %q", contentType)
	}
}

// The acceptance sentence about a repeat: a replayed Idempotency-Key returns the identical body and
// applies nothing twice.
//
// The guard is generic and tested on its own; what this proves is that the bulk goes through it -
// it is a POST like any other, and a client that lost the answer to five hundred operations must be
// able to ask again without applying them a second time (api-guidelines.md §5).
func TestARepeatedBulkIsReplayedRatherThanApplied(t *testing.T) {
	cat := &catalogue{out: bulkAnswer()}
	controller := NewRestController()
	controller.UseCases = cat

	routes := controller.Routes()
	keeper := &recordingGuard{}
	guarded := Idempotent{Guard: keeper, Routes: routes, Next: routes}

	body := `{"operations":[{"op":"COMPLETE_ITEM","item_id":"` + itemID + `"}]}`
	first := httptest.NewRecorder()
	guarded.ServeHTTP(first, bulkRequest(t, body))

	if first.Code != http.StatusOK || !cat.invoked {
		t.Fatalf("the first bulk answered %d, invoked=%v", first.Code, cat.invoked)
	}

	// The answer as it was stored, which is what the guard replays.
	keeper.stored = &repository.Record{Status: first.Code, Body: first.Body.Bytes()}
	cat.invoked = false

	repeat := httptest.NewRecorder()
	guarded.ServeHTTP(repeat, bulkRequest(t, body))

	if cat.invoked {
		t.Error("the repeat applied the operations a second time")
	}
	if repeat.Code != first.Code || repeat.Body.String() != first.Body.String() {
		t.Errorf("the repeat answered %d %s, want the identical answer",
			repeat.Code, repeat.Body.String())
	}
	if repeat.Header().Get(ReplayedHeader) != "true" {
		t.Error("the repeat does not say it is one")
	}
}

// recordingGuard replays whatever a test stored on it, and runs the handler until then.
type recordingGuard struct{ stored *repository.Record }

func (g *recordingGuard) Begin(
	context.Context, appshared.ActorContext, repository.Key, []byte,
) (idempotency.Attempt, error) {
	return idempotency.Attempt{Replay: g.stored}, nil
}

func (g *recordingGuard) Complete(
	context.Context, appshared.ActorContext, repository.Key, int, []byte,
) error {
	return nil
}

func bulkRequest(t *testing.T, body string) *http.Request {
	t.Helper()

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, APIBasePath+"/items:bulk",
		strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(IdempotencyKeyHeader, "5f9d1f8e-0000-4000-8000-0000000000c1")
	return request
}
