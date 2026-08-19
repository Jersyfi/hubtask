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

// The container lifecycle over REST (B-06). What this layer owes is the merge patch on the rename -
// which members the client actually sent, null told apart from absent - and the replacement
// semantics on the policies. Everything else is the application layer's.

const containerID = "0192f000-0000-7000-8000-00000000000b"

func renamedContainer() usecase.Output {
	return usecase.Output{
		"id":                 containerID,
		"type":               "COLLECTION",
		"parent_id":          "0192f000-0000-7000-8000-00000000000c",
		"name":               "Groceries",
		"order_key":          "a0",
		"archived_at":        nil,
		"effective_archived": false,
		"deleted_at":         nil,
		"policies":           map[string]any{"completion_policy": "MANUAL"},
		"created_at":         time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC),
		"updated_at":         time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC),
		"version":            4,
	}
}

func containerRequest(
	t *testing.T, registry UseCaseRegistry, method, path, body, ifMatch string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, method, APIBasePath+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func patchContainer(t *testing.T, registry UseCaseRegistry, body, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	return containerRequest(t, registry, http.MethodPatch, "/containers/"+containerID, body, ifMatch)
}

func TestRenamingAContainerAnswers200WithTheContainerAndTheNewETag(t *testing.T) {
	registry := &catalogue{out: renamedContainer()}

	recorder := patchContainer(t, registry, `{"name":"Groceries"}`, "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "RenameContainer" {
		t.Errorf("use case = %q", registry.name)
	}
	// The version after the change, which is what a client needs to follow this update with another.
	if tag := recorder.Header().Get("ETag"); tag != `"4"` {
		t.Errorf("ETag = %q", tag)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the response is not JSON: %v", err)
	}
	if body["name"] != "Groceries" || body["version"] != float64(4) {
		t.Errorf("body = %v", body)
	}
	// Both fields are always in the response: a client that had to guess what an absent
	// `effective_archived` meant would guess wrong on exactly the collections it may not write to.
	if body["effective_archived"] != false {
		t.Errorf("effective_archived = %v, want it present and false", body["effective_archived"])
	}
	policies, ok := body["policies"].(map[string]any)
	if !ok || policies["completion_policy"] != "MANUAL" {
		t.Errorf("policies = %v", body["policies"])
	}
}

// The whole of what a merge patch means, and the reason presence is read rather than the value: an
// absent member leaves the field alone, so it must not reach the catalogue at all.
func TestAContainerMemberThatWasNotSentDoesNotReachTheCatalogue(t *testing.T) {
	registry := &catalogue{out: renamedContainer()}

	patchContainer(t, registry, `{"name":"Groceries"}`, "")

	for _, absent := range []string{"description", "icon", "color_token"} {
		if _, present := registry.in[absent]; present {
			t.Errorf("the absent member %q was invented: %v", absent, registry.in)
		}
	}
	if registry.in["container_id"] != containerID {
		t.Errorf("container_id = %v", registry.in["container_id"])
	}
}

// Null is an instruction, and it is a different one from absence: it clears the field. Both arrive
// as a nil pointer in the generated struct, which is why this layer reads presence.
func TestANullMemberClearsTheFieldRatherThanBeingIgnored(t *testing.T) {
	registry := &catalogue{out: renamedContainer()}

	patchContainer(t, registry, `{"icon":null}`, "")

	if registry.in["icon"] != "" {
		t.Errorf("icon = %v, want the empty string that clears it", registry.in["icon"])
	}
	if _, present := registry.in["name"]; present {
		t.Error("a request that only cleared the icon also sent a name")
	}
}

func TestTheIfMatchHeaderReachesTheCatalogueAsAVersion(t *testing.T) {
	registry := &catalogue{out: renamedContainer()}

	patchContainer(t, registry, `{"name":"Groceries"}`, `"3"`)

	if registry.in["expected_version"] != 3 {
		t.Errorf("expected_version = %v", registry.in["expected_version"])
	}
}

// An empty body is a request that says nothing. The catalogue decides that, not this layer - but
// the request has to reach it, rather than being turned into something else on the way.
func TestAnEmptyRenameBodyStillReachesTheCatalogue(t *testing.T) {
	registry := &catalogue{out: renamedContainer()}

	patchContainer(t, registry, `{}`, "")

	if !registry.invoked {
		t.Error("an empty merge patch was answered without asking the catalogue")
	}
	if len(registry.in) != 1 {
		t.Errorf("input = %v, want the identifier alone", registry.in)
	}
}

// Unknown fields are refused rather than ignored: a client that misspells `color_token` and
// receives a 200 has changed nothing and has no way to find out.
func TestAnUnknownRenameFieldIsRefused(t *testing.T) {
	registry := &catalogue{out: renamedContainer()}

	recorder := patchContainer(t, registry, `{"colour_token":"green"}`, "")

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.invoked {
		t.Error("a body with an unknown field reached the catalogue")
	}
}

func TestUpdatingThePoliciesAnswers200AndNamesTheUseCase(t *testing.T) {
	out := renamedContainer()
	out["policies"] = map[string]any{"completion_policy": "ROLLUP"}
	registry := &catalogue{out: out}

	recorder := containerRequest(t, registry, http.MethodPut,
		"/containers/"+containerID+"/policies", `{"completion_policy":"ROLLUP"}`, `"3"`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "UpdateContainerPolicies" {
		t.Errorf("use case = %q", registry.name)
	}
	if registry.in["completion_policy"] != "ROLLUP" || registry.in["expected_version"] != 3 {
		t.Errorf("input = %v", registry.in)
	}
	if tag := recorder.Header().Get("ETag"); tag != `"4"` {
		t.Errorf("ETag = %q", tag)
	}
}

// A PUT replaces, so an omitted key is the default rather than "leave it alone". The handler
// therefore sends nothing at all for it, and the catalogue reads that as the default - there is no
// third state for this operation to express.
func TestAnOmittedPolicyKeyIsNotSentAsAnInstruction(t *testing.T) {
	registry := &catalogue{out: renamedContainer()}

	containerRequest(t, registry, http.MethodPut,
		"/containers/"+containerID+"/policies", `{}`, "")

	if _, present := registry.in["completion_policy"]; present {
		t.Errorf("an omitted key was invented: %v", registry.in)
	}
	if !registry.invoked {
		t.Error("an empty policies document was answered without asking the catalogue")
	}
}
