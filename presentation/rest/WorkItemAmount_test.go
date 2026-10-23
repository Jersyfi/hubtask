// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// An entry's amount (UC-WRK-25 check 2) on the route: carried into the catalogue on a create and a
// merge patch, null clearing it, answered back, and a refusal by its code.

func TestAnAmountTravelsIntoTheCreateAndBack(t *testing.T) {
	out := createdItem()
	out["amount"] = "2"
	registry := &catalogue{out: out}

	recorder := postItem(t, registry, `{"type":"TASK","collection_id":"`+collectionID+`","title":"milk","amount":"2"}`)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.in["amount"] != "2" {
		t.Errorf("amount reached the catalogue as %#v", registry.in["amount"])
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["amount"] != "2" {
		t.Errorf("answered amount %#v", body["amount"])
	}
}

func TestAnAmountIsPatchedAndANullClearsIt(t *testing.T) {
	registry := &catalogue{out: updatedItem()}
	patchItem(t, registry, `{"amount":"3"}`, "")
	if registry.in["amount"] != "3" {
		t.Errorf("amount = %#v", registry.in["amount"])
	}

	registry = &catalogue{out: updatedItem()}
	patchItem(t, registry, `{"amount":null}`, "")
	if value, present := registry.in["amount"]; !present || value != "" {
		t.Errorf("a null amount reached the catalogue as %#v (present %v), want the empty string", value, present)
	}

	registry = &catalogue{out: updatedItem()}
	patchItem(t, registry, `{"title":"milk"}`, "")
	if _, present := registry.in["amount"]; present {
		t.Error("an amount nobody sent reached the catalogue")
	}
}

func TestAnAmountTheCollectionDoesNotOfferIsA422(t *testing.T) {
	registry := &catalogue{err: shared.ErrCapabilityNotSupported.WithDetail("items.capability_not_supported").
		WithParams(map[string]string{"item_type": "TASK", "capability": "AMOUNT"})}
	recorder := postItem(t, registry, `{"type":"TASK","collection_id":"`+collectionID+`","title":"milk","amount":"2"}`)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	var problem map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem["code"] != "capability_not_supported" {
		t.Errorf("problem %v", problem)
	}
}
