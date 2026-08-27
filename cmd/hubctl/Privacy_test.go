// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const caseID = "01936f2a-7c1e-7000-8000-0000000000f1"

const oneCase = `{"id":"` + caseID + `","kind":"ERASURE","status":"RECEIVED",
  "scope":"TENANT","subject_account_id":"` + itemID + `","subject_email":null,
  "received_at":"2026-08-27T09:00:00Z","due_at":"2026-09-26T09:00:00Z",
  "completed_at":null,"result_archive":null}`

func TestRaisingACaseSendsTheKindAndWhoItIsAbout(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneCase)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"dsr", "create", "--kind", "ERASURE", "--subject", itemID, "--notes", "asked by email")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+privacyRequestsPath {
		t.Errorf("path %q", stub.request.URL.Path)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["kind"] != "ERASURE" || sent["subject_account_id"] != itemID {
		t.Errorf("the case lost something: %v", sent)
	}
	// The deadline is the point of the whole resource, so the table has to carry it.
	if !strings.Contains(out, "2026-09-26") {
		t.Errorf("the deadline is not shown: %q", out)
	}
}

// A case is about somebody. Without an account or an address there is nothing to answer, and
// saying so here costs a round trip less than letting the server say it.
func TestACaseAboutNobodyIsRefused(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneCase)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "dsr", "create", "--kind", "ACCESS")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "--subject") {
		t.Errorf("the complaint does not say what is missing: %q", errOut)
	}
}

func TestListingCasesPassesTheDeadlineWindow(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"data":[`+oneCase+`],"page":{"has_more":true,"next_cursor":"c9"}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"dsr", "ls", "--due-within", "7", "--status", "RECEIVED", "--include-closed")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	query := stub.request.URL.Query()
	if query.Get("due_within_days") != "7" || query.Get("status") != "RECEIVED" ||
		query.Get("include_closed") != "true" {
		t.Errorf("query %v", query)
	}
	if !strings.Contains(out, "ERASURE") {
		t.Errorf("output %q", out)
	}
	if !strings.Contains(errOut, "--cursor c9") {
		t.Errorf("the next page is not offered: %q", errOut)
	}
}

// Starting an erasure is the transition that does the work, and the mode travels with it: the
// controller's choice, not this system's.
func TestStartingAnErasureSendsTheModeAndTheTransition(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneCase)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"dsr", "start", caseID, "--mode", "FULL_DELETE")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPatch {
		t.Errorf("method %s", stub.request.Method)
	}
	if want := APIPath + privacyRequestsPath + "/" + caseID; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["status"] != "IN_PROGRESS" || sent["erasure_mode"] != "FULL_DELETE" {
		t.Errorf("the transition lost something: %v", sent)
	}
}

func TestCompletingACaseMovesItToCompleted(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneCase)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"dsr", "complete", caseID, "--notes", "the export was handed over")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["status"] != "COMPLETED" || sent["notes"] != "the export was handed over" {
		t.Errorf("body %v", sent)
	}
}

func TestRefusingACaseWithoutAReasonIsRefused(t *testing.T) {
	var called bool
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(oneCase))
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "dsr", "reject", caseID)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if called {
		t.Error("a refusal with no reason was sent anyway")
	}
	if !strings.Contains(errOut, "silence is not") {
		t.Errorf("the complaint does not say why: %q", errOut)
	}
}
