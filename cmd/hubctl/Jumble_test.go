// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The inbox as a person meets it, and the two decisions this client makes about it: a
// listing shows the subject and not the body, and a dismissal says what it is - a state rather
// than a deletion.

const jumbleEntryID = "01936f2a-7c1e-7000-8000-000000000e11"

const oneJumbleEntry = `{"id":"` + jumbleEntryID + `","channel":"EMAIL","sender":"orders@example.org",
  "raw_subject":"Order #42","raw_body":"the whole message, which a table is not for",
  "attachments":[],"status":"NEW","received_at":"2026-08-20T08:00:00Z"}`

// The body is not in the table. An entry's raw content is the least trusted text in the system and
// a terminal is where an escape sequence would land; what a listing is for is deciding which entry
// to look at.
func TestTheListingShowsTheSubjectAndNotTheBody(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"data":[`+oneJumbleEntry+`],"page":{"has_more":false,"next_cursor":null}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"jumble", "ls", "--status", "NEW")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.URL.Query().Get("status"); got != "NEW" {
		t.Errorf("status %q", got)
	}
	if !strings.Contains(out, "Order #42") || !strings.Contains(out, "orders@example.org") {
		t.Errorf("the table is %q", out)
	}
	if strings.Contains(out, "a table is not for") {
		t.Errorf("the body reached the table: %q", out)
	}
}

// The whole of an entry is `--json`'s, for something that can handle it.
func TestTheWholeEntryIsAvailableAsJSON(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"data":[`+oneJumbleEntry+`],"page":{"has_more":false,"next_cursor":null}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "--json", "jumble", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "a table is not for") {
		t.Errorf("the payload is not the API's own: %q", out)
	}
	if errOut != "" {
		t.Errorf("standard error carried %q", errOut)
	}
}

// One entry by its identifier, in the listing's columns and with the body left out of the table.
func TestOneEntryIsShownByItsIdentifier(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneJumbleEntry)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "jumble", "show", jumbleEntryID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.URL.Path; !strings.HasSuffix(got, "/jumble/entries/"+jumbleEntryID) {
		t.Errorf("asked %q", got)
	}
	if !strings.Contains(out, "Order #42") || strings.Contains(out, "a table is not for") {
		t.Errorf("the table is %q", out)
	}
}

// The destinations are the server's answer, named with their hub.
func TestTheDestinationsAreListedWithTheirHub(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data":[{"id":"01936f2a-7c1e-7000-8000-000000000c01","name":"Orders",
	  "hub_id":"01936f2a-7c1e-7000-8000-000000000c00","hub_name":"Shop","hub_private":false}],
	  "page":{"has_more":false,"next_cursor":null}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "jumble", "destinations", "--size", "20")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.HasSuffix(stub.request.URL.Path, "/jumble/destinations") || stub.request.URL.Query().Get("size") != "20" {
		t.Errorf("asked %s", stub.request.URL)
	}
	if !strings.Contains(out, "Shop") || !strings.Contains(out, "Orders") {
		t.Errorf("the table is %q", out)
	}
}

// Something has to be in an entry, and the client says so rather than sending an empty one.
// An empty submission is the entry's to refuse (jumble.entry_empty); the command passes the refusal on
// and fails.
func TestAnEmptySubmissionIsRefused(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		problemJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"status": 422, "code": "validation_failed", "detail_code": "jumble.entry_empty",
		})
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "jumble", "submit")
	if code != exitError || stub.body != "{}" {
		t.Fatalf("exit %d, sent %q: %s", code, stub.body, errOut)
	}
}

func TestSubmittingSendsTheChannelAndTheText(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneJumbleEntry)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"jumble", "submit", "--raw-subject", "Call the printer", "--channel", "QUICK_CAPTURE")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["raw_subject"] != "Call the printer" || sent["channel"] != "QUICK_CAPTURE" {
		t.Errorf("the submission reached the server as %v", sent)
	}
}

// A conversion needs somewhere to land, and the client refuses without one rather than letting the
// round trip say so.
func TestAConversionWithoutADestinationIsRefusedHere(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneJumbleEntry)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"jumble", "convert", jumbleEntryID)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "--collection") {
		t.Errorf("the complaint does not name what is missing: %q", errOut)
	}
}

// A conversion names the item it produced, which is the provenance pair's other half.
func TestAConversionNamesTheItemItBecame(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"id":"`+jumbleEntryID+`","channel":"EMAIL","attachments":[],"status":"PROCESSED",
		  "received_at":"2026-08-20T08:00:00Z","settled_at":"2026-08-20T09:00:00Z",
		  "target_item_id":"`+itemID+`"}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"jumble", "convert", jumbleEntryID, "--collection-id", collectionID, "--title", "Call back")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["collection_id"] != collectionID || sent["title"] != "Call back" {
		t.Errorf("the conversion reached the server as %v", sent)
	}
	if !strings.Contains(out, itemID) {
		t.Errorf("the answer does not name the item: %q", out)
	}
	if !strings.Contains(out, "PROCESSED") {
		t.Errorf("the table is %q", out)
	}
}

// A dismissal is a state and not a deletion, and somebody dismissing an entry should be told that
// rather than believing they destroyed it.
func TestADismissalSaysTheEntryStays(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"id":"`+jumbleEntryID+`","channel":"API","attachments":[],"status":"DISMISSED",
		  "received_at":"2026-08-20T08:00:00Z","settled_at":"2026-08-20T09:00:00Z"}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "jumble", "dismiss", jumbleEntryID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "DISMISSED") {
		t.Errorf("the answer is %q", out)
	}
}

// The intake address is a credential, and a credential printed to a terminal says so once.
func TestTheIntakeTokenIsShownWithItsWarning(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"token":"`+collectionID+`.a-secret-nobody-else-has","rotated_at":"2026-08-20T08:00:00Z"}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"jumble", "intake", "rotate-token")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "a-secret-nobody-else-has") {
		t.Errorf("the token is not in the answer: %q", out)
	}
	if !strings.Contains(errOut, "shown once") {
		t.Errorf("the warning is %q", errOut)
	}
}
