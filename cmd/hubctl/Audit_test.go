// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const oneEntry = `{"id":"` + holdID + `","seq":42,"occurred_at":"2026-08-27T09:00:00Z",
  "action":"legal_hold.placed","outcome":"SUCCESS","severity":"NOTICE",
  "actor":{"type":"USER","id":"` + itemID + `","label":"Anna Beispiel"},
  "target":{"type":"legal_hold","id":"` + holdID + `","label":null},
  "changes":[],"legal_basis":"dsr.erasure","hash":"abc"}`

func TestQueryingTheTrailPassesEveryFilterItWasGiven(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"items":[`+oneEntry+`],"next_cursor":"c2"}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"audit", "query", "--from", "2026-08-01", "--to", "2026-08-27T12:00:00Z",
		"--action", "legal_hold.", "--actor", itemID, "--target", holdID,
		"--target-type", "legal_hold", "--outcome", "SUCCESS", "--limit", "10")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	query := stub.request.URL.Query()
	// A bare date is midnight UTC: a period whose ends moved with the operator's timezone would
	// answer differently in Berlin and in Dublin for the same command.
	if query.Get("from") != "2026-08-01T00:00:00Z" {
		t.Errorf("from %q", query.Get("from"))
	}
	if query.Get("to") != "2026-08-27T12:00:00Z" {
		t.Errorf("to %q", query.Get("to"))
	}
	for name, want := range map[string]string{
		"action": "legal_hold.", "actor_id": itemID, "target_id": holdID,
		"target_type": "legal_hold", "outcome": "SUCCESS", "limit": "10",
	} {
		if query.Get(name) != want {
			t.Errorf("%s = %q, want %q", name, query.Get(name), want)
		}
	}

	// The label is what makes a trail readable after the account is gone.
	if !strings.Contains(out, "Anna Beispiel") || !strings.Contains(out, "legal_hold.placed") {
		t.Errorf("output %q", out)
	}
	if !strings.Contains(errOut, "--cursor c2") {
		t.Errorf("the next page is not offered: %q", errOut)
	}
}

func TestAMomentThatIsNotOneIsAUsageError(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"items":[],"next_cursor":null}`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"audit", "query", "--from", "last tuesday")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "2026-08-27") {
		t.Errorf("the complaint does not show a spelling that works: %q", errOut)
	}
}

func TestAnIntactChainVerifiesAndSaysNothingIsAnchored(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"valid":true,"checked":1200,"first_broken_seq":null,"gaps":[],"gap_count":0,"sealed_until":null}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"audit", "verify", "--from", "2026-08-01", "--to", "2026-08-27")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+auditVerifyPath {
		t.Errorf("path %q", stub.request.URL.Path)
	}
	if !strings.Contains(out, "1200") || !strings.Contains(out, "none") {
		t.Errorf("output %q", out)
	}
	// The check proves the chain intact inside the database and nothing more; "never" is the
	// honest version of a blank column.
	if !strings.Contains(out, "never") {
		t.Errorf("the anchoring is not stated: %q", out)
	}
}

// A broken chain is a failure of the command. A table with `valid false` in it and exit 0 beside
// it is how a scheduled check reports for years that everything is fine.
func TestABrokenChainFailsTheCommand(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"valid":false,"checked":900,"first_broken_seq":513,"gaps":[513,514,515,516],
		  "gap_count":4,"sealed_until":null}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"audit", "verify", "--from", "2026-08-01", "--to", "2026-08-27")
	if code != exitError {
		t.Fatalf("exit %d, want %d: %s", code, exitError, errOut)
	}
	if !strings.Contains(out, "513") {
		t.Errorf("the first broken entry is not shown: %q", out)
	}
	if !strings.Contains(errOut, "does not hold") {
		t.Errorf("the failure does not say what happened: %q", errOut)
	}
}

func TestExportingTheTrailSendsThePeriodAndTheTarget(t *testing.T) {
	stub := serveJSON(t, http.StatusAccepted,
		`{"job_id":"`+jobID+`","status":"QUEUED","result_url":null}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"audit", "export", "--from", "2026-08-01", "--to", "2026-08-27",
		"--target", targetID, "--format", "JSONL")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+auditExportPath {
		t.Errorf("path %q", stub.request.URL.Path)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["from"] != "2026-08-01T00:00:00Z" || sent["target_id"] != targetID || sent["format"] != "JSONL" {
		t.Errorf("the request lost something: %v", sent)
	}
	if !strings.Contains(out, "QUEUED") {
		t.Errorf("output %q", out)
	}
}
