// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The change step a sign-in owes - a password the rule refuses, or a start password - is answered
// here, so `hubctl login` finishes what it started: the new password is asked twice and presented
// with the pending credential, and the pair that answers is held.
func TestASignInOwedAChangeAsksTheNewPasswordAndFinishes(t *testing.T) {
	var paths []string
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, ":set-password") {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(sessionTokensJSON(firstAccess, firstRefresh, time.Now().Add(15*time.Minute))))
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"pending_token":"` + pendingCredit + `",
		  "expires_at":"2026-09-05T09:05:00Z","methods":["PASSWORD_CHANGE"]}`))
	})
	profile := filepath.Join(t.TempDir(), "profile.json")

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile},
		"start\nseven blue lanterns\nseven blue lanterns\n",
		"login", "--url", stub.server.URL, "--sign-in-name", "lena")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(paths) != 2 || !strings.HasSuffix(paths[1], ":set-password") {
		t.Fatalf("the change step was not taken: %v", paths)
	}
	if !strings.Contains(stub.body, `"password":"seven blue lanterns"`) || !strings.Contains(stub.body, pendingCredit) {
		t.Errorf("the change did not carry the new password and the pending credential: %s", stub.body)
	}
	if stored, _ := LoadProfile(profile); stored.Session.IsEmpty() {
		t.Error("the finished sign-in held no session")
	}
}

// A second typing that differs is a typing mistake, and nothing is sent.
func TestTwoDifferentTypingsOfTheNewPasswordSendNothing(t *testing.T) {
	var paths []string
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"pending_token":"` + pendingCredit + `",
		  "expires_at":"2026-09-05T09:05:00Z","methods":["PASSWORD_CHANGE"]}`))
	})
	profile := filepath.Join(t.TempDir(), "profile.json")

	code, _, _ := invokeAgainst(t, stub, map[string]string{envProfile: profile},
		"start\nseven blue lanterns\neight blue lanterns\n",
		"login", "--url", stub.server.URL, "--sign-in-name", "lena")
	if code == exitOK {
		t.Error("two different new passwords finished the sign-in")
	}
	if len(paths) != 1 {
		t.Errorf("a mistyped password was sent: %v", paths)
	}
}

// After the code, the change can still be owed: the verify answers 202 and the change follows.
func TestTheChangeOwedAfterTheCodeIsAnsweredToo(t *testing.T) {
	var paths []string
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":set-password"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(sessionTokensJSON(firstAccess, firstRefresh, time.Now().Add(15*time.Minute))))
		case strings.HasSuffix(r.URL.Path, ":verify"):
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"pending_token":"` + pendingCredit + `-change",
			  "expires_at":"2026-09-05T09:05:00Z","methods":["PASSWORD_CHANGE"]}`))
		default:
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"pending_token":"` + pendingCredit + `",
			  "expires_at":"2026-09-05T09:05:00Z","methods":["TOTP","RECOVERY"]}`))
		}
	})
	profile := filepath.Join(t.TempDir(), "profile.json")

	code, _, errOut := invokeAgainst(t, stub,
		map[string]string{envProfile: profile, envTotp: "123456"},
		"old\nseven blue lanterns\nseven blue lanterns\n",
		"login", "--url", stub.server.URL, "--email", "eva@acme.example")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(paths) != 3 || !strings.HasSuffix(paths[2], ":set-password") {
		t.Fatalf("the change after the code was not taken: %v", paths)
	}
}

// A sign-in name travels as the identifier, never as an address the contract would refuse.
func TestASignInNameTravelsAsTheIdentifier(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated,
		sessionTokensJSON(firstAccess, firstRefresh, time.Now().Add(15*time.Minute)))
	profile := filepath.Join(t.TempDir(), "profile.json")

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"login", "--url", stub.server.URL, "--sign-in-name", "lena")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"identifier":"lena"`) || strings.Contains(stub.body, `"email"`) {
		t.Errorf("the sign-in sent %s", stub.body)
	}

	code, _, _ = invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"login", "--url", stub.server.URL, "--sign-in-name", "lena", "--email", "eva@acme.example")
	if code == exitOK {
		t.Error("both an address and a sign-in name were accepted")
	}
}
