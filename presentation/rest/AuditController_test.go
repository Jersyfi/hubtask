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

// The audit trail over REST (E-09). What this layer owes is the filters reaching the catalogue by
// the names it declares, and the page shape this one path has carried since phase 0 - `items` and
// `next_cursor` rather than `data` and `page`.

func auditPageOutput() usecase.Output {
	at := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)

	return usecase.Output{
		"data": []usecase.Output{
			{
				"id":          "0192f000-0000-7000-8000-00000000000b",
				"seq":         int64(41),
				"occurred_at": at,
				"action":      "membership.role_changed",
				"outcome":     "SUCCESS",
				"severity":    "NOTICE",
				"hash":        "abcd",
				"actor": map[string]any{
					"type": "USER", "id": "0192f000-0000-7000-8000-00000000000d",
					"label": "Anna Beispiel",
				},
				"target": map[string]any{
					"type": "membership", "id": "0192f000-0000-7000-8000-00000000000f",
				},
				"context": map[string]any{"request_id": "req-1", "ip_prefix": "198.51.100.0/24"},
				"changes": []map[string]any{
					{"field": "role", "from": "MEMBER", "to": "ADMIN"},
					{"field": "title", "changed": true, "to_hash": "9f86d0"},
				},
			},
		},
		"page": map[string]any{"next_cursor": "opaque", "has_more": true},
	}
}

func getAudit(t *testing.T, registry UseCaseRegistry, query string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(
		ctx, http.MethodGet, APIBasePath+"/audit"+query, strings.NewReader(""))

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestTheTrailIsServedAsItemsAndACursor(t *testing.T) {
	registry := &catalogue{out: auditPageOutput()}

	recorder := getAudit(t, registry, "?limit=25")
	if recorder.Code != http.StatusOK {
		t.Fatalf("the trail answered %d: %s", recorder.Code, recorder.Body)
	}

	var body struct {
		Items []struct {
			ID         string `json:"id"`
			Seq        int    `json:"seq"`
			Action     string `json:"action"`
			Outcome    string `json:"outcome"`
			Hash       string `json:"hash"`
			OccurredAt string `json:"occurred_at"`
			Actor      struct {
				Type  string `json:"type"`
				ID    string `json:"id"`
				Label string `json:"label"`
			} `json:"actor"`
			Target struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"target"`
			Context struct {
				RequestID string `json:"request_id"`
				IPPrefix  string `json:"ip_prefix"`
			} `json:"context"`
			Changes []map[string]any `json:"changes"`
		} `json:"items"`
		NextCursor *string `json:"next_cursor"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not the shape the contract declares: %v", err)
	}

	if len(body.Items) != 1 {
		t.Fatalf("%d entries came back", len(body.Items))
	}
	entry := body.Items[0]
	if entry.Seq != 41 || entry.Action != "membership.role_changed" || entry.Hash != "abcd" {
		t.Errorf("the entry came back as %+v", entry)
	}
	if entry.Actor.Label != "Anna Beispiel" || entry.Target.Type != "membership" {
		t.Errorf("the actor or the target was lost: %+v", entry)
	}
	if entry.Context.IPPrefix != "198.51.100.0/24" {
		t.Errorf("the truncated address came back as %q", entry.Context.IPPrefix)
	}
	if len(entry.Changes) != 2 || entry.Changes[1]["changed"] != true {
		t.Errorf("the masked change was lost: %v", entry.Changes)
	}
	if body.NextCursor == nil || *body.NextCursor != "opaque" {
		t.Errorf("the cursor came back as %v", body.NextCursor)
	}
}

// The filters have to arrive under the names the descriptor declares - a key the catalogue does
// not know is a 400 on a route that looks implemented.
func TestEveryFilterReachesTheCatalogue(t *testing.T) {
	registry := &catalogue{out: auditPageOutput()}

	recorder := getAudit(t, registry,
		"?from=2026-08-01T00:00:00Z&to=2026-08-27T00:00:00Z&action=auth.&"+
			"actor_id=0192f000-0000-7000-8000-00000000000d&target_type=container&"+
			"target_id=0192f000-0000-7000-8000-00000000000f&outcome=DENIED&cursor=opaque&limit=10")
	if recorder.Code != http.StatusOK {
		t.Fatalf("the trail answered %d: %s", recorder.Code, recorder.Body)
	}

	for field, want := range map[string]any{
		"action":      "auth.",
		"actor_id":    "0192f000-0000-7000-8000-00000000000d",
		"target_type": "container",
		"target_id":   "0192f000-0000-7000-8000-00000000000f",
		"outcome":     "DENIED",
		"cursor":      "opaque",
		"size":        10,
	} {
		if got := registry.in[field]; got != want {
			t.Errorf("%s reached the catalogue as %v, want %v", field, got, want)
		}
	}
	if registry.in["from"] != "2026-08-01T00:00:00Z" {
		t.Errorf("the start of the period reached the catalogue as %v", registry.in["from"])
	}
	if registry.name != listAuditEntriesUseCase {
		t.Errorf("the request ran %q", registry.name)
	}
}

// An empty trail is an empty array rather than a null: a client reading `items` unconditionally is
// what the shape promises.
func TestAnEmptyTrailIsAnEmptyArray(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"data": []usecase.Output{},
		"page": map[string]any{"next_cursor": nil, "has_more": false},
	}}

	recorder := getAudit(t, registry, "")
	if body := recorder.Body.String(); !strings.Contains(body, `"items":[]`) {
		t.Errorf("an empty trail came back as %s", body)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"next_cursor":null`) {
		t.Errorf("the last page carried no explicit null: %s", body)
	}
}
