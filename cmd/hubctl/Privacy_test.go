// SPDX-License-Identifier: Apache-2.0
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

// extendedCase is a case after its one extension: the deadline in force and the original beside it.
const extendedCase = `{"id":"` + caseID + `","kind":"ACCESS","status":"RECEIVED","scope":"TENANT",
  "subject_email":"anna@example.org","received_at":"2026-08-27T09:00:00Z",
  "due_at":"2026-11-27T22:59:59Z","original_due_at":"2026-09-26T09:00:00Z",
  "extension_reason":"COMPLEXITY","informed_on":"2026-08-28"}`

// UC-PRV-01 check 9 through hubctl: the action, its three fields as the contract names them, and
// both deadlines in the answer.
func TestExtendingACaseSendsTheDayTheReasonAndTheInformedDay(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, extendedCase)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"dsr", "extend", caseID, "--until", "2026-11-27", "--reason", "COMPLEXITY", "--informed", "2026-08-28")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPost {
		t.Errorf("method %s", stub.request.Method)
	}
	if want := APIPath + privacyRequestsPath + "/" + caseID + ":extend"; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["due_on"] != "2026-11-27" || sent["reason"] != "COMPLEXITY" || sent["informed_on"] != "2026-08-28" {
		t.Errorf("the extension lost something: %v", sent)
	}
	if !strings.Contains(strings.ToLower(out), "original due") || !strings.Contains(out, "2026-09-26") ||
		!strings.Contains(out, "2026-11-2") {
		t.Errorf("both deadlines are not shown: %q", out)
	}
}

// UC-PRV-01 check 10: the register lists the original deadline beside the one in force, and a dash
// where there was no extension.
func TestTheListShowsBothDeadlines(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"data":[`+extendedCase+`,`+oneCase+`],"page":{"has_more":false,"next_cursor":null}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "dsr", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.Contains(strings.ToLower(lines[0]), "original due") {
		t.Fatalf("the table is %q", out)
	}
	if !strings.Contains(lines[1], "2026-09-26") {
		t.Errorf("the extended case does not show its original deadline: %q", lines[1])
	}
	if strings.Count(lines[2], " - ") < 1 {
		t.Errorf("a case never extended does not show a dash for the original: %q", lines[2])
	}
}

func TestAnExtensionWithoutItsThreeFactsIsRefusedHere(t *testing.T) {
	var called bool
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(extendedCase))
	})

	for _, args := range [][]string{
		{"dsr", "extend", caseID, "--until", "2026-11-27", "--reason", "COMPLEXITY"},
		{"dsr", "extend", caseID, "--until", "27.11.2026", "--reason", "COMPLEXITY", "--informed", "2026-08-28"},
	} {
		code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", args...)
		if code != exitUsage {
			t.Errorf("%v: exit %d, want %d: %s", args, code, exitUsage, errOut)
		}
	}
	if called {
		t.Error("an incomplete extension was sent anyway")
	}
}
