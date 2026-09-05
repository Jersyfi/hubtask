// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const otherSessionID = "01936f2a-7c1e-7000-8000-000000000502"

const twoSessions = `[
  {"id":"` + sessionID + `","created_at":"2026-09-05T09:00:00Z",
   "last_used_at":"2026-09-05T09:20:00Z","user_agent":"hubctl/dev",
   "ip_class":"203.0.113.0/24","current":true},
  {"id":"` + otherSessionID + `","created_at":"2026-09-01T08:00:00Z",
   "last_used_at":null,"user_agent":null,"ip_class":null,"current":false}]`

func TestListingSessionsSaysWhichOneIsAnsweringTheCall(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, twoSessions)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "session", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+sessionsPath {
		t.Errorf("the listing called %s", stub.request.URL.Path)
	}
	if !strings.Contains(out, sessionID) || !strings.Contains(out, otherSessionID) {
		t.Errorf("both sessions should be listed: %q", out)
	}
	// Which line is this shell is the column the whole listing exists for.
	if !strings.Contains(out, "THIS ONE") {
		t.Errorf("nothing says which session is answering: %q", out)
	}
	// The network is what was recorded, coarsened where it was recorded (T-01).
	if !strings.Contains(out, "203.0.113.0/24") {
		t.Errorf("the network hint is missing: %q", out)
	}
}

// A session that ended is a credential that refuses on its next request. A profile still holding
// it would make the next command fail with an answer about a credential rather than with the
// plain fact that this shell signed itself out.
func TestRevokingThisShellsOwnSessionForgetsItLocally(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "",
		"session", "revoke", sessionID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodDelete ||
		stub.request.URL.Path != APIPath+sessionRevokePrefix+sessionID {
		t.Errorf("%s %s", stub.request.Method, stub.request.URL.Path)
	}
	if !strings.Contains(errOut, "hubctl login") {
		t.Errorf("nothing says how to come back: %q", errOut)
	}
	stored, _ := LoadProfile(profile)
	if !stored.Session.IsEmpty() {
		t.Error("a session that is over survived in the profile")
	}
}

// Somebody else's session is somebody else's; ending it leaves this shell signed in.
func TestRevokingAnotherSessionLeavesThisOneAlone(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "",
		"session", "revoke", otherSessionID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	stored, _ := LoadProfile(profile)
	if stored.Session.IsEmpty() {
		t.Error("ending somebody else's session ended this one")
	}
}

func TestSigningOutEverywhereEndsThisShellToo(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "",
		"session", "revoke", "--all")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodDelete || stub.request.URL.Path != APIPath+sessionsPath {
		t.Errorf("%s %s", stub.request.Method, stub.request.URL.Path)
	}
	stored, _ := LoadProfile(profile)
	if !stored.Session.IsEmpty() {
		t.Error("signing out everywhere left this shell signed in")
	}
}

// An identifier or --all, and saying neither or both is a mistake in the invocation rather than
// something to guess at.
func TestRevokingNeedsToSayWhichSession(t *testing.T) {
	for _, args := range [][]string{
		{"session", "revoke"},
		{"session", "revoke", sessionID, "--all"},
	} {
		stub := serve(t, func(http.ResponseWriter, *http.Request) {
			t.Errorf("a call was made for %v", args)
		})
		code, _, _ := invokeAgainst(t, stub, signedIn(stub), "", args...)
		if code != exitUsage {
			t.Errorf("%v exited %d, want %d", args, code, exitUsage)
		}
	}
}
