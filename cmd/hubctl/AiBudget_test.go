// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

// The workspace sets its own model's budget from the command line, as from the screen (P-08), and
// clears it with a null - "no budget", the default.
func TestTheOwnAiBudgetIsSetAndClearedFromTheCommandLine(t *testing.T) {
	answer := `[{"quota":"ai_tokens_per_day","limit":200000,"used":10},
	  {"quota":"ai_own_tokens_per_day","limit":5000,"used":40,"configured":true}]`

	stub := serveJSON(t, http.StatusOK, answer)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "ai", "budget", "set", "5000")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPut || stub.request.URL.Path != APIPath+aiBudgetPath ||
		!strings.Contains(stub.body, `"own_tokens_per_day":5000`) {
		t.Errorf("%s %s with %s", stub.request.Method, stub.request.URL.Path, stub.body)
	}
	if !strings.Contains(out, "ai_own_tokens_per_day") || !strings.Contains(out, "5000") {
		t.Errorf("the answer is not the table: %q", out)
	}

	stub = serveJSON(t, http.StatusOK, answer)
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "ai", "budget", "clear"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"own_tokens_per_day":null`) {
		t.Errorf("clearing sent %s", stub.body)
	}

	stub = serveJSON(t, http.StatusOK, answer)
	if code, _, _ := invokeAgainst(t, stub, signedIn(stub), "", "ai", "budget", "set", "0"); code == exitOK {
		t.Error("a budget of nought was sent")
	}
}
