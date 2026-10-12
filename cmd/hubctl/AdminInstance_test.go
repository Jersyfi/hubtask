// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The second of ADR-0070 §5's three doors, at the terminal.
//
// What is worth asserting is the two things a terminal does that a screen does not: it prints the
// whole level in one go, and it changes one key without making somebody retype the other
// twenty-one. Both would be easy to get subtly wrong — a table that showed only what was set, or a
// `PUT` that cleared everything it did not mention.

const wholeLevel = `{
  "sign_in": {
    "min_length": {"set": true, "value": 16, "locked": true, "kind": "COUNT"},
    "min_digits": {"set": false, "locked": false, "kind": "COUNT"},
    "methods": {"set": false, "locked": false, "kind": "CHOICES"},
    "breach_check": {"set": false, "locked": false, "kind": "FLAG"}
  },
  "legal": {"imprint_url": {"set": false, "locked": false, "kind": "LINK"}},
  "localisation": {"locale": {"set": true, "value": "de", "locked": false}},
  "quotas": {"export_jobs": {"set": true, "value": 9, "locked": true}},
  "ai": {"sources": {"set": true, "value": "EITHER", "locked": false}, "min_jurisdiction": {"set": false, "locked": false}, "person_opt_out": {"set": false, "locked": false, "kind": "CHOICE"}},
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
		"legal.imprint_url", "localisation.locale", "quotas.export_jobs",
		"ai.sources", "ai.min_jurisdiction",
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
	// And so do the other areas.
	for _, area := range []string{"legal", "localisation", "quotas", "ai"} {
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

// The word is typed by the kind the level declares for the key, not guessed from the word: a link
// with a comma in it stays one address, and a word that is not of the declared kind travels as the
// word, for the server to refuse as it refuses the same word from the file (UC-INS-09 check 6).
func TestAValueIsTypedByItsDeclaredKind(t *testing.T) {
	kind := func(k openapi.InstanceSettingKind) *openapi.InstanceSettingKind { return &k }
	cases := []struct {
		kind *openapi.InstanceSettingKind
		raw  string
		want any
	}{
		{kind(openapi.InstanceSettingKindCOUNT), "14", 14},
		{kind(openapi.InstanceSettingKindCOUNT), "fourteen", "fourteen"},
		{kind(openapi.InstanceSettingKindCOUNT), "3.5", "3.5"},
		{kind(openapi.InstanceSettingKindFLAG), "true", true},
		{kind(openapi.InstanceSettingKindFLAG), "false", false},
		{kind(openapi.InstanceSettingKindFLAG), "yes", "yes"},
		{kind(openapi.InstanceSettingKindCHOICE), "ADMINS", "ADMINS"},
		{kind(openapi.InstanceSettingKindCHOICE), "7", "7"},
		{kind(openapi.InstanceSettingKindLINK), "https://example.org/a,b", "https://example.org/a,b"},
		{kind(openapi.InstanceSettingKindLOCALE), "true", "true"},
		{kind(openapi.InstanceSettingKindCHOICES), "PASSWORD,OIDC", []any{"PASSWORD", "OIDC"}},
		{kind(openapi.InstanceSettingKindCHOICES), "PASSWORD", []any{"PASSWORD"}},
	}
	for _, c := range cases {
		got := settingValue(c.kind, c.raw)
		if list, isList := c.want.([]any); isList {
			asList, isList := got.([]any)
			if !isList || len(asList) != len(list) {
				t.Errorf("%s %q read as %v", *c.kind, c.raw, got)
				continue
			}
			for i := range list {
				if asList[i] != list[i] {
					t.Errorf("%s %q read as %v", *c.kind, c.raw, got)
				}
			}
			continue
		}
		if got != c.want {
			t.Errorf("%s %q read as %v (%T), want %v", *c.kind, c.raw, got, got, c.want)
		}
	}
}

// A key the read did not answer is a typo, refused before anything is sent: the server would skip
// it as a newer client's key and save nothing, which a person at a terminal would read as done.
func TestAnUnknownKeyIsRefusedBeforeAnythingIsSent(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, wholeLevel)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "settings", "set", "sign_in.min_size", "12")
	if code == exitOK {
		t.Fatal("an unknown key was accepted")
	}
	if stub.request.Method == http.MethodPut {
		t.Error("an unknown key was sent")
	}
	if !strings.Contains(errOut, "admin settings show") {
		t.Errorf("the refusal does not say where the keys are: %q", errOut)
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

// The AI area is set like any other: `admin settings set --locked ai.sources OFFERED` is how a
// provider holds its workspaces to the model it offers.
func TestTheAiAreaIsSetFromTheTerminal(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, wholeLevel)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "settings", "set", "--locked", "ai.sources", "OFFERED")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	ai, _ := sent["ai"].(map[string]any)
	sources, _ := ai["sources"].(map[string]any)
	if sources["value"] != "OFFERED" || sources["locked"] != true {
		t.Errorf("ai.sources was sent as %v", sources)
	}
}

// `ai.person_opt_out` is set the same way: an installation holds every workspace at "each person
// may keep their content out" with `--locked` (UC-PRV-05 check 11).
func TestThePersonOptOutIsLockedFromTheTerminal(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, wholeLevel)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "settings", "set", "--locked", "ai.person_opt_out", "OFFERED")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	ai, _ := sent["ai"].(map[string]any)
	position, _ := ai["person_opt_out"].(map[string]any)
	if position["value"] != "OFFERED" || position["locked"] != true {
		t.Errorf("ai.person_opt_out was sent as %v", position)
	}
	if sources, _ := ai["sources"].(map[string]any); sources["value"] != "EITHER" {
		t.Errorf("the rest of the area was not sent back: %v", ai)
	}
}
