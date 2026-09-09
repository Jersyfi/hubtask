// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

// What AI proposed, as a person meets it (J-05, J-16). Two decisions are asserted here: the listing
// prints the provenance rather than the payload, and accepting is a command of its own because it
// is a write with the caller's own rights.

const suggestionID = "01936f2a-7c1e-7000-8000-000000000f11"

const oneSuggestion = `{"id":"` + suggestionID + `","kind":"FIELDS","status":"PROPOSED",
  "target_type":"ITEM","target_id":"` + itemID + `","source":"AI",
  "model":"gpt-4o-mini","prompt_id":"suggest-fields","prompt_version":"v1",
  "payload":{"title":"the model's own words about somebody's content"},
  "produced_at":"2026-09-09T08:00:00Z","created_at":"2026-09-09T08:00:01Z","version":1}`

// The provenance is what makes a suggestion traceable a year later: the model, the prompt and its
// version. The payload is model output about somebody's content and belongs in --json.
func TestTheSuggestionListingShowsProvenanceAndNotThePayload(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"items":[`+oneSuggestion+`],"has_more":false,"next_cursor":null}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"suggestion", "ls", "--target", itemID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.URL.Query().Get("target_id"); got != itemID {
		t.Errorf("target_id %q", got)
	}
	if got := stub.request.URL.Query().Get("target_type"); got != "ITEM" {
		t.Errorf("target_type %q, want the default", got)
	}
	for _, want := range []string{"gpt-4o-mini", "suggest-fields", "v1", "PROPOSED"} {
		if !strings.Contains(out, want) {
			t.Errorf("the table does not say %q: %q", want, out)
		}
	}
	if strings.Contains(out, "the model's own words") {
		t.Errorf("the payload reached the table: %q", out)
	}
}

func TestTheWholeSuggestionIsAvailableAsJSON(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"items":[`+oneSuggestion+`],"has_more":false,"next_cursor":null}`)

	code, out, _ := invokeAgainst(t, stub, signedIn(stub), "",
		"--json", "suggestion", "ls", "--target", itemID)
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "the model's own words") {
		t.Errorf("the payload is not the API's own: %q", out)
	}
}

// Accepting and dismissing are two verbs on one record, and both reach the action route the
// contract declares - never a general update, because deciding a suggestion is its own act.
func TestDecidingASuggestionCallsItsOwnAction(t *testing.T) {
	for verb, want := range map[string]string{"accept": ":accept", "dismiss": ":dismiss"} {
		t.Run(verb, func(t *testing.T) {
			stub := serveJSON(t, http.StatusOK, oneSuggestion)

			code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
				"suggestion", verb, suggestionID)
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			if !strings.HasSuffix(stub.request.URL.Path, suggestionID+want) {
				t.Errorf("the call went to %q", stub.request.URL.Path)
			}
			if stub.request.Method != http.MethodPost {
				t.Errorf("the call was a %s", stub.request.Method)
			}
		})
	}
}

func TestListingSuggestionsNeedsATarget(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made without the required filter")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "suggestion", "ls")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--target") {
		t.Errorf("the message %q does not name what is missing", errOut)
	}
}
