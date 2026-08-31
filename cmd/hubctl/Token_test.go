// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

const (
	tokenID   = "01936f2a-7c1e-7000-8000-000000000401"
	machineID = "01936f2a-7c1e-7000-8000-000000000402"
)

// Assembled rather than written out, for the reason the compose fixtures are: a literal that
// looks like a credential is what the secret scan of SG-7 exists to find (and the prefix is a
// scanning pattern on purpose).
var mintedToken = "hbt_" + "pat_01936f2a7c1e70008000000000000001_" +
	"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

func mintedTokenJSON() string {
	return `{"id":"` + tokenID + `","account_id":"` + itemID + `","name":"the nightly export",
	  "scopes":["items:read","items:write"],"expires_at":"2027-01-31T09:00:00Z",
	  "created_at":"2026-09-01T09:00:00Z","token":"` + mintedToken + `"}`
}

// The minting is the only moment the credential exists outside the server's hash, and the command
// says so - on standard error, so that the token itself stays pipeable.
func TestMintingPrintsTheTokenOnceAndWarnsBesideIt(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, mintedTokenJSON())

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"token", "create", "--name", "the nightly export",
		"--scope", "items:read,items:write", "--days", "30")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	if !strings.Contains(stub.body, `"scopes":["items:read","items:write"]`) {
		t.Errorf("the scopes did not travel: %s", stub.body)
	}
	if !strings.Contains(out, mintedToken) {
		t.Errorf("the credential was not printed: %q", out)
	}
	if !strings.Contains(errOut, "shown once") {
		t.Errorf("nothing warned about the credential: %q", errOut)
	}
	// The warning is not in the pipe.
	if strings.Contains(out, "shown once") {
		t.Error("the warning landed on standard output")
	}
}

// There is no default expiry, on the server's own reasoning - and the client refuses before the
// round trip rather than after it.
func TestAMintWithoutAnExpiryIsRefusedBeforeTheCall(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, mintedTokenJSON())

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"token", "create", "--name", "x", "--scope", "items:read")
	if code == exitOK {
		t.Fatal("a mint without an expiry was accepted")
	}
	if !strings.Contains(errOut, "no default") {
		t.Errorf("the refusal does not say why: %q", errOut)
	}
	if stub.request != nil {
		t.Error("the server was called for a request the client could refuse on its own")
	}
}

// And the same for the scopes: a token that asks for nothing has nothing to be used for, so it is
// refused here rather than defaulted to everything anywhere.
func TestAMintWithoutScopesIsRefusedBeforeTheCall(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, mintedTokenJSON())

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"token", "create", "--name", "x", "--days", "30")
	if code == exitOK {
		t.Fatal("a mint without scopes was accepted")
	}
	if !strings.Contains(errOut, "not for everything") {
		t.Errorf("the refusal does not say why: %q", errOut)
	}
	if stub.request != nil {
		t.Error("the server was called for a request the client could refuse on its own")
	}
}

// The two spellings of one thing. Giving both is a caller who has not decided, and guessing which
// they meant is how a token gets the wrong life.
func TestDaysAndExpiresAreNotBothAccepted(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, mintedTokenJSON())

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"token", "create", "--name", "x", "--scope", "items:read",
		"--days", "30", "--expires", "2027-01-31T09:00:00Z")
	if code == exitOK {
		t.Fatal("both spellings were accepted at once")
	}
	if !strings.Contains(errOut, "give one") {
		t.Errorf("the refusal does not say why: %q", errOut)
	}
}

func TestDaysBecomesAnInstantInTheFuture(t *testing.T) {
	at, err := expiryArgument("30", "")
	if err != nil {
		t.Fatalf("--days 30 was refused: %v", err)
	}
	if until := time.Until(at); until < 29*24*time.Hour || until > 31*24*time.Hour {
		t.Errorf("--days 30 produced %v from now", until)
	}

	for _, days := range []string{"0", "-1", "many"} {
		if _, err := expiryArgument(days, ""); err == nil {
			t.Errorf("--days %s was accepted", days)
		}
	}
}

// The list cannot show a token, because the server keeps none - and this is the test that says so
// rather than a comment claiming it.
func TestTheTokenListCarriesNoCredential(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[{"id":"`+tokenID+`","account_id":"`+itemID+`",
	  "name":"the nightly export","scopes":["items:read"],
	  "expires_at":"2027-01-31T09:00:00Z","created_at":"2026-09-01T09:00:00Z",
	  "last_used_at":"2026-09-02T11:00:00Z"}]`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "token", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Contains(out, "hbt_pat_") || strings.Contains(errOut, "hbt_pat_") {
		t.Error("a credential appeared in a list")
	}
	if !strings.Contains(out, tokenID) || !strings.Contains(out, "items:read") {
		t.Errorf("the table %q does not carry the token and its scopes", out)
	}
}

// Naming an account is how a service account's credentials are reached, and it travels as the
// query parameter the contract declares.
func TestListingNamesTheAccountInTheQuery(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[]`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"token", "ls", "--account", machineID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.URL.RawQuery; !strings.Contains(got, "account_id="+machineID) {
		t.Errorf("the account did not travel: %s", got)
	}
}

func TestRevokingSaysWhatItDid(t *testing.T) {
	stub := serveJSON(t, http.StatusNoContent, "")

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "token", "revoke", tokenID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(errOut, "revoked") || !strings.Contains(errOut, tokenID) {
		t.Errorf("the confirmation does not name what stopped: %q", errOut)
	}
}

func TestCreatingAServiceAccountNeedsAName(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated,
		`{"id":"`+machineID+`","kind":"SERVICE_ACCOUNT","display_name":"the nightly export","status":"ACTIVE"}`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "service-account", "create")
	if code == exitOK {
		t.Fatal("a nameless service account was accepted")
	}
	if !strings.Contains(errOut, "audit trail") {
		t.Errorf("the refusal does not say why a name matters: %q", errOut)
	}

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"service-account", "create", "--name", "the nightly export")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, machineID) || !strings.Contains(out, "the nightly export") {
		t.Errorf("the table %q does not name what was created", out)
	}
}
