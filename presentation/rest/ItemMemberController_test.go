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

func itemMemberRequest(
	t *testing.T, registry UseCaseRegistry, method string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, method,
		APIBasePath+"/items/"+itemID+"/members/"+assigneeID, strings.NewReader(""))

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestAddingAMemberAnswersWithTheSetTheEntryNowCarries(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"item_id": itemID, "member_ids": []string{assigneeID},
	}}

	recorder := itemMemberRequest(t, registry, http.MethodPut)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != addMemberUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	if registry.in["item_id"] != itemID || registry.in["account_id"] != assigneeID {
		t.Errorf("the pair from the path did not reach the use case: %+v", registry.in)
	}
	// Neither operation touches the entry's own row, so there is no version to write against and no
	// ETag to invite an If-Match with (api-guidelines.md §5).
	if tag := recorder.Header().Get("ETag"); tag != "" {
		t.Errorf("the answer carries an ETag %q for a row it did not touch", tag)
	}

	var set map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &set); err != nil {
		t.Fatalf("the body is not an object: %v (%s)", err, recorder.Body)
	}
	ids, _ := set["member_ids"].([]any)
	if len(ids) != 1 || ids[0] != assigneeID {
		t.Errorf("member_ids is %v", set["member_ids"])
	}
}

func TestRemovingAMemberAnswersWithTheSetTheEntryNowCarries(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"item_id": itemID, "member_ids": []string{}}}

	recorder := itemMemberRequest(t, registry, http.MethodDelete)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != removeMemberUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}

	// An empty array rather than null: a client that iterates the answer should not have to
	// nil-check the field.
	var set map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &set); err != nil {
		t.Fatalf("the body is not an object: %v (%s)", err, recorder.Body)
	}
	if string(set["member_ids"]) != "[]" {
		t.Errorf("member_ids is %s, want an empty array", set["member_ids"])
	}
}
