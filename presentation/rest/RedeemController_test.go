// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// UC-ID-03 check 8: where the redemption owes the ENROLL step, the route answers 202 with the
// challenge - the step and the invited address, and no token of a session.
func TestARedemptionOwingTheFactorAnswersTheChallenge(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"mfa_required": true, "pending_token": "hbt_pnd_x", "expires_at": time.Now().UTC(),
		"methods": []any{"ENROLL"}, "email": "bert@example.org",
	}}
	controller := NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/auth/invitations:redeem",
		strings.NewReader(`{"token":"hbt_inv_x","password":"seven blue lanterns over the harbour"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)

	if registry.name != redeemInvitationUseCase {
		t.Fatalf("the route asked %q", registry.name)
	}
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("answered %d %s, want 202", recorder.Code, recorder.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body["email"] != "bert@example.org" || body["pending_token"] != "hbt_pnd_x" {
		t.Errorf("the challenge answered %v", body)
	}
	if _, held := body["access_token"]; held {
		t.Error("a session token travels beside the challenge")
	}
}
