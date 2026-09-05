// SPDX-License-Identifier: BUSL-1.1
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

const (
	appID   = "01936f2a-7c1e-7000-8000-0000000005b1"
	grantID = "01936f2a-7c1e-7000-8000-0000000005b2"
)

var (
	appSecret     = "hbt_" + "ocs_theapps"
	knownVerifier = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
)

// A confidential client is answered a secret in clear, once - and a public one is answered none,
// which must not print as an empty line that reads like a secret that failed to arrive.
func TestRegisteringAnAppShowsAConfidentialSecretOnceAndAPublicOneNone(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		answer     string
		wantSecret bool
	}{
		{"confidential", []string{"--confidential"},
			`{"id":"` + appID + `","name":"Kanban","redirect_uris":["https://app.example/cb"],
			  "confidential":true,"created_at":"2026-09-05T09:00:00Z",
			  "client_secret":"` + appSecret + `"}`, true},
		{"public", nil,
			`{"id":"` + appID + `","name":"Kanban","redirect_uris":["https://app.example/cb"],
			  "confidential":false,"created_at":"2026-09-05T09:00:00Z","client_secret":null}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := serveJSON(t, http.StatusCreated, tc.answer)

			args := append([]string{"oauth", "client", "add", "--name", "Kanban",
				"--redirect", "https://app.example/cb"}, tc.args...)
			code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", args...)
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			if !strings.Contains(stub.body, `"redirect_uris":["https://app.example/cb"]`) {
				t.Errorf("the redirect URI did not travel: %s", stub.body)
			}
			if got := strings.Contains(out, appSecret); got != tc.wantSecret {
				t.Errorf("the secret was printed: %v, want %v (%q)", got, tc.wantSecret, out)
			}
			if got := strings.Contains(errOut, "shown once"); got != tc.wantSecret {
				t.Errorf("the warning was printed: %v, want %v (%q)", got, tc.wantSecret, errOut)
			}
		})
	}
}

// Consenting is a person's act, and the verifier never leaves this machine: only its hash does.
// Both halves come back because the exchange is a second command.
func TestConsentingDrawsAVerifierAndSendsOnlyItsHash(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated,
		`{"code":"the-code","expires_at":"2026-09-05T09:05:00Z","state":"xyz"}`)
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, out, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "",
		"--json", "oauth", "authorize", "--client", appID,
		"--redirect", "https://app.example/cb", "--scope", "items:read,items:write",
		"--state", "xyz")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var consent OauthConsent
	if err := json.Unmarshal([]byte(out), &consent); err != nil {
		t.Fatalf("the answer is not one document: %v (%q)", err, out)
	}
	if consent.CodeVerifier == "" || consent.Code != "the-code" {
		t.Fatalf("both halves should come back: %+v", consent)
	}
	// 32 bytes of entropy as base64url without padding is 43 characters, which is also RFC 7636's
	// minimum.
	if len(consent.CodeVerifier) != 43 {
		t.Errorf("the verifier is %d characters", len(consent.CodeVerifier))
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the request is not JSON: %v", err)
	}
	if sent["code_challenge"] != challengeFor(consent.CodeVerifier) {
		t.Errorf("the challenge is not the verifier's hash: %v", sent["code_challenge"])
	}
	// `plain` would put the verifier on the wire twice, so there is nothing to choose.
	if sent["code_challenge_method"] != "S256" {
		t.Errorf("challenge method %v", sent["code_challenge_method"])
	}
	if strings.Contains(stub.body, consent.CodeVerifier) {
		t.Error("the verifier itself travelled to the server")
	}
	if scopes, _ := sent["scopes"].([]any); len(scopes) != 2 {
		t.Errorf("the scopes did not travel: %v", sent["scopes"])
	}
}

func TestAVerifierThatWasGivenIsTheOneUsed(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated,
		`{"code":"the-code","expires_at":"2026-09-05T09:05:00Z","state":null}`)
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, out, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "",
		"--json", "oauth", "authorize", "--client", appID,
		"--redirect", "https://app.example/cb", "--scope", "items:read",
		"--verifier", knownVerifier)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, knownVerifier) {
		t.Errorf("the given verifier was not the one answered: %q", out)
	}
	if !strings.Contains(stub.body, challengeFor(knownVerifier)) {
		t.Errorf("the challenge is not the given verifier's hash: %s", stub.body)
	}
}

// The exchange is the app's own call: public, because what authenticates is in the body, and a
// bearer credential beside it would be a second identity the route does not accept.
func TestTheExchangeCarriesNoBearerAndPrintsThePairOnce(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated,
		sessionTokensJSON(firstAccess, firstRefresh, time.Now().Add(15*time.Minute)))
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, out, errOut := invokeAgainst(t, stub,
		map[string]string{envProfile: profile, envClientSecret: "the-apps-secret"}, "",
		"oauth", "token", "--client", appID, "--redirect", "https://app.example/cb",
		"--code", "the-code", "--verifier", knownVerifier, "--confidential")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.Header.Get("Authorization"); got != "" {
		t.Errorf("the exchange carried a bearer credential: %q", got)
	}
	if !strings.Contains(stub.body, `"code_verifier":"`+knownVerifier+`"`) ||
		!strings.Contains(stub.body, `"client_secret":"the-apps-secret"`) ||
		!strings.Contains(stub.body, `"grant_type":"authorization_code"`) {
		t.Errorf("the exchange did not carry what authenticates it: %s", stub.body)
	}
	if !strings.Contains(out, firstAccess) {
		t.Errorf("the app's credential is not where it can be read: %q", out)
	}
	if !strings.Contains(errOut, "shown once") {
		t.Errorf("nothing warns about the single showing: %q", errOut)
	}

	// The pair belongs to the app, not to this shell: the profile still holds the person's own
	// session.
	stored, _ := LoadProfile(profile)
	if stored.Session.AccessToken.Reveal() != firstAccess {
		t.Error("the profile was overwritten")
	}
	if stored.Session.ID != sessionID {
		t.Error("the app's session replaced the person's")
	}
}

func TestGrantsAreListedAndWithdrawn(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"` + grantID + `","client_id":"` + appID + `",
		  "client_name":"Kanban","scopes":["items:read"],
		  "created_at":"2026-09-05T09:00:00Z","last_used_at":null}]`))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "oauth", "grant", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "Kanban") || !strings.Contains(out, "items:read") {
		t.Errorf("the grant listing does not say what was allowed: %q", out)
	}

	code, _, errOut = invokeAgainst(t, stub, signedIn(stub), "", "oauth", "grant", "revoke", grantID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+oauthGrantsPath+"/"+grantID {
		t.Errorf("the withdrawal called %s", stub.request.URL.Path)
	}
	if !strings.Contains(errOut, "next request") {
		t.Errorf("nothing says when it takes effect: %q", errOut)
	}
}
