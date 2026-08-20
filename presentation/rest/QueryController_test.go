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

// query issues the POST through the router, so that the body is bound the way production binds it.
func query(t *testing.T, registry UseCaseRegistry, body string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})

	request := httptest.NewRequestWithContext(
		ctx, http.MethodPost, APIBasePath+"/items:query", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func queryResult(rows []usecase.Output, groups []usecase.Output, total any) usecase.Output {
	return usecase.Output{
		"data":   rows,
		"groups": groups,
		"page":   map[string]any{"next_cursor": nil, "has_more": false},
		"total":  total,
	}
}

// The filter is a grammar, and this layer neither reads nor reshapes it: what the client sent is
// what the use case is handed, because the grammar lives in the domain and MCP and automation do
// not come through here (ADR-0026).
func TestAQueryHandsTheFilterOnUntouched(t *testing.T) {
	registry := &catalogue{out: queryResult(nil, nil, nil)}

	response := query(t, registry, `{
		"scope": {"container_id": "`+sampleContainerID+`", "include_descendants": false},
		"filter": {"op": "AND", "nodes": [
			{"field": "is_completed", "op": "EQ", "value": false},
			{"field": "title", "op": "CONTAINS", "value": "milk"}
		]},
		"sort": [{"field": "title", "dir": "DESC"}],
		"group_by": {"field": "bucket_id", "limit_per_group": 10},
		"page": {"size": 25},
		"include_archived": true,
		"count": "exact"
	}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if registry.name != queryItemsUseCase {
		t.Fatalf("the handler ran %q", registry.name)
	}

	in := registry.in
	if in["scope_container_id"] != sampleContainerID || in["include_descendants"] != false {
		t.Errorf("the scope arrived as %v / %v", in["scope_container_id"], in["include_descendants"])
	}
	if in["size"] != 25 || in["include_archived"] != true || in["count"] != "exact" {
		t.Errorf("the page and the flags arrived as %v", in)
	}

	filter, ok := in["filter"].(map[string]any)
	if !ok || filter["op"] != "AND" {
		t.Fatalf("the filter arrived as %#v", in["filter"])
	}
	if nodes, ok := filter["nodes"].([]any); !ok || len(nodes) != 2 {
		t.Errorf("the filter's nodes arrived as %#v", filter["nodes"])
	}
	if sort, ok := in["sort"].([]any); !ok || len(sort) != 1 {
		t.Errorf("the sort arrived as %#v", in["sort"])
	}
	if group, ok := in["group_by"].(map[string]any); !ok || group["field"] != "bucket_id" {
		t.Errorf("the grouping arrived as %#v", in["group_by"])
	}
}

// A part the client did not send must not reach the catalogue as an empty document: absent is what
// makes the use case's own default apply, and `{}` is a filter that refuses to parse.
func TestAnAbsentPartOfAQueryStaysAbsent(t *testing.T) {
	registry := &catalogue{out: queryResult(nil, nil, nil)}

	if response := query(t, registry, `{"scope": {"item_id": "`+sampleItemID+`"}}`); response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	for _, part := range []string{"filter", "sort", "group_by", "cursor", "size", "count"} {
		if value, present := registry.in[part]; present && value != nil {
			t.Errorf("%s arrived as %#v without having been sent", part, value)
		}
	}
	if registry.in["scope_item_id"] != sampleItemID {
		t.Errorf("the scope arrived as %v", registry.in["scope_item_id"])
	}
}

// Both shapes are always arrays, whichever kind of query was sent: the contract says both are
// present, and a client that had to check for null before iterating would check on every response.
func TestAQueryAnswersBothShapesAsArrays(t *testing.T) {
	t.Run("ungrouped", func(t *testing.T) {
		registry := &catalogue{out: queryResult([]usecase.Output{readItem()}, nil, nil)}

		response := query(t, registry, `{"scope": {"container_id": "`+sampleContainerID+`"}}`)
		body := decodeQueryResult(t, response)

		if rows, _ := body["data"].([]any); len(rows) != 1 {
			t.Errorf("data reads %v", body["data"])
		}
		if groups, ok := body["groups"].([]any); !ok || len(groups) != 0 {
			t.Errorf("groups reads %v, want []", body["groups"])
		}
		if body["total"] != nil {
			t.Errorf("total reads %v, want null", body["total"])
		}
	})

	t.Run("grouped", func(t *testing.T) {
		groups := []usecase.Output{
			{
				"key":         "0192f000-0000-7000-8000-000000000401",
				"count":       2,
				"data":        []usecase.Output{readItem()},
				"page":        map[string]any{"next_cursor": "opaque", "has_more": true},
				"unused_here": nil,
			},
			{
				"key":  nil,
				"data": []usecase.Output{},
				"page": map[string]any{"next_cursor": nil, "has_more": false},
			},
		}
		registry := &catalogue{out: queryResult(nil, groups, 3)}

		response := query(t, registry, `{
			"scope": {"container_id": "`+sampleContainerID+`"},
			"group_by": {"field": "bucket_id"},
			"count": "exact"
		}`)
		body := decodeQueryResult(t, response)

		if rows, ok := body["data"].([]any); !ok || len(rows) != 0 {
			t.Errorf("data reads %v, want []", body["data"])
		}
		rendered, _ := body["groups"].([]any)
		if len(rendered) != 2 {
			t.Fatalf("groups reads %v", body["groups"])
		}

		first, _ := rendered[0].(map[string]any)
		if first["key"] != "0192f000-0000-7000-8000-000000000401" || first["count"] != float64(2) {
			t.Errorf("the first group reads %v", first)
		}
		if page, _ := first["page"].(map[string]any); page["next_cursor"] != "opaque" {
			t.Errorf("a group's own cursor reads %v", first["page"])
		}

		// The entries with no value are a group of their own, keyed null - a board draws that column.
		second, _ := rendered[1].(map[string]any)
		if key, present := second["key"]; !present || key != nil {
			t.Errorf("the group with no value reads key %v", second["key"])
		}
		if body["total"] != float64(3) {
			t.Errorf("total reads %v", body["total"])
		}
	})
}

// A field the contract does not declare is refused rather than ignored, exactly as on every other
// body: a client that misspelled `filter` would otherwise get an unfiltered result and no hint.
func TestAQueryRefusesAnUnknownField(t *testing.T) {
	registry := &catalogue{out: queryResult(nil, nil, nil)}

	response := query(t, registry, `{"scope": {"container_id": "`+sampleContainerID+`"}, "filters": {}}`)
	if response.Code != http.StatusUnprocessableEntity && response.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if registry.invoked {
		t.Error("a body with an unknown field still reached the catalogue")
	}
}

func TestAQueryRefusesAnExpansionItDoesNotServe(t *testing.T) {
	registry := &catalogue{out: queryResult(nil, nil, nil)}

	response := query(t, registry,
		`{"scope": {"container_id": "`+sampleContainerID+`"}, "expand": ["children:1"]}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if registry.invoked {
		t.Error("an unserved expansion still reached the catalogue")
	}
}

func decodeQueryResult(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	return body
}
