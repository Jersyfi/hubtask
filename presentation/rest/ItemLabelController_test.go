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

const labelledItem = "0192f000-0000-7000-8000-0000000000e1"

func itemLabelRequest(
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
		APIBasePath+"/items/"+labelledItem+"/labels/"+firstLabel, strings.NewReader(""))

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestAddingALabelAnswersWithTheSetTheEntryNowCarries(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"item_id": labelledItem, "label_ids": []string{firstLabel},
	}}

	recorder := itemLabelRequest(t, registry, http.MethodPut)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != addLabelUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}
	if registry.in["item_id"] != labelledItem || registry.in["label_id"] != firstLabel {
		t.Errorf("the pair from the path did not reach the use case: %+v", registry.in)
	}

	var set map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &set); err != nil {
		t.Fatalf("the body is not an object: %v (%s)", err, recorder.Body)
	}
	ids, _ := set["label_ids"].([]any)
	if len(ids) != 1 || ids[0] != firstLabel {
		t.Errorf("label_ids is %v", set["label_ids"])
	}
}

func TestRemovingALabelAnswersWithTheSetTheEntryNowCarries(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"item_id": labelledItem, "label_ids": []string{},
	}}

	recorder := itemLabelRequest(t, registry, http.MethodDelete)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}
	if registry.name != removeLabelUseCase {
		t.Errorf("the handler invoked %q", registry.name)
	}

	// An empty array rather than null: a client that iterates the answer should not have to
	// nil-check the field.
	var set map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &set); err != nil {
		t.Fatalf("the body is not an object: %v (%s)", err, recorder.Body)
	}
	if string(set["label_ids"]) != "[]" {
		t.Errorf("label_ids is %s, want an empty array", set["label_ids"])
	}
}

// Neither operation touches the entry's own row, so there is no version to write against - and a
// header saying otherwise would invite a client to send an If-Match that nothing here honours.
func TestLabellingAnEntryCarriesNoETag(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"item_id": labelledItem, "label_ids": []string{firstLabel},
	}}

	recorder := itemLabelRequest(t, registry, http.MethodPut)

	if tag := recorder.Header().Get("ETag"); tag != "" {
		t.Errorf("ETag is %q, want none", tag)
	}
}

// The handler holds no rules, so a capability refusal reaches the client unchanged.
func TestLabellingAnActivityIsReportedAsAProblem(t *testing.T) {
	registry := &catalogue{err: shared.ErrCapabilityNotSupported.
		WithDetail("items.capability_not_supported")}

	recorder := itemLabelRequest(t, registry, http.MethodPut)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422: %s", recorder.Code, recorder.Body)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(
		contentType, "application/problem+json") {
		t.Errorf("content type %q", contentType)
	}
}
