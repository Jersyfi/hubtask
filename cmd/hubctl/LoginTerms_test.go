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

// The TERMS step (UC-ID-19) is answered here too: the terms are shown by their link and version,
// and the session opens once agreed - asked, or by `--agree-terms`. Declining sends nothing, holds
// nothing, and says whom to ask. A TERMS step is never mistaken for an enrolment.

var termsChallenge = `{"pending_token":"` + pendingCredit + `-terms",
  "expires_at":"2026-09-05T09:05:00Z","methods":["TERMS"],
  "terms":{"url":"https://acme.example/terms","version":"4","set_by":"WORKSPACE"}}`

// termsStub answers the password with the step owed first, and the TERMS step after it.
func termsStub(t *testing.T, first string, paths *[]string) *installation {
	t.Helper()
	return serve(t, func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":agree-terms"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(sessionTokensJSON(firstAccess, firstRefresh, time.Now().Add(15*time.Minute))))
		case strings.HasSuffix(r.URL.Path, ":set-password"), strings.HasSuffix(r.URL.Path, ":verify"):
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(termsChallenge))
		default:
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(first))
		}
	})
}

func TestTheTermsAreAskedAndAgreedBeforeTheSessionIsHeld(t *testing.T) {
	var paths []string
	stub := termsStub(t, termsChallenge, &paths)
	profile := filepath.Join(t.TempDir(), "profile.json")

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\nyes\n",
		"login", "--url", stub.server.URL, "--email", "eva@acme.example")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(errOut, "https://acme.example/terms (version 4)") {
		t.Errorf("the terms were not shown: %s", errOut)
	}
	if len(paths) != 2 || !strings.HasSuffix(paths[1], ":agree-terms") {
		t.Fatalf("the terms step was not taken: %v", paths)
	}
	if !strings.Contains(stub.body, `"terms_url":"https://acme.example/terms"`) ||
		!strings.Contains(stub.body, `"terms_version":"4"`) || !strings.Contains(stub.body, pendingCredit+"-terms") {
		t.Errorf("the agreement sent %s", stub.body)
	}
	if stored, _ := LoadProfile(profile); stored.Session.IsEmpty() {
		t.Error("the finished sign-in held no session")
	}
}

func TestDecliningTheTermsSendsNothingAndSaysWhomToAsk(t *testing.T) {
	var paths []string
	stub := termsStub(t, termsChallenge, &paths)
	profile := filepath.Join(t.TempDir(), "profile.json")

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\nno\n",
		"login", "--url", stub.server.URL, "--email", "eva@acme.example")
	if code == exitOK {
		t.Fatal("declining the terms finished the sign-in")
	}
	if len(paths) != 1 {
		t.Errorf("declining sent %v", paths)
	}
	if !strings.Contains(errOut, "this workspace's administrators") {
		t.Errorf("the refusal does not say whom to ask: %s", errOut)
	}
	if stored, _ := LoadProfile(profile); !stored.Session.IsEmpty() {
		t.Error("a declined sign-in held a session")
	}
}

// After the code and after the change step, the terms can still be owed - and `--agree-terms`
// agrees without the question.
func TestTheTermsAfterTheCodeAndAfterTheChangeAreAnsweredToo(t *testing.T) {
	for name, first := range map[string]string{
		"after the code": `{"pending_token":"` + pendingCredit + `",
		  "expires_at":"2026-09-05T09:05:00Z","methods":["TOTP","RECOVERY"]}`,
		"after the change": `{"pending_token":"` + pendingCredit + `",
		  "expires_at":"2026-09-05T09:05:00Z","methods":["PASSWORD_CHANGE"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var paths []string
			stub := termsStub(t, first, &paths)
			profile := filepath.Join(t.TempDir(), "profile.json")

			code, _, errOut := invokeAgainst(t, stub,
				map[string]string{envProfile: profile, envTotp: "123456"},
				"old\nseven blue lanterns\nseven blue lanterns\n",
				"login", "--url", stub.server.URL, "--email", "eva@acme.example", "--agree-terms")
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			if len(paths) != 3 || !strings.HasSuffix(paths[2], ":agree-terms") {
				t.Fatalf("the terms after %s were not agreed: %v", name, paths)
			}
		})
	}
}
