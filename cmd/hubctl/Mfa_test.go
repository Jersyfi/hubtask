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

const totpSecret = "JBSWY3DPEHPK3PXP"

var enrolmentJSON = `{"secret":"` + totpSecret + `",
  "otpauth_uri":"otpauth://totp/Acme:eva@acme.example?secret=` + totpSecret + `&issuer=Acme",
  "recovery_codes":["AAAA-BBBB-CCCC-DDDD","EEEE-FFFF-GGGG-HHHH"]}`

// The enrolment's single showing, with the discipline `calendar mint` set: what a script has to
// read goes to standard output, and the warning that makes "once" true goes beside it.
func TestEnrollingShowsTheSecretOnceAndWarnsBesideIt(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, enrolmentJSON)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "mfa", "enroll")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+totpEnrollPath {
		t.Errorf("the enrolment called %s", stub.request.URL.Path)
	}
	for _, wanted := range []string{totpSecret, "otpauth://", "AAAA-BBBB-CCCC-DDDD"} {
		if !strings.Contains(out, wanted) {
			t.Errorf("%q is not where a person or a script can read it: %q", wanted, out)
		}
	}
	if !strings.Contains(errOut, "shown once") {
		t.Errorf("nothing warns about the single showing: %q", errOut)
	}
	if strings.Contains(out, "shown once") {
		t.Error("the warning landed on standard output")
	}
	// Nothing is armed by enrolling, and the client says so rather than leaving somebody to find
	// out at their next sign-in.
	if !strings.Contains(errOut, "mfa confirm") {
		t.Errorf("nothing says what arms it: %q", errOut)
	}
}

// The enforcement route: a person routed into enrolment instead of into a session has no bearer
// credential, and presenting a stale one beside the pending credential would be verified - and
// refused.
func TestTheEnforcementFlowCarriesThePendingCredentialAndNoOther(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, enrolmentJSON)
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "",
		"mfa", "enroll", "--pending", pendingCredit)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.Header.Get("Authorization"); got != "" {
		t.Errorf("a second credential travelled beside the pending one: %q", got)
	}
	if !strings.Contains(stub.body, pendingCredit) {
		t.Errorf("the pending credential did not travel: %s", stub.body)
	}
}

// A confirmation under the enforcement flow is where the sign-in finally lands: by then the
// person has proved both factors, so the answer carries the pair.
func TestConfirmingUnderEnforcementHoldsTheSessionItAnswers(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"armed":true,"tokens":`+sessionTokensJSON(firstAccess, firstRefresh, time.Now().Add(15*time.Minute))+`}`)
	profile := filepath.Join(t.TempDir(), "profile.json")
	if err := SaveProfile(profile, Profile{BaseURL: stub.server.URL, Tenant: tenantID}); err != nil {
		t.Fatalf("saving: %v", err)
	}

	code, _, errOut := invokeAgainst(t, stub,
		map[string]string{envProfile: profile, envTotp: "123456"}, "",
		"mfa", "confirm", "--pending", pendingCredit)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"code":"123456"`) ||
		!strings.Contains(stub.body, pendingCredit) {
		t.Errorf("the confirmation did not carry both halves: %s", stub.body)
	}
	stored, _ := LoadProfile(profile)
	if stored.Session.AccessToken.Reveal() != firstAccess {
		t.Error("the pair the confirmation answered was not held")
	}
	// The workspace `hubctl login` remembered before it handed over survives the round trip.
	if stored.Tenant != tenantID {
		t.Errorf("the workspace was lost between the two commands: %q", stored.Tenant)
	}
}

// A signed-in caller confirms with their bearer credential and receives no pair - they already
// hold one.
func TestConfirmingWhileSignedInArmsAndChangesNothingElse(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"armed":true}`)
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, _, errOut := invokeAgainst(t, stub,
		map[string]string{envProfile: profile, envTotp: "123456"}, "", "mfa", "confirm")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.Header.Get("Authorization"); got != "Bearer "+firstAccess {
		t.Errorf("Authorization %q, want the held session", got)
	}
	stored, _ := LoadProfile(profile)
	if stored.Session.AccessToken.Reveal() != firstAccess {
		t.Error("an arming that answered no pair replaced the session anyway")
	}
}

// The password afresh: a live session is deliberately not enough to remove the factor.
func TestDisablingAsksForThePasswordAndSendsNothingElse(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"mfa", "disable")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+mfaDisablePath {
		t.Errorf("the removal called %s", stub.request.URL.Path)
	}
	if !strings.Contains(stub.body, `"password":"hunter2"`) {
		t.Errorf("the password did not travel: %s", stub.body)
	}
	if !strings.Contains(errOut, "recovery codes") {
		t.Errorf("nothing says the codes went too: %q", errOut)
	}
}

// Under tenant enforcement an administrator cannot remove it at all, and the refusal names the
// switch - in the catalogue's sentence rather than as a problem document.
func TestARemovalTheTenantForbidsReadsAsASentence(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		problemJSON(w, http.StatusForbidden, map[string]any{
			"status": 403, "code": "forbidden", "detail_code": "auth.mfa_required_by_tenant",
		})
	})
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"mfa", "disable")
	if code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	if strings.Contains(errOut, "{") || strings.Contains(errOut, "detail_code") {
		t.Errorf("the refusal was printed as a document: %q", errOut)
	}
}
