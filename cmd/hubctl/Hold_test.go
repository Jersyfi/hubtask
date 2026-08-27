// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const holdID = "01936f2a-7c1e-7000-8000-0000000000e5"

const oneHold = `{"id":"` + holdID + `","scope":{"kind":"CONTAINER","id":"` + collectionID + `"},
  "reason":"the Meier proceedings","placed_by":"` + itemID + `",
  "placed_at":"2026-08-20T08:00:00Z","released_at":null,"released_reason":null}`

func TestPlacingAHoldSendsTheScopeAndTheReason(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneHold)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"hold", "place", "--scope", "CONTAINER", "--id", collectionID, "--reason", "the Meier proceedings")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	scope, _ := sent["scope"].(map[string]any)
	if scope["kind"] != "CONTAINER" || scope["id"] != collectionID {
		t.Errorf("scope %v", sent["scope"])
	}
	if sent["reason"] != "the Meier proceedings" {
		t.Errorf("reason %v", sent["reason"])
	}
	// A hold that is doing something says so, rather than leaving the column blank.
	if !strings.Contains(out, "in force") {
		t.Errorf("output %q", out)
	}
}

func TestPlacingAHoldWithoutAReasonIsRefusedHere(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneHold)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "hold", "place", "--scope", "TENANT")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "--reason") {
		t.Errorf("the complaint does not name what is missing: %q", errOut)
	}
}

func TestListingHoldsAsksForTheReleasedOnesWhenTold(t *testing.T) {
	released := `{"id":"` + holdID + `","scope":{"kind":"TENANT","id":null},"reason":"the audit",
	  "placed_by":"` + itemID + `","placed_at":"2026-08-01T08:00:00Z",
	  "released_at":"2026-08-20T08:00:00Z","released_reason":"the proceedings ended"}`
	stub := serveJSON(t, http.StatusOK, `[`+released+`]`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "hold", "ls", "--include-released")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Query().Get("include_released") != "true" {
		t.Errorf("query %v", stub.request.URL.Query())
	}
	// A lifted hold carries why, which is the whole reason a released one is shown at all.
	if !strings.Contains(out, "the proceedings ended") {
		t.Errorf("output %q", out)
	}
}

// The reason is required because the API requires it, and it is refused here rather than after a
// round trip: the sentence is the same either way, and one of the two is faster.
func TestReleasingAHoldWithoutAReasonNeverReachesTheServer(t *testing.T) {
	var called bool
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(oneHold))
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "hold", "release", holdID)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if called {
		t.Error("a release with no reason was sent anyway")
	}
	if !strings.Contains(errOut, "deletable") {
		t.Errorf("the complaint does not say why the reason matters: %q", errOut)
	}
}

func TestReleasingAHoldPostsTheReasonToItsReleasePath(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneHold)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"hold", "release", holdID, "--reason", "the proceedings ended")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := APIPath + holdsPath + "/" + holdID + ":release"; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
	if !strings.Contains(stub.body, "the proceedings ended") {
		t.Errorf("body %q", stub.body)
	}
}
