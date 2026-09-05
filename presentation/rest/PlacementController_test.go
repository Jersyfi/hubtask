// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The move route at the mapping layer, and the one distinction it exists to carry: `target_parent_id`
// sent as null asks for the top level, omitted asks for the parent to stay. Both reach the catalogue
// as one map, so this is the only layer at which the difference can be proved to survive.

const moveCollectionID = "0192f000-0000-7000-8000-0000000004aa"

func postMove(t *testing.T, registry UseCaseRegistry, body string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost,
		APIBasePath+"/items/"+itemID+":move", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// An explicit null is an instruction, and the use case reads instructions by presence. A value the
// catalogue reads as absent is how "move this out to the level above" became a 200 that moved nothing.
func TestAnExplicitNullParentReachesTheCatalogueAsAnInstruction(t *testing.T) {
	cat := &catalogue{out: usecase.Output{"item": createdItem()}}

	recorder := postMove(t, cat, `{"target_parent_id":null,"target_collection_id":"`+moveCollectionID+`"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if !cat.in.Present("target_parent_id") {
		t.Fatal("an explicit null read as absent, so the entry keeps the parent it asked to leave")
	}
	parent, err := cat.in.ID("target_parent_id")
	if err != nil {
		t.Fatalf("reading the parent: %v", err)
	}
	if !parent.IsZero() {
		t.Errorf("the parent read as %q, want the zero identifier - that is the top level", parent)
	}
}

// The other half of the same distinction. Without this one, passing the field on unconditionally would
// satisfy the test above and move entries nobody asked to move.
func TestAnOmittedParentDoesNotReachTheCatalogueAtAll(t *testing.T) {
	cat := &catalogue{out: usecase.Output{"item": createdItem()}}

	recorder := postMove(t, cat, `{"target_collection_id":"`+moveCollectionID+`"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if cat.in.Present("target_parent_id") {
		t.Error("a field nobody sent read as present, which reparents entries nobody asked to move")
	}
}

// `target_bucket_id` carries the same shape for the board: null takes the entry off it, absent leaves
// the column where the collection allows.
func TestAnExplicitNullBucketReachesTheCatalogueAsAnInstruction(t *testing.T) {
	cat := &catalogue{out: usecase.Output{"item": createdItem()}}

	recorder := postMove(t, cat, `{"target_bucket_id":null}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if !cat.in.Present("target_bucket_id") {
		t.Error("an explicit null read as absent, so the entry stays on the board it asked to leave")
	}
}
