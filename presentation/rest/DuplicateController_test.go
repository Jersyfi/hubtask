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

// The copy route at the mapping layer - the catalogue's answer projected into the contract's
// schema - because that is the layer where a field quietly goes missing (C-11).

func duplicateResult() usecase.Output {
	return usecase.Output{
		"item":   createdItem(),
		"copied": 3,
		"dropped_references": []usecase.Output{{
			"item_id": itemID,
			"kind":    "LABEL",
			"id":      "0192f000-0000-7000-8000-000000000301",
			"code":    "labels.not_in_collection",
		}},
	}
}

func postDuplicate(t *testing.T, registry UseCaseRegistry, body string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost,
		APIBasePath+"/items/"+itemID+":duplicate", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestACopyAnswersWithItsLocationItsVersionAndWhatItLost(t *testing.T) {
	cat := &catalogue{out: duplicateResult()}

	recorder := postDuplicate(t, cat, `{"include_subtree":true}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if location := recorder.Header().Get("Location"); location != APIBasePath+"/items/"+itemID {
		t.Errorf("Location %q", location)
	}
	if tag := recorder.Header().Get("ETag"); tag == "" {
		t.Error("no ETag, so a client has no version to write against")
	}
	if cat.name != "DuplicateWorkItem" || cat.in["include_subtree"] != true {
		t.Errorf("the catalogue was asked for %s with %v", cat.name, cat.in)
	}

	var body struct {
		Item struct {
			ID string `json:"id"`
		} `json:"item"`
		Copied            int `json:"copied"`
		DroppedReferences []struct {
			ItemID string `json:"item_id"`
			Kind   string `json:"kind"`
			ID     string `json:"id"`
			Code   string `json:"code"`
		} `json:"dropped_references"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer does not decode: %v", err)
	}
	if body.Item.ID != itemID || body.Copied != 3 {
		t.Errorf("the answer is %+v", body)
	}
	if len(body.DroppedReferences) != 1 || body.DroppedReferences[0].Kind != "LABEL" ||
		body.DroppedReferences[0].Code != "labels.not_in_collection" ||
		body.DroppedReferences[0].ItemID != itemID {
		t.Errorf("the losses are %+v", body.DroppedReferences)
	}
}

// No body at all is a copy where the entry already is, which is the commonest copy there is - and
// the fields the client did not send are not passed on as empty ones.
func TestACopyWithoutABodyAsksForNothingElse(t *testing.T) {
	cat := &catalogue{out: duplicateResult()}

	controller := NewRestController()
	controller.UseCases = cat
	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodPost,
		APIBasePath+"/items/"+itemID+":duplicate", nil)

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if len(cat.in) != 1 || cat.in["item_id"] != itemID {
		t.Errorf("the catalogue was asked with %v, want the entry alone", cat.in)
	}
}

// The parent sent as null is "the top level of a collection", and omitting it is "beside the
// original". A handler that could not tell them apart would copy entries to places nobody asked
// for, and the value alone cannot tell the two apart.
func TestACopyPassesOnANullParentAndNotAnAbsentOne(t *testing.T) {
	withNull := &catalogue{out: duplicateResult()}
	postDuplicate(t, withNull, `{"target_parent_id":null,"target_collection_id":"`+collectionID+`"}`)

	if _, sent := withNull.in["target_parent_id"]; !sent {
		t.Errorf("an explicit null was dropped: %v", withNull.in)
	}

	omitted := &catalogue{out: duplicateResult()}
	postDuplicate(t, omitted, `{"title":"Plan the second move"}`)

	if _, sent := omitted.in["target_parent_id"]; sent {
		t.Errorf("an absent parent was invented: %v", omitted.in)
	}
	if omitted.in["title"] != "Plan the second move" {
		t.Errorf("the title was not passed on: %v", omitted.in)
	}
}

// The copy carries the same distinction the move does, and lost it the same way: a null
// `target_parent_id` asks for the top level of a collection, and reaching the catalogue as nil would
// spell "the caller said nothing" - which puts the copy beside the original instead.
func TestACopyReadsAnExplicitNullParentAsAnInstruction(t *testing.T) {
	cat := &catalogue{out: duplicateResult()}

	recorder := postDuplicate(t, cat,
		`{"target_parent_id":null,"target_collection_id":"0192f000-0000-7000-8000-0000000004aa"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
	}
	if !cat.in.Present("target_parent_id") {
		t.Fatal("an explicit null read as absent, so the copy lands beside the original")
	}
	parent, err := cat.in.ID("target_parent_id")
	if err != nil {
		t.Fatalf("reading the parent: %v", err)
	}
	if !parent.IsZero() {
		t.Errorf("the parent read as %q, want the zero identifier", parent)
	}
}
