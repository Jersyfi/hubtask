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

// search issues the POST through the router, so that the body is bound the way production binds it.
func search(t *testing.T, registry UseCaseRegistry, body string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})

	request := httptest.NewRequestWithContext(
		ctx, http.MethodPost, APIBasePath+"/items:search", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestASearchReachesTheUseCaseWithEveryPartOfTheRequest(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"data": []usecase.Output{},
		"page": map[string]any{"next_cursor": nil, "has_more": false},
	}}

	response := search(t, registry, `{
		"q": "quarterly report",
		"container_id": "`+sampleContainerID+`",
		"language": "de-AT",
		"include_archived": true,
		"page": {"size": 25, "cursor": "the-previous-page"}
	}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if registry.name != searchItemsUseCase {
		t.Fatalf("the handler ran %q", registry.name)
	}

	in := registry.in
	if in["q"] != "quarterly report" || in["container_id"] != sampleContainerID {
		t.Errorf("the words and the scope arrived as %v / %v", in["q"], in["container_id"])
	}
	if in["language"] != "de-AT" || in["include_archived"] != true {
		t.Errorf("the language and the flags arrived as %v", in)
	}
	if in["size"] != 25 || in["cursor"] != "the-previous-page" {
		t.Errorf("the page arrived as %v / %v", in["size"], in["cursor"])
	}
}

// Nothing is defaulted in this layer. The language a caller did not state is the actor's locale and
// the size they did not state is the contract's default - both are the use case's to decide, so
// that MCP and automation get the same answer as HTTP (ADR-0005).
func TestASearchLeavesWhatWasNotSentToTheUseCase(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"data": []usecase.Output{},
		"page": map[string]any{"next_cursor": nil, "has_more": false},
	}}

	if response := search(t, registry, `{"q": "milk"}`); response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	for _, field := range []string{"container_id", "language"} {
		if value := registry.in[field]; value != "" && value != nil {
			t.Errorf("%s was invented as %v", field, value)
		}
	}
	if _, present := registry.in["size"]; present {
		t.Errorf("a page size was invented: %v", registry.in["size"])
	}
}

// The answer is the ordinary item page: the same shape as `GET /items`, so a client renders hits
// with the code it already has. `data` is an array even when nothing matched - a null would make
// every client check before iterating.
func TestASearchAnswersAnItemPage(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"data": []usecase.Output{{
			"id":            sampleItemID,
			"type":          "TASK",
			"collection_id": sampleContainerID,
			"title":         "Quarterly report",
			"completion": map[string]any{
				"is_completed": false, "completed_at": nil, "completed_by": nil,
			},
			"version": 3,
		}},
		"page": map[string]any{"next_cursor": "the-next-page", "has_more": true},
	}}

	response := search(t, registry, `{"q": "quarterly"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	var body struct {
		Data []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"data"`
		Page struct {
			NextCursor *string `json:"next_cursor"`
			HasMore    bool    `json:"has_more"`
		} `json:"page"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("the response does not decode: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].Title != "Quarterly report" {
		t.Fatalf("the hits are %+v", body.Data)
	}
	if body.Page.NextCursor == nil || *body.Page.NextCursor != "the-next-page" || !body.Page.HasMore {
		t.Errorf("the walk was not carried through: %+v", body.Page)
	}
}

// A body without words is passed on rather than refused here. What "no words" means is the
// domain's answer (`search.words_required`), and this layer refusing it first would be a second
// validator - one that MCP and automation do not go through, and that would therefore have to be
// kept in step with the first for ever (ADR-0005).
func TestASearchWithoutWordsIsLeftToTheUseCase(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"data": []usecase.Output{},
		"page": map[string]any{"next_cursor": nil, "has_more": false},
	}}

	if response := search(t, registry, `{}`); response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if registry.name != searchItemsUseCase {
		t.Fatalf("the handler ran %q", registry.name)
	}
	if registry.in["q"] != "" {
		t.Errorf("the words arrived as %v, want the empty request the domain refuses", registry.in["q"])
	}
}
