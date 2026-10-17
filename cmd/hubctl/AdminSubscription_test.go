// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const platformSubscriptionJSON = `{"id":"01936f2a-7c1e-7000-8000-000000000f51","target_url":"https://shop.example/hubtask",
  "state":"ACTIVE","dead_letters":4,"from_file":false,"created_at":"2026-10-17T09:00:00Z","version":1,
  "last_delivery_at":null,"last_error":"webhooks.target_unavailable"}`

// A subscription is added behind a fresh step-up, and its secret printed this once.
func TestAPlatformSubscriptionIsAddedBehindAStepUp(t *testing.T) {
	minted := strings.TrimSuffix(platformSubscriptionJSON, "}") + `,"secret":"the-shown-once-secret"}`
	stub, paths, proofs := demandingFor(t, http.MethodPost, adminSubscriptionsPath, http.StatusCreated, minted)
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, out, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"admin", "subscription", "add", "--target", "https://shop.example/hubtask")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(*proofs) != 2 || (*proofs)[1] != stepUpToken {
		t.Fatalf("not retried with the proof: %v (paths %v)", *proofs, *paths)
	}
	if !strings.Contains(stub.body, `"target_url":"https://shop.example/hubtask"`) {
		t.Errorf("sent %s", stub.body)
	}
	if !strings.Contains(out, "the-shown-once-secret") {
		t.Errorf("the secret was not printed: %q", out)
	}
}

func TestAPlatformSubscriptionIsListedPausedAndRedelivered(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[`+platformSubscriptionJSON+`]`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "subscription", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, want := range []string{"https://shop.example/hubtask", "ACTIVE", "4", "webhooks.target_unavailable"} {
		if !strings.Contains(out, want) {
			t.Errorf("%s is missing from %q", want, out)
		}
	}

	pause := serveJSON(t, http.StatusOK, platformSubscriptionJSON)
	if code, _, errOut := invokeAgainst(t, pause, signedIn(pause), "", "admin", "subscription", "pause",
		"01936f2a-7c1e-7000-8000-000000000f51"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var sent map[string]any
	_ = json.Unmarshal([]byte(pause.body), &sent)
	if pause.request.Method != http.MethodPatch || sent["state"] != "PAUSED" ||
		pause.request.URL.Path != APIPath+adminSubscriptionsPath+"/01936f2a-7c1e-7000-8000-000000000f51" {
		t.Errorf("paused with %s %s %s", pause.request.Method, pause.request.URL.Path, pause.body)
	}

	redeliver := serveJSON(t, http.StatusAccepted, `{"queued":4}`)
	code, out, errOut = invokeAgainst(t, redeliver, signedIn(redeliver), "", "admin", "subscription", "redeliver",
		"01936f2a-7c1e-7000-8000-000000000f51")
	if code != exitOK || !strings.Contains(out, "4") ||
		redeliver.request.URL.Path != APIPath+adminSubscriptionsPath+"/01936f2a-7c1e-7000-8000-000000000f51:redeliver" {
		t.Errorf("exit %d (%s), %q, %s", code, errOut, out, redeliver.request.URL.Path)
	}
}

func TestAPlatformSubscriptionsDeadLettersAreListed(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data":[{"id":"01936f2a-7c1e-7000-8000-000000000f61",
	  "subscription_id":"01936f2a-7c1e-7000-8000-000000000f51","event_id":"01936f2a-7c1e-7000-8000-000000000f71",
	  "event_type":"de.hubtask.installation.workspace.suspended.v1","attempt":8,"status":"DEAD_LETTER",
	  "response_status":503,"error_code":"webhooks.target_unavailable","next_attempt_at":null,
	  "created_at":"2026-10-17T09:00:00Z","updated_at":"2026-10-17T09:00:00Z"}],"page":{"next_cursor":null,"has_more":false}}`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "subscription", "deliveries",
		"01936f2a-7c1e-7000-8000-000000000f51", "--status", "DEAD_LETTER")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Query().Get("status") != "DEAD_LETTER" || !strings.Contains(out, "503") || !strings.Contains(out, "DEAD_LETTER") {
		t.Errorf("asked %s, printed %q", stub.request.URL, out)
	}
}
