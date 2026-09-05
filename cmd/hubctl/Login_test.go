// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	sessionID = "01936f2a-7c1e-7000-8000-000000000501"
	tenantID  = "01936f2a-7c1e-7000-8000-0000000005e0"
)

// Assembled rather than written out, for the reason `mintedToken` is: a literal that looks like a
// credential is what the secret scan of SG-7 exists to find.
var (
	firstAccess   = "hbt_" + "sat_first"
	firstRefresh  = "hbt_" + "srt_first"
	nextAccess    = "hbt_" + "sat_next"
	nextRefresh   = "hbt_" + "srt_next"
	pendingCredit = "hbt_" + "mfa_pending"
)

// sessionTokensJSON is the pair, as the contract answers it.
func sessionTokensJSON(access, refresh string, accessExpiry time.Time) string {
	return `{"token_type":"Bearer",
	  "access_token":"` + access + `",
	  "access_token_expires_at":"` + accessExpiry.UTC().Format(time.RFC3339) + `",
	  "refresh_token":"` + refresh + `",
	  "refresh_token_expires_at":"` + accessExpiry.Add(720*time.Hour).UTC().Format(time.RFC3339) + `",
	  "session":{"id":"` + sessionID + `","created_at":"2026-09-05T09:00:00Z","current":true}}`
}

// A session is what the profile holds afterwards - not a sentence about one. The pair is answered
// once, and a client that printed it and forgot it would make every later command a second
// sign-in.
func TestAPasswordSignInHoldsThePairInTheProfile(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated,
		sessionTokensJSON(firstAccess, firstRefresh, time.Now().Add(15*time.Minute)))
	profile := filepath.Join(t.TempDir(), "profile.json")

	code, out, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"login", "--url", stub.server.URL, "--email", "eva@acme.example")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+sessionsPath {
		t.Errorf("the sign-in called %s", stub.request.URL.Path)
	}
	// Public by contract: a credential presented beside the body is verified all the same, and
	// there is none to present yet anyway.
	if got := stub.request.Header.Get("Authorization"); got != "" {
		t.Errorf("the sign-in carried a credential: %q", got)
	}
	if !strings.Contains(stub.body, `"password":"hunter2"`) {
		t.Errorf("the password did not travel: %s", stub.body)
	}
	// Nothing on standard output, as `auth login`: a sign-in has no payload.
	if out != "" {
		t.Errorf("standard output carried %q", out)
	}
	// And the credential itself is not printed anywhere.
	if strings.Contains(errOut, firstAccess) || strings.Contains(errOut, firstRefresh) {
		t.Errorf("the pair was printed: %q", errOut)
	}

	stored, err := LoadProfile(profile)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if stored.Session.AccessToken.Reveal() != firstAccess ||
		stored.Session.RefreshToken.Reveal() != firstRefresh {
		t.Errorf("the profile does not hold the pair: %+v", stored.Session)
	}
	if stored.Session.ID != sessionID {
		t.Errorf("the profile does not name the session: %q", stored.Session.ID)
	}
	if !stored.Token.IsEmpty() {
		t.Error("a personal access token was invented beside the session")
	}
}

// The workspace, in an installation running more than one: a sign-in has no credential to read it
// off yet, so it has to be said, and it is said once rather than on every command after it.
func TestTheWorkspaceTravelsOnTheSignInAndIsRemembered(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated,
		sessionTokensJSON(firstAccess, firstRefresh, time.Now().Add(15*time.Minute)))
	profile := filepath.Join(t.TempDir(), "profile.json")

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"login", "--url", stub.server.URL, "--email", "eva@acme.example", "--tenant", tenantID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.Header.Get(restTenantHeader); got != tenantID {
		t.Errorf("%s %q, want the workspace", restTenantHeader, got)
	}
	stored, err := LoadProfile(profile)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if stored.Tenant != tenantID {
		t.Errorf("the workspace was not remembered: %q", stored.Tenant)
	}
}

