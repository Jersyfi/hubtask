// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
)

// What a legal hold keeps from an erasure, at the REST door (UC-PRV-03 check 11): the preview and a
// partly completed case, each judged by the contract's schema.

func keptPart(erased bool) map[string]any {
	part := map[string]any{
		"hold_id":    "0192f000-0000-7000-8000-0000000003a1",
		"hold_scope": map[string]any{"kind": "CONTAINER", "id": "0192f000-0000-7000-8000-0000000000b1"},
		"account":    false, "entries": 0, "comments": 2, "assignments": 1, "intake": 0,
	}
	if erased {
		part["erased_at"] = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	} else {
		part["blocked"] = map[string]any{
			"code": "privacy.erasure_blocked_by_rule", "params": map[string]string{"rules": "1"},
		}
	}
	return part
}

func TestThePreviewAnswersWhatEachHoldWouldKeep(t *testing.T) {
	spec := contractSpec(t)
	status, raw := readResponse(t,
		"/privacy/requests/0192f000-0000-7000-8000-0000000000e1/erasure-preview?mode=FULL_DELETE",
		catalogue.Output{"mode": "FULL_DELETE", "kept": []map[string]any{keptPart(false)}})
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, raw)
	}
	problems, err := spec.validateAgainst("ErasurePreview", raw)
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, problem := range problems {
		t.Errorf("ErasurePreview: %s", problem)
	}
	if !strings.Contains(string(raw), `"comments":2`) || !strings.Contains(string(raw), `"kind":"CONTAINER"`) {
		t.Errorf("the preview answered %s", raw)
	}

	// Nothing kept is an empty list, not an absent one: the confirmation reads "no hold reaches".
	_, empty := readResponse(t,
		"/privacy/requests/0192f000-0000-7000-8000-0000000000e1/erasure-preview",
		catalogue.Output{"mode": "ANONYMIZE", "kept": []map[string]any{}})
	if !strings.Contains(string(empty), `"kept":[]`) {
		t.Errorf("an empty preview answered %s", empty)
	}
}

func TestAPartlyCompletedCaseSaysWhatWasKeptAndWhy(t *testing.T) {
	spec := contractSpec(t)
	received := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	partly := catalogue.Output{
		"id": "0192f000-0000-7000-8000-0000000000e2", "kind": "ERASURE", "status": "COMPLETED",
		"received_at": received, "due_at": received.Add(30 * 24 * time.Hour),
		"completed_at": received.Add(time.Hour), "erasure_mode": "FULL_DELETE",
		"kept":             []map[string]any{keptPart(false), keptPart(true)},
		"kept_legal_basis": "ART_17_3_E",
	}
	_, raw := readResponse(t, "/privacy/requests", catalogue.Output{
		"data": []catalogue.Output{partly}, "page": map[string]any{"next_cursor": nil, "has_more": false},
	})

	var page struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &page); err != nil || len(page.Data) != 1 {
		t.Fatalf("the list answered %s", raw)
	}
	problems, err := spec.validateAgainst("DataSubjectRequest", page.Data[0])
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, problem := range problems {
		t.Errorf("DataSubjectRequest: %s", problem)
	}
	for _, want := range []string{`"kept_legal_basis":"ART_17_3_E"`, `"erased_at":"2026-09-01T10:00:00Z"`,
		`"code":"privacy.erasure_blocked_by_rule"`} {
		if !strings.Contains(string(page.Data[0]), want) {
			t.Errorf("the case does not say %s: %s", want, page.Data[0])
		}
	}
}
