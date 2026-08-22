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

const assigneeID = "0192f000-0000-7000-8000-0000000000f2"

func assignmentRequest(
	t *testing.T, registry UseCaseRegistry, action, body, ifMatch string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost,
		APIBasePath+"/items/"+itemID+":"+action, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func assignedItem() usecase.Output {
	out := createdItem()
	out["assignee_id"] = assigneeID
	out["version"] = 2
	return out
}

func TestAssigningAnswersWithTheEntryAndItsNewETag(t *testing.T) {
	registry := &catalogue{out: assignedItem()}

	recorder := assignmentRequest(t, registry, "assign", `{"account_id":"`+assigneeID+`"}`, "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != assignWorkItemUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	if registry.in["item_id"] != itemID || registry.in["account_id"] != assigneeID {
		t.Errorf("the path and the body did not reach the use case: %+v", registry.in)
	}
	// The version after the change, which is what a client needs in order to follow the assignment
	// with an edit (api-guidelines.md §5).
	if recorder.Header().Get("ETag") != `"2"` {
		t.Errorf("ETag %q, want the version the write produced", recorder.Header().Get("ETag"))
	}

	var item map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &item); err != nil {
		t.Fatalf("the body is not an object: %v (%s)", err, recorder.Body)
	}
	if item["assignee_id"] != assigneeID {
		t.Errorf("assignee_id is %v", item["assignee_id"])
	}
}

// The optimistic lock reaches the use case as a version rather than as a header, which is what lets
// the same rule apply to a call that arrived through MCP.
func TestAssigningPassesOnTheIfMatchVersion(t *testing.T) {
	registry := &catalogue{out: assignedItem()}

	assignmentRequest(t, registry, "assign", `{"account_id":"`+assigneeID+`"}`, `"1"`)

	if registry.in["expected_version"] != 1 {
		t.Errorf("expected_version is %v", registry.in["expected_version"])
	}
}

// No body: an entry has one assignee, and taking them off names nobody.
func TestUnassigningTakesNoBody(t *testing.T) {
	out := createdItem()
	out["assignee_id"] = nil
	registry := &catalogue{out: out}

	recorder := assignmentRequest(t, registry, "unassign", "", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != unassignWorkItemUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	if _, named := registry.in["account_id"]; named {
		t.Errorf("the unassignment named an account: %+v", registry.in)
	}

	var item map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &item); err != nil {
		t.Fatalf("the body is not an object: %v (%s)", err, recorder.Body)
	}
	if _, present := item["assignee_id"]; present {
		t.Errorf("the answer still carries an assignee: %v", item["assignee_id"])
	}
}

// A body that is not an assignment is refused before anything is invoked: the account is the whole
// of what the request says, so there is nothing to fall back on.
func TestAssigningWithoutAnAccountIsRefusedBeforeTheUseCase(t *testing.T) {
	registry := &catalogue{out: assignedItem()}

	recorder := assignmentRequest(t, registry, "assign", `{`, "")

	if recorder.Code < 400 {
		t.Fatalf("status %d, want a refusal: %s", recorder.Code, recorder.Body)
	}
	if registry.invoked {
		t.Error("a malformed body reached the use case")
	}
}
