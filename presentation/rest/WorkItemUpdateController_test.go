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

// PATCH /items/{itemId} (B-05). What this layer owes is the merge patch: which members the client
// actually sent, and null told apart from absent. Everything else is the application layer's.

func updatedItem() usecase.Output {
	out := createdItem()
	out["title"] = "Buy oat milk"
	out["updated_at"] = time.Date(2026, 8, 18, 11, 0, 0, 0, time.UTC)
	out["version"] = 2
	return out
}

func patchItem(t *testing.T, registry UseCaseRegistry, body string, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPatch, APIBasePath+"/items/"+itemID,
		strings.NewReader(body))
	request.Header.Set("Content-Type", "application/merge-patch+json")
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestUpdatingAnItemAnswers200WithTheItemAndTheNewETag(t *testing.T) {
	registry := &catalogue{out: updatedItem()}

	recorder := patchItem(t, registry, `{"title":"Buy oat milk"}`, "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "UpdateWorkItem" {
		t.Errorf("use case = %q", registry.name)
	}
	// The version after the change, which is what a client needs to follow this update with another.
	if tag := recorder.Header().Get("ETag"); tag != `"2"` {
		t.Errorf("ETag = %q", tag)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the response is not JSON: %v", err)
	}
	if body["title"] != "Buy oat milk" || body["version"] != float64(2) {
		t.Errorf("body = %v", body)
	}
}

// The whole of what a merge patch means, and the reason presence is read rather than the value: an
// absent member leaves the field alone, so it must not reach the catalogue at all.
func TestAMemberThatWasNotSentDoesNotReachTheCatalogue(t *testing.T) {
	registry := &catalogue{out: updatedItem()}

	patchItem(t, registry, `{"title":"Buy oat milk"}`, "")

	if _, present := registry.in["notes"]; present {
		t.Errorf("an absent member was invented: %v", registry.in)
	}
	if registry.in["title"] != "Buy oat milk" {
		t.Errorf("title = %v", registry.in["title"])
	}
	if registry.in["item_id"] != itemID {
		t.Errorf("item_id = %v", registry.in["item_id"])
	}
}

// And the other side of it: null is an instruction. It reaches the catalogue as the empty string,
// because the domain holds `notes` as a string whose empty value is "none" - inventing a second
// spelling for it here would be a distinction only this layer believed in.
func TestANullMemberClearsTheFieldRatherThanBeingDropped(t *testing.T) {
	registry := &catalogue{out: updatedItem()}

	patchItem(t, registry, `{"notes":null}`, "")

	value, present := registry.in["notes"]
	if !present {
		t.Fatalf("clearing the notes was dropped: %v", registry.in)
	}
	if value != "" {
		t.Errorf("notes = %v, want the empty string", value)
	}
}

func TestAnEmptyStringIsPassedOnAsItself(t *testing.T) {
	registry := &catalogue{out: updatedItem()}

	patchItem(t, registry, `{"notes":""}`, "")

	if value, present := registry.in["notes"]; !present || value != "" {
		t.Errorf("notes = %v (present: %v)", value, present)
	}
}

// The optimistic lock arrives as a header and leaves as a field, so that every channel expresses it
// the same way (api-guidelines.md §5).
func TestTheIfMatchHeaderBecomesTheExpectedVersion(t *testing.T) {
	registry := &catalogue{out: updatedItem()}

	patchItem(t, registry, `{"title":"Buy oat milk"}`, `"1"`)

	if registry.in["expected_version"] != 1 {
		t.Errorf("expected_version = %v", registry.in["expected_version"])
	}
}

func TestWithoutAnIfMatchNoVersionIsInvented(t *testing.T) {
	registry := &catalogue{out: updatedItem()}

	patchItem(t, registry, `{"title":"Buy oat milk"}`, "")

	if _, present := registry.in["expected_version"]; present {
		t.Errorf("a version was invented: %v", registry.in["expected_version"])
	}
}

// A member of the contract that no use case writes yet is passed on so the catalogue refuses it by
// name. Dropping it is the failure that would be worst here: a client that clears a due date with a
// merge patch and receives a 200 believes the reminder is gone.
func TestAMemberNoUseCaseWritesYetIsPassedOnRatherThanDropped(t *testing.T) {
	registry := &catalogue{out: updatedItem()}

	patchItem(t, registry, `{"due_at":null,"title":"Buy oat milk"}`, "")

	if _, present := registry.in["due_at"]; !present {
		t.Errorf("clearing a due date was dropped: %v", registry.in)
	}
}

func TestAnUnknownMemberIsRefused(t *testing.T) {
	registry := &catalogue{out: updatedItem()}

	recorder := patchItem(t, registry, `{"titel":"Buy oat milk"}`, "")

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.invoked {
		t.Error("a request with an unknown member reached the catalogue")
	}
}