// The second step, answered from the environment so that a scripted session can produce a code
// that is only good for thirty seconds.
func TestAChallengedSignInPresentsTheCodeAndFinishes(t *testing.T) {
	var paths []string
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, ":verify") {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(sessionTokensJSON(firstAccess, firstRefresh, time.Now().Add(15*time.Minute))))
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"pending_token":"` + pendingCredit + `",
		  "expires_at":"2026-09-05T09:05:00Z","methods":["TOTP","RECOVERY"]}`))
	})
	profile := filepath.Join(t.TempDir(), "profile.json")

	code, _, errOut := invokeAgainst(t, stub,
		map[string]string{envProfile: profile, envTotp: "123456"}, "hunter2\n",
		"login", "--url", stub.server.URL, "--email", "eva@acme.example")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(paths) != 2 || !strings.HasSuffix(paths[1], ":verify") {
		t.Fatalf("the second step was not taken: %v", paths)
	}
	if !strings.Contains(stub.body, `"code":"123456"`) ||
		!strings.Contains(stub.body, pendingCredit) {
		t.Errorf("the completion did not carry the code and the pending credential: %s", stub.body)
	}
	stored, _ := LoadProfile(profile)
	if stored.Session.IsEmpty() {
		t.Error("the completed sign-in held no session")
	}
}

// The enforcement route: the password was right, and what is owed is an enrolment nobody can do
// in one call. The sign-in hands over the pending credential and names the two commands that
// finish it, rather than pretending to have signed anybody in.
func TestASignInOwedAnEnrolmentHandsOverThePendingCredential(t *testing.T) {
	stub := serveJSON(t, http.StatusAccepted, `{"pending_token":"`+pendingCredit+`",
	  "expires_at":"2026-09-05T09:05:00Z","methods":["ENROLL"]}`)
	profile := filepath.Join(t.TempDir(), "profile.json")

	code, out, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"login", "--url", stub.server.URL, "--email", "eva@acme.example")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, pendingCredit) {
		t.Errorf("the pending credential is not where a script can read it: %q", out)
	}
	if !strings.Contains(errOut, "mfa enroll") || !strings.Contains(errOut, "mfa confirm") {
		t.Errorf("nothing says how to finish: %q", errOut)
	}
	// The warning must not land in whatever the output is piped into.
	if strings.Contains(out, "shown once") {
		t.Error("the warning landed on standard output")
	}
	stored, _ := LoadProfile(profile)
	if !stored.Session.IsEmpty() {
		t.Error("a sign-in that did not finish stored a session anyway")
	}
}

// The renewal, which is what makes holding a session worth anything: the access token lives
// fifteen minutes and a person types more than one command in that time.
func TestANearlySpentSessionIsRenewedBeforeTheCommandRuns(t *testing.T) {
	var paths []string
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, ":refresh") {
			_, _ = w.Write([]byte(sessionTokensJSON(nextAccess, nextRefresh, time.Now().Add(15*time.Minute))))
			return
		}
		_, _ = w.Write([]byte(`{"data":[],"page":{"has_more":false,"next_cursor":null}}`))
	})
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Second))

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "",
		"container", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(paths) != 2 || !strings.HasSuffix(paths[0], ":refresh") {
		t.Fatalf("the renewal did not happen first: %v", paths)
	}
	// The rotation retires the presented token as it mints the next one, so the profile has to
	// hold the new one before anything else uses it: a retired token presented again is read as
	// theft and kills the whole family.
	stored, _ := LoadProfile(profile)
	if stored.Session.RefreshToken.Reveal() != nextRefresh {
		t.Errorf("the rotated pair was not written: %+v", stored.Session)
	}
	if got := stub.request.Header.Get("Authorization"); got != "Bearer "+nextAccess {
		t.Errorf("the command was made with %q, want the renewed credential", got)
	}
}

// A renewal the installation refuses is not the end of the run. The family may be gone, and the
// useful next command is `hubctl login` - which a fatal error here would prevent.
func TestARefusedRenewalForgetsTheSessionRatherThanEndingTheRun(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ":refresh") {
			problemJSON(w, http.StatusUnauthorized, map[string]any{
				"status": 401, "code": "unauthenticated", "detail_code": "auth.refresh_reused",
			})
			return
		}
		t.Errorf("a call was made with a credential that could not be renewed: %s", r.URL.Path)
	})
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(-time.Minute))

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "",
		"container", "ls")
	if code != exitError {
		t.Fatalf("exit %d, want %d: %s", code, exitError, errOut)
	}
	if !strings.Contains(errOut, "hubctl login") {
		t.Errorf("nothing says what to do next: %q", errOut)
	}
	stored, _ := LoadProfile(profile)
	if !stored.Session.IsEmpty() {
		t.Error("a session nothing can use survived in the profile")
	}
}

// Which credential a command would use is a question `auth status` answers, and a session is one
// of the answers.
func TestStatusNamesTheSessionAsTheSource(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, "https://hub.example.com", time.Now().Add(10*time.Minute))

	code, out, errOut := invoke(t, map[string]string{envProfile: profile}, "", "--json", "auth", "status")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, `"token_source": "`+sourceSession+`"`) {
		t.Errorf("status %q does not name the session", out)
	}
}

func saveSession(t *testing.T, path, baseURL string, accessExpiry time.Time) {
	t.Helper()
	if err := SaveProfile(path, Profile{
		BaseURL: baseURL,
		Session: Session{
			ID:               sessionID,
			AccessToken:      secret.New(firstAccess),
			AccessExpiresAt:  accessExpiry,
			RefreshToken:     secret.New(firstRefresh),
			RefreshExpiresAt: accessExpiry.Add(720 * time.Hour),
		},
	}); err != nil {
		t.Fatalf("saving the profile: %v", err)
	}
}
