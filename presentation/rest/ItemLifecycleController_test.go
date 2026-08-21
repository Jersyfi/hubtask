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

// The entry's lifecycle over REST (B-10). What this layer owes is small and worth pinning down all
// the same: the right use case name, the If-Match read off a header the specification does not
// declare for an action, and the new tag on the way back.

const lifecycleItemID = "0192f000-0000-7000-8000-00000000000e"

func archivedItemOutput() usecase.Output {
	at := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	archivedAt := at.Add(time.Hour)

	return usecase.Output{
		"id":            lifecycleItemID,
		"type":          "TASK",
		"collection_id": "0192f000-0000-7000-8000-00000000000b",
		"parent_id":     nil,
		"path":          "/" + lifecycleItemID + "/",
		"depth":         1,
		"title":         "Weekly shop",
		"completion": map[string]any{
			"is_completed": false, "completed_at": nil, "completed_by": nil,
		},
		"bucket_id":   nil,
		"order_key":   "a0",
		"archived_at": archivedAt,
		"deleted_at":  nil,
		"created_by":  "0192f000-0000-7000-8000-00000000000d",
		"created_at":  at,
		"updated_at":  archivedAt,
		"version":     5,
	}
}

// lifecycleAction issues one lifecycle POST through the router, so that the path parameter is bound
// the way production binds it.
func lifecycleAction(
	t *testing.T, registry UseCaseRegistry, suffix, ifMatch string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(
		ctx, http.MethodPost, APIBasePath+"/items/"+lifecycleItemID+":"+suffix, strings.NewReader(""))
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestTheArchiveActionsReachTheirUseCasesAndAnswerWithTheEntry(t *testing.T) {
	for _, c := range []struct {
		suffix  string
		useCase string
	}{
		{"archive", "ArchiveWorkItem"},
		{"unarchive", "UnarchiveWorkItem"},
	} {
		t.Run(c.suffix, func(t *testing.T) {
			registry := &catalogue{out: archivedItemOutput()}

			recorder := lifecycleAction(t, registry, c.suffix, "")

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
			if registry.name != c.useCase {
				t.Errorf("use case = %q, want %q", registry.name, c.useCase)
			}
			if registry.in["item_id"] != lifecycleItemID {
				t.Errorf("item_id = %v", registry.in["item_id"])
			}
			// No If-Match was sent, so no version travels: the caller read none and accepts whatever
			// is there. A zero passed on would be indistinguishable from one, which is a version.
			if _, sent := registry.in["expected_version"]; sent {
				t.Errorf("a version was passed on without an If-Match: %v", registry.in)
			}
			// The tag is the version *after* the change, which is what a client needs in order to
			// follow the action with an edit (api-guidelines.md §5).
			if tag := recorder.Header().Get("ETag"); tag != `"5"` {
				t.Errorf("ETag = %q, want \"5\"", tag)
			}

			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("the answer is not JSON: %v", err)
			}
			if body["archived_at"] == nil {
				t.Error("the answer does not carry the archive stamp")
			}
		})
	}
}

// The action routes carry no If-Match in the specification, so the header is read off the request.
// It still has to arrive: a client that read version 4 and archives has said which state it is
// acting on.
func TestTheArchiveActionPassesOnTheIfMatch(t *testing.T) {
	registry := &catalogue{out: archivedItemOutput()}

	if recorder := lifecycleAction(t, registry, "archive", `"4"`); recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.in["expected_version"] != 4 {
		t.Errorf("expected_version = %v, want 4", registry.in["expected_version"])
	}
}

// A version conflict from the application layer reaches the client as the contract's 409, with the
// detail code that says which conflict it is - not as a 500.
func TestAStaleArchiveAnswers409(t *testing.T) {
	registry := &catalogue{err: shared.ErrVersionConflict.WithDetail("items.version_conflict")}

	recorder := lifecycleAction(t, registry, "archive", `"2"`)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var problem map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	if problem["code"] != "version_conflict" {
		t.Errorf("code = %v, want version_conflict", problem["code"])
	}
}

// The deletion is a DELETE with no body back. What the client has to know is where to find the
// entry again, and that is the trash rather than this response (api-guidelines.md §2).
func TestTrashingAnEntryAnswers204AndPassesOnTheIfMatch(t *testing.T) {
	registry := &catalogue{out: archivedItemOutput()}

	controller := NewRestController()
	controller.UseCases = registry
	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(
		ctx, http.MethodDelete, APIBasePath+"/items/"+lifecycleItemID, strings.NewReader(""))
	request.Header.Set("If-Match", `"3"`)

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("a 204 carries a body: %s", recorder.Body)
	}
	if registry.name != "TrashWorkItem" {
		t.Errorf("use case = %q, want TrashWorkItem", registry.name)
	}
	if registry.in["expected_version"] != 3 {
		t.Errorf("expected_version = %v, want 3", registry.in["expected_version"])
	}
}

// The restore is an action with the entry back, because the client has to redraw it and needs the
// version it now has.
func TestRestoringAnEntryAnswers200WithTheEntry(t *testing.T) {
	registry := &catalogue{out: archivedItemOutput()}

	recorder := lifecycleAction(t, registry, "restore", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "RestoreWorkItem" {
		t.Errorf("use case = %q, want RestoreWorkItem", registry.name)
	}
	if tag := recorder.Header().Get("ETag"); tag != `"5"` {
		t.Errorf("ETag = %q, want \"5\"", tag)
	}
}
