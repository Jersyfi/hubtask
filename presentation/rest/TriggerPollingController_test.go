// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The polling trigger over REST (G-04). What this layer owes is that the CloudEvent reaches the
// wire as the use case rendered it - every attribute, including the extension attributes a
// field-by-field mapper would drop - and that the query parameters arrive as the catalogue's input.

const polledEventUUID = "0192f000-0000-7000-8000-0000000000e1"

// renderedEvent is the shape ToCloudEvent produces, extension attributes included. Those are the
// ones at risk: they sit beside the specification's own attributes rather than inside `data`.
func renderedEvent() map[string]any {
	return map[string]any{
		"specversion":     "1.0",
		"id":              polledEventUUID,
		"source":          "https://hubtask.example",
		"type":            "de.hubtask.work.item.created.v1",
		"subject":         "item/0192f000-0000-7000-8000-0000000000e2",
		"time":            "2026-08-27T09:00:00Z",
		"datacontenttype": "application/json",
		"tenantid":        "0192f000-0000-7000-8000-00000000000a",
		"actortype":       "USER",
		"correlationid":   "0192f000-0000-7000-8000-0000000000e3",
		"causationdepth":  0,
		"data":            map[string]any{"title_present": true},
	}
}

func pollPage() usecase.Output {
	return usecase.Output{
		"data": []any{renderedEvent()},
		"page": map[string]any{"next_cursor": "Y3Vyc29y", "has_more": true},
	}
}

func callPoll(t *testing.T, registry UseCaseRegistry, path string) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, APIBasePath+path, nil)

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// The whole point of passing the document through unread: an extension attribute added tomorrow
// reaches a poller without this file changing, and cannot be dropped for the pull half alone.
func TestAPolledEventReachesTheWireWhole(t *testing.T) {
	registry := &catalogue{out: pollPage()}

	recorder := callPoll(t, registry,
		"/integrations/triggers/de.hubtask.work.item.created.v1")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", recorder.Code, recorder.Body)
	}

	var body struct {
		Data []map[string]any `json:"data"`
		Page struct {
			NextCursor *string `json:"next_cursor"`
			HasMore    bool    `json:"has_more"`
		} `json:"page"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	if len(body.Data) != 1 {
		t.Fatalf("%d events, want 1", len(body.Data))
	}

	for attribute, want := range renderedEvent() {
		got, present := body.Data[0][attribute]
		if !present {
			t.Errorf("%q did not reach the wire", attribute)
			continue
		}
		// Compared through JSON on both sides, because a number that went out as an integer comes
		// back as a float and the mismatch would be the encoder's rather than the mapper's.
		if encode(t, got) != encode(t, want) {
			t.Errorf("%q is %v, want %v", attribute, got, want)
		}
	}

	if body.Page.NextCursor == nil || *body.Page.NextCursor != "Y3Vyc29y" {
		t.Errorf("cursor %v, want the one the use case answered", body.Page.NextCursor)
	}
	if !body.Page.HasMore {
		t.Error("has_more did not reach the wire")
	}
}

// The type comes from the path and the two query parameters from the query, and all three reach
// the catalogue under the names the descriptor declares.
func TestThePollParametersReachTheCatalogue(t *testing.T) {
	registry := &catalogue{out: pollPage()}

	callPoll(t, registry,
		"/integrations/triggers/de.hubtask.work.item.created.v1?since=Y3Vyc29y&limit=25")

	if registry.name != "PollTriggerEvents" {
		t.Errorf("invoked %q, want PollTriggerEvents", registry.name)
	}
	if got := registry.in["event_type"]; got != "de.hubtask.work.item.created.v1" {
		t.Errorf("event_type is %v", got)
	}
	if got := registry.in["cursor"]; got != "Y3Vyc29y" {
		t.Errorf("cursor is %v", got)
	}
	if got := registry.in["limit"]; got != 25 {
		t.Errorf("limit is %v, want 25", got)
	}
}

// An absent parameter reaches the catalogue as an absent entry rather than as a zero, so that the
// use case's own default applies rather than "start at the beginning, answer nothing".
func TestAbsentPollParametersAreNotSentAsZeroes(t *testing.T) {
	registry := &catalogue{out: pollPage()}

	callPoll(t, registry, "/integrations/triggers/de.hubtask.work.item.created.v1")

	for _, field := range []string{"cursor", "limit"} {
		if _, present := registry.in[field]; present {
			t.Errorf("%q was sent although the request carried none: %v", field, registry.in[field])
		}
	}
}

// A refusal from the use case is a problem document, not a page of nothing.
func TestARefusedPollAnswersAProblem(t *testing.T) {
	registry := &catalogue{err: shared.ErrGone.WithDetail("triggers.cursor_expired")}

	recorder := callPoll(t, registry,
		"/integrations/triggers/de.hubtask.work.item.created.v1?since=stale")
	if recorder.Code != http.StatusGone {
		t.Fatalf("status %d, want 410: %s", recorder.Code, recorder.Body)
	}

	var problem struct {
		DetailCode string `json:"detail_code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	if problem.DetailCode != "triggers.cursor_expired" {
		t.Errorf("detail_code %q, want triggers.cursor_expired", problem.DetailCode)
	}
}

func encode(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encoding %v: %v", value, err)
	}
	return string(raw)
}
