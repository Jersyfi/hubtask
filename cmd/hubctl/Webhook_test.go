// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The outbound integration surface (G-13). The two things worth testing out here are the two this
// client decides: that a secret printed to a terminal says it is shown once, and that a replay
// names the event it repeats.

const (
	webhookID     = "01936f2a-7c1e-7000-8000-000000000f11"
	deliveryID    = "01936f2a-7c1e-7000-8000-000000000f12"
	eventIdentity = "01936f2a-7c1e-7000-8000-000000000f13"
)

const oneSubscription = `{"id":"` + webhookID + `","target_url":"https://example.org/hooks",
  "event_types":["de.hubtask.work.item.completed.v1"],"state":"ACTIVE","failure_count":0,
  "created_at":"2026-08-20T08:00:00Z","version":1}`

func TestSubscribingShowsTheSecretOnceAndSaysSo(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated,
		`{"id":"`+webhookID+`","target_url":"https://example.org/hooks",
		  "event_types":["de.hubtask.work.item.completed.v1"],"state":"ACTIVE","failure_count":0,
		  "created_at":"2026-08-20T08:00:00Z","version":1,"secret":"whsec_nobody_else_has_this"}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"webhook", "add", "--url", "https://example.org/hooks",
		"--event", "de.hubtask.work.item.completed.v1")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	types, _ := sent["event_types"].([]any)
	if sent["target_url"] != "https://example.org/hooks" || len(types) != 1 {
		t.Errorf("the subscription reached the server as %v", sent)
	}
	if !strings.Contains(out, "whsec_nobody_else_has_this") {
		t.Errorf("the secret is not in the answer: %q", out)
	}
	if !strings.Contains(errOut, "shown once") {
		t.Errorf("the warning is %q", errOut)
	}
}

// An event type is a value somebody pastes, and the flag is repeatable rather than
// comma-separated: a comma inside one would be a silent truncation.
func TestEveryEventTypeIsItsOwnFlag(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneSubscription+``)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"webhook", "add", "--url", "https://example.org/hooks",
		"--event", "de.hubtask.work.item.completed.v1",
		"--event", "de.hubtask.work.item.overdue.v1")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	types, _ := sent["event_types"].([]any)
	if len(types) != 2 {
		t.Errorf("event_types %v", sent["event_types"])
	}
}

// A subscription with no target is a mistake this client names rather than sends.
func TestASubscriptionWithoutATargetIsRefusedHere(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneSubscription)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"webhook", "add", "--event", "de.hubtask.work.item.completed.v1")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "--url") {
		t.Errorf("the complaint does not name what is missing: %q", errOut)
	}
}

// Pausing is a deliberate act and `DISABLED` is a conclusion the system draws, so the client can
// ask for one and not the other.
func TestPausingSendsThePausedStateAndNotDisabled(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneSubscription)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "webhook", "pause", webhookID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["state"] != "PAUSED" {
		t.Errorf("state %v", sent["state"])
	}
}

// A replay carries the event identifier it carried, so a subscriber deduplicating on it recognises
// the repeat - and the client says which one, because that is what makes the repeat checkable.
func TestAReplayNamesTheEventItRepeats(t *testing.T) {
	stub := serveJSON(t, http.StatusAccepted,
		`{"id":"`+deliveryID+`","subscription_id":"`+webhookID+`","event_id":"`+eventIdentity+`",
		  "status":"PENDING","attempt":2,"created_at":"2026-08-20T08:00:00Z"}`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"webhook", "replay", webhookID, deliveryID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.request.URL.Path, webhookID+"/deliveries/"+deliveryID) {
		t.Errorf("the path is %q", stub.request.URL.Path)
	}
	if !strings.Contains(errOut, eventIdentity) {
		t.Errorf("the note does not name the event: %q", errOut)
	}
}

// A delivery that never reached the target shows a dash rather than a zero: a timeout and a 500
// are different failures, and the column has to be able to show which.
func TestADeliveryThatGotNoAnswerSaysSo(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"data":[{"id":"`+deliveryID+`","subscription_id":"`+webhookID+`",
		   "event_id":"`+eventIdentity+`","status":"FAILED","attempt":3,
		   "created_at":"2026-08-20T08:00:00Z","error_code":"dependency.unavailable"}],
		  "page":{"has_more":false,"next_cursor":null}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"webhook", "deliveries", webhookID, "--status", "FAILED")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.URL.Query().Get("status"); got != "FAILED" {
		t.Errorf("status %q", got)
	}
	if !strings.Contains(out, "dependency.unavailable") || !strings.Contains(out, "-") {
		t.Errorf("the table is %q", out)
	}
}

// A rotation with no grace period retires the old secret at once, which is what a leak calls for -
// and it has to be expressible, so zero is sent rather than treated as "unset".
func TestARotationWithoutGraceSendsZero(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"id":"`+webhookID+`","target_url":"https://example.org/hooks",
		  "event_types":["de.hubtask.work.item.completed.v1"],"state":"ACTIVE","failure_count":0,
		  "created_at":"2026-08-20T08:00:00Z","version":2,"secret":"whsec_the_new_one"}`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"webhook", "rotate-secret", webhookID, "--grace", "0")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	grace, present := sent["grace_seconds"]
	if !present || grace != float64(0) {
		t.Errorf("grace_seconds %v (present: %v)", grace, present)
	}
	if !strings.Contains(errOut, "shown once") {
		t.Errorf("the warning is %q", errOut)
	}
}
