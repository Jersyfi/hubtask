// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// A person's own AI choice over REST: giving consent back, reading the view, acknowledging the
// notice.

func TestAPersonGivesConsentBackOverRest(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"id": "0192f000-0000-7000-8000-0000000000f1", "purpose": "ai_processing", "granted": true,
		"granted_at": time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC), "source": "user",
		"account_id": "0192f000-0000-7000-8000-0000000000b1",
	}}
	recorder := callBackup(t, registry, http.MethodPost, "/privacy/consents:give", `{"purpose": "ai_processing"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "GiveConsent" || registry.in["purpose"] != "ai_processing" {
		t.Errorf("the use case received %s %v", registry.name, registry.in)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if body["granted"] != true || body["source"] != "user" {
		t.Errorf("answered %v", body)
	}
}

func TestAPersonReadsAndAcknowledgesTheirAiPositionOverRest(t *testing.T) {
	since := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	view := usecase.Output{
		"shown": true, "person_opt_out": "NOT_OFFERED", "basis": "WORKS_AGREEMENT",
		"contact": "The works council", "since": since,
		"keeping_out": true, "in_effect": false, "notice_pending": true,
	}
	registry := &catalogue{out: view}
	recorder := callBackup(t, registry, http.MethodGet, "/privacy/ai-objection", "")
	if recorder.Code != http.StatusOK || registry.name != "ReadOwnAiPosition" {
		t.Fatalf("status = %d (%s), body = %s", recorder.Code, registry.name, recorder.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if body["shown"] != true || body["basis"] != "WORKS_AGREEMENT" || body["contact"] != "The works council" ||
		body["notice_pending"] != true || body["in_effect"] != false {
		t.Errorf("answered %v", body)
	}

	recorder = callBackup(t, registry, http.MethodPost, "/privacy/ai-objection:noticed", "")
	if recorder.Code != http.StatusOK || registry.name != "AcknowledgeAiPosition" {
		t.Errorf("status = %d (%s), body = %s", recorder.Code, registry.name, recorder.Body)
	}
}
