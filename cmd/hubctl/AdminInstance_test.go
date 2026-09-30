// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The second of ADR-0070 §5's three doors, at the terminal.
//
// What is worth asserting is the two things a terminal does that a screen does not: it prints the
// whole level in one go, and it changes one key without making somebody retype the other
// twenty-one. Both would be easy to get subtly wrong — a table that showed only what was set, or a
// `PUT` that cleared everything it did not mention.

const wholeLevel = `{
  "sign_in": {
    "min_length": {"set": true, "value": 16, "locked": true},
    "min_digits": {"set": false, "locked": false}
  },
  "legal": {"imprint": {"set": false, "locked": false}},
  "localisation": {"locale": {"set": true, "value": "de", "locked": false}},
  "quotas": {"export_jobs": {"set": true, "value": 9, "locked": true}},
  "source": "database",
  "is_enforced_from_file": false
}`

// The table prints every key of every area, decided or not — the same whole level the dashboard
// draws. A terminal that showed only what was set could not tell somebody which switches exist.
func TestTheSettingsTableShowsTheWholeLevelDecidedOrNot(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, wholeLevel)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "settings", "show")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+adminSettingsPath {
		t.Errorf("the read called %s", stub.request.URL.Path)
	}
	for _, expected := range []string{
		"sign_in.min_length", "sign_in.min_digits",
		"legal.imprint", "localisation.locale", "quotas.export_jobs",
	} {
		if !strings.Contains(out, expected) {
			t.Errorf("%s is missing from the table: %q", expected, out)
		}
	}
	if !strings.Contains(out, "LOCKED") || !strings.Contains(out, "open") {
		t.Errorf("the table does not say who may change what: %q", out)
	}
	// An undecided switch is a dash, not a blank and not a zero: a zero would read as a decision.
	if !strings.Contains(out, "-") {
		t.Errorf("an undecided switch has no mark: %q", out)
	}
}

// `set` changes one key and sends the rest back untouched, because `PUT` replaces the level. A
// terminal that made somebody retype twenty-two switches to change one is a terminal nobody uses.
func TestSettingOneKeySendsTheRestBack(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(wholeLevel))
			return
		}
		_, _ = w.Write([]byte(wholeLevel))
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "settings", "set", "sign_in.min_digits", "2")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPut {
		t.Fatalf("the change was a %s", stub.request.Method)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	signIn, _ := sent["sign_in"].(map[string]any)
	changed, _ := signIn["min_digits"].(map[string]any)
	if changed["value"] != float64(2) {
		t.Errorf("the changed switch reads %v", changed)
	}
	// The one that was not named survives, with its lock.
	kept, _ := signIn["min_length"].(map[string]any)
	if kept["value"] != float64(16) || kept["locked"] != true {
		t.Errorf("an untouched switch changed: %v", kept)
	}
	// And so do the other three areas.
	for _, area := range []string{"legal", "localisation", "quotas"} {
		if _, held := sent[area]; !held {
			t.Errorf("%s was dropped by a change to sign_in", area)
		}
	}
}

// `clear` is how the installation stops deciding a switch: the key leaves the body, and `PUT`
// replacing the level is what makes the absence mean something.
func TestClearingAKeyRemovesItFromTheBody(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, wholeLevel)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "settings", "clear", "sign_in.min_length")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	signIn, _ := sent["sign_in"].(map[string]any)
	if _, held := signIn["min_length"]; held {
		t.Error("a cleared switch was sent back")
	}
	if _, held := signIn["min_digits"]; !held {
		t.Error("clearing one switch dropped another")
	}
}

// A key that is not `<area>.<name>` is a usage error rather than a request: the shape is what
// `show` prints, so somebody can copy a row out of the table and into the command.
func TestAKeyWithoutAnAreaIsAUsageError(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, wholeLevel)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "settings", "set", "min_length", "12")
	if code == exitOK {
		t.Fatal("a bare name was accepted")
	}
	if !strings.Contains(errOut, "area") {
		t.Errorf("the refusal does not say what is missing: %q", errOut)
	}
}

// The value keeps the kind a terminal cannot type: a flag, a whole number, a list. Guessed in the
// order that cannot surprise, because `12` is never meant as the word "12".
func TestAValueKeepsItsKind(t *testing.T) {
	cases := map[string]any{
		"true":          true,
		"false":         false,
		"14":            14,
		"ADMINS":        "ADMINS",
		"PASSWORD,OIDC": []any{"PASSWORD", "OIDC"},
	}
	for raw, want := range cases {
		got := settingValue(raw)
		if list, isList := want.([]any); isList {
			asList, isList := got.([]any)
			if !isList || len(asList) != len(list) {
				t.Errorf("%q read as %v", raw, got)
				continue
			}
			for i := range list {
				if asList[i] != list[i] {
					t.Errorf("%q read as %v", raw, got)
				}
			}
			continue
		}
		if got != want {
			t.Errorf("%q read as %v (%T), want %v", raw, got, got, want)
		}
	}
}

// writeThenList is the shape both `add` commands have: they write, then re-read so the caller sees
// the register or the listing afterwards. The stub keeps the *write's* body, because the read that
// follows would otherwise overwrite it with nothing.
func writeThenList(t *testing.T, listing string, written *string) *installation {
	t.Helper()
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(listing))
			return
		}
		// `serve` has already drained the body into the stub, so this is where the write's own
		// payload is taken — the read that follows would otherwise replace it with nothing.
		*written = stub.body
		_, _ = w.Write([]byte(`{}`))
	})
	return stub
}

// An operator is named by workspace and address, because an identifier is not something a terminal
// can look up either: no command may list accounts across workspaces.
func TestAddingAnOperatorSendsTheWorkspaceAndTheAddress(t *testing.T) {
	var written string
	stub := writeThenList(t, `[]`, &written)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "operator", "add", "--workspace", "acme", "--email", "ada@acme.example")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(written), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["workspace"] != "acme" || sent["email"] != "ada@acme.example" {
		t.Errorf("the body reads %v", sent)
	}
	if _, held := sent["account_id"]; held {
		t.Error("an identifier nobody supplied was sent")
	}
}

// And half a pair names nobody, which is a usage error rather than a request the server refuses.
func TestHalfAPairIsAUsageError(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[]`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "operator", "add", "--workspace", "acme")
	if code == exitOK {
		t.Fatal("half a pair was accepted")
	}
	if !strings.Contains(errOut, "--email") {
		t.Errorf("the refusal does not name what is missing: %q", errOut)
	}
}

// A provider's directories are what `DOMAINS` reads where the preset names organisations, and the
// domains are the other list. Both travel, and neither is invented where it was not given.
func TestAddingAProviderCarriesTheDirectoriesItWasGiven(t *testing.T) {
	var written string
	stub := writeThenList(t, `[]`, &written)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "provider", "add",
		"--issuer", "https://login.microsoftonline.com/common/v2.0",
		"--client-id", "hubtask", "--client-secret", "s3cr3t",
		"--admits", "DOMAINS", "--directory", "72f988bf-86f1-41af-91ab-2d7cd011db47, 9188040d-6c67-4c5b-b112-36a304b66dad")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(written), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	directories, _ := sent["allowed_directories"].([]any)
	if len(directories) != 2 || directories[0] != "72f988bf-86f1-41af-91ab-2d7cd011db47" {
		t.Errorf("the directories read %v", directories)
	}
	if _, held := sent["allowed_email_domains"]; held {
		t.Error("a domain list nobody gave was sent")
	}
}
