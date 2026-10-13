// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// `hubctl ai opt-out` (UC-PRV-05 check 9, P-08): the workspace's position from the terminal, with
// its basis, and the number of people it touches - never who.

const optOutAnswer = `{"source":"OWN","sources":"OWN","installation_sources":"OWN",
  "sources_origin":"DEFAULT","min_jurisdiction":"ANY","min_jurisdiction_origin":"DEFAULT","version":1,
  "offered":[],"person_opt_out":"NOT_OFFERED","person_opt_out_origin":"WORKSPACE",
  "person_opt_out_basis":"WORKS_AGREEMENT","person_opt_out_contact":"The works council",
  "keeping_out_count":2}`

func TestAWorkspaceMakesAiPartOfEverybodysWorkFromTheTerminal(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, optOutAnswer)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"ai", "opt-out", "set", "--position", "NOT_OFFERED", "--basis", "WORKS_AGREEMENT",
		"--contact", "The works council")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPut || stub.request.URL.Path != APIPath+aiPersonOptOutPath {
		t.Errorf("called %s %s", stub.request.Method, stub.request.URL.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["person_opt_out"] != "NOT_OFFERED" || sent["basis"] != "WORKS_AGREEMENT" || sent["contact"] != "The works council" {
		t.Errorf("sent %v", sent)
	}
	for _, expected := range []string{"NOT_OFFERED", "WORKS_AGREEMENT", "The works council", "2"} {
		if !strings.Contains(out, expected) {
			t.Errorf("%q is missing from %q", expected, out)
		}
	}
}

func TestTheOptOutNeedsAPosition(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, optOutAnswer)
	code, _, _ := invokeAgainst(t, stub, signedIn(stub), "", "ai", "opt-out", "set", "--basis", "WORKS_AGREEMENT")
	if code == exitOK {
		t.Error("a position nobody named was sent")
	}
}
