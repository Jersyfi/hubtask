// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	privacyservice "github.com/Jersyfi/hubtask/core/application/service/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// UC-PRV-01 check 9 at the REST door: what the router, the decoder and the registry answer to an
// extension that is incomplete or malformed, and the case it answers when it is not. The registry is
// the real one; nothing it refuses reaches the use case, so the use case needs no dependencies here.

const extensionPath = "/privacy/requests/0192f000-0000-7000-8000-0000000000e1:extend"

func extensionActor() context.Context {
	return appshared.ContextWithActor(context.Background(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		Scopes:    []string{"privacy:manage"},
	})
}

func postExtension(t *testing.T, use rest.UseCaseRegistry, body string) (int, rest.Problem, []byte) {
	t.Helper()
	controller := rest.NewRestController()
	controller.UseCases = use

	request := httptest.NewRequestWithContext(extensionActor(), http.MethodPost,
		rest.APIBasePath+extensionPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "0192f000-0000-7000-8000-0000000000f1")
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	var problem rest.Problem
	_ = json.Unmarshal(response.Body.Bytes(), &problem)
	return response.Code, problem, response.Body.Bytes()
}

// D6's REST column: a missing field is absent at the registry, an unknown reason is outside its
// enum, a malformed day never gets past the decoder.
func TestAnIncompleteExtensionIsRefusedWithTheRegistrysCodes(t *testing.T) {
	registry, err := catalogue.NewRegistry(nil, privacyservice.ExtendDataSubjectRequest{}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	complete := map[string]string{
		"due_on": `"2026-11-26"`, "reason": `"COMPLEXITY"`, "informed_on": `"2026-08-26"`,
	}
	body := func(without, replace, with string) string {
		parts := []string{}
		for _, field := range []string{"due_on", "reason", "informed_on"} {
			value := complete[field]
			if field == without {
				continue
			}
			if field == replace {
				value = with
			}
			parts = append(parts, `"`+field+`":`+value)
		}
		return "{" + strings.Join(parts, ",") + "}"
	}

	// A refusal of the input's content is 422, one of its form 400 (api-guidelines.md §6).
	const invalid, malformed = http.StatusUnprocessableEntity, http.StatusBadRequest
	cases := map[string]struct {
		body             string
		status           int
		detail, at, code string
	}{
		"no new deadline":   {body("due_on", "", ""), invalid, "usecase.input_invalid", "/due_on", "usecase.field_required"},
		"no reason":         {body("reason", "", ""), invalid, "usecase.input_invalid", "/reason", "usecase.field_required"},
		"no informed day":   {body("informed_on", "", ""), invalid, "usecase.input_invalid", "/informed_on", "usecase.field_required"},
		"an unknown reason": {body("", "reason", `"HOLIDAYS"`), invalid, "usecase.input_invalid", "/reason", "usecase.field_not_in_enum"},
		"a malformed day":   {body("", "due_on", `"26.11.2026"`), malformed, "request.body_malformed", "", ""},
		"a time, not a day": {body("", "informed_on", `"2026-08-26T10:00:00Z"`), malformed, "request.body_malformed", "", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			status, problem, raw := postExtension(t, registry, c.body)
			if status != c.status || problem.DetailCode != c.detail {
				t.Fatalf("status %d: %s", status, raw)
			}
			if c.at == "" {
				return
			}
			if len(problem.FieldErrors) != 1 || problem.FieldErrors[0].Path != c.at ||
				problem.FieldErrors[0].Code != c.code {
				t.Errorf("the field errors are %+v", problem.FieldErrors)
			}
		})
	}
}

// The answer is a DataSubjectRequest with the extension's fields, judged by the contract's schema.
func TestTheExtensionResponseMatchesTheSchema(t *testing.T) {
	spec := contractSpec(t)
	received := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)

	recording := &recordingCatalogue{}
	answer := extensionAnswer{recording: recording, out: catalogue.Output{
		"id": "0192f000-0000-7000-8000-0000000000e1", "kind": "ACCESS", "status": "RECEIVED",
		"scope": "TENANT", "subject_email": "anna@example.org", "received_at": received,
		"due_at": time.Date(2026, 11, 26, 22, 59, 59, 0, time.UTC), "original_due_at": received.Add(30 * 24 * time.Hour),
		"extension_reason": "COMPLEXITY", "informed_on": "2026-08-26",
	}}
	status, _, raw := postExtension(t, answer,
		`{"due_on":"2026-11-26","reason":"COMPLEXITY","informed_on":"2026-08-26"}`)
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, raw)
	}
	if recording.name != "ExtendDataSubjectRequest" || recording.input["due_on"] != "2026-11-26" ||
		recording.input["informed_on"] != "2026-08-26" || recording.input["reason"] != "COMPLEXITY" {
		t.Errorf("the use case %q was handed %v", recording.name, recording.input)
	}

	problems, err := spec.validateAgainst("DataSubjectRequest", raw)
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, problem := range problems {
		t.Errorf("DataSubjectRequest: %s", problem)
	}
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	if body["original_due_at"] == nil || body["informed_on"] != "2026-08-26" || body["extendable_until"] != nil {
		t.Errorf("the case answered %s", raw)
	}

	// A case that can still be extended answers its bound as a day.
	open := catalogue.Output{
		"id": "0192f000-0000-7000-8000-0000000000e2", "kind": "ACCESS", "status": "RECEIVED",
		"received_at": received, "due_at": received.Add(30 * 24 * time.Hour), "extendable_until": "2026-11-26",
	}
	_, listed := readResponse(t, "/privacy/requests", catalogue.Output{
		"data": []catalogue.Output{open}, "page": map[string]any{"next_cursor": nil, "has_more": false},
	})
	if !strings.Contains(string(listed), `"extendable_until":"2026-11-26"`) {
		t.Errorf("the list answered %s", listed)
	}
}

// extensionAnswer records what it was handed and answers a fixed case.
type extensionAnswer struct {
	recording *recordingCatalogue
	out       catalogue.Output
}

func (a extensionAnswer) Invoke(
	ctx context.Context, name string, actor appshared.ActorContext, in catalogue.Input,
) (catalogue.Output, error) {
	_, _ = a.recording.Invoke(ctx, name, actor, in)
	return a.out, nil
}
