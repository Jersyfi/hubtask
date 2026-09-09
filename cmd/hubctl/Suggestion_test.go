// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
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

// Asking is asking: the API answers 202 and the suggestion appears later, so the client says so
// rather than blocking. A client that waited would be inventing a synchronous shape the API
// deliberately does not have.
func TestAskingDispatchesOnTheTargetAndDoesNotWait(t *testing.T) {
	for targetType, want := range map[string]string{
		"JUMBLE_ENTRY": ":suggest",
		"ITEM":         ":suggest-fields",
	} {
		t.Run(targetType, func(t *testing.T) {
			stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			})

			code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
				"suggestion", "ask", "--target", itemID, "--target-type", targetType)
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			if !strings.HasSuffix(stub.request.URL.Path, itemID+want) {
				t.Errorf("the call went to %q, want one ending %q", stub.request.URL.Path, want)
			}
			// And it says where the answer will turn up, because nothing came back with it.
			if !strings.Contains(errOut, "suggestion ls") {
				t.Errorf("the client does not say where to look: %q", errOut)
			}
		})
	}
}

// A target type nobody serves is a usage error rather than a call to a path that does not exist.
func TestAnUnknownTargetTypeIsAUsageError(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made for a target type nobody serves")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"suggestion", "ask", "--target", itemID, "--target-type", "COLLECTION")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "JUMBLE_ENTRY") {
		t.Errorf("the message %q does not say what is allowed", errOut)
	}
}

// Accepting carries what the person changed or added. It is not a convenience: a proposal about a
// jumble entry is accepted by converting it, and a model cannot name a destination collection - so
// this is how the one thing only a person knows reaches the use case that performs the change.
func TestAcceptingCarriesTheOverrides(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneSuggestion)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"suggestion", "accept", suggestionID, "--override", "collection_id="+collectionID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent struct {
		Overrides map[string]any `json:"overrides"`
	}
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the request is not JSON: %v", err)
	}
	if sent.Overrides["collection_id"] != collectionID {
		t.Errorf("the override did not travel: %v", sent.Overrides)
	}
}

// An acceptance with nothing to add sends no overrides rather than an empty object, because the two
// are different requests to a use case that merges what it is given.
func TestAcceptingWithoutOverridesSendsNone(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneSuggestion)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "suggestion", "accept", suggestionID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Contains(stub.body, "overrides") {
		t.Errorf("an empty acceptance sent %q", stub.body)
	}
}

// An override that is not name=value is a usage error rather than a request the server has to
// refuse.
func TestAnOverrideIsNameEqualsValue(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a malformed override reached the installation")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"suggestion", "accept", suggestionID, "--override", "collection_id")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "name=value") {
		t.Errorf("the message %q does not say the shape", errOut)
	}
}
