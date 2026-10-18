// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

// UC-INS-14 check 7: the use is read from the terminal - the range and the filters in the query, one
// row per workspace, metric and day, and where it is final.
func TestTheUseIsReadFromTheTerminal(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data": [{"workspace_id": "0192f000-0000-7000-8000-0000000000e1",
	  "purged": false, "metrics": {"accounts": [{"day": "2026-10-01", "value": 6}]}}],
	  "metrics": [{"name": "accounts", "aggregate": "HIGHEST", "day": "UTC"}],
	  "final_through": "2026-10-16", "page": {"next_cursor": null, "has_more": false}}`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "usage",
		"--from", "2026-10-01", "--to", "2026-10-01", "--metric", "accounts")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != "/api/v1/admin/usage" && !strings.HasSuffix(stub.request.URL.Path, "/admin/usage") {
		t.Errorf("called %s", stub.request.URL.Path)
	}
	if q := stub.request.URL.Query(); q.Get("from") != "2026-10-01" || q.Get("metric") != "accounts" {
		t.Errorf("asked %v", q)
	}
	if !strings.Contains(out, "accounts") || !strings.Contains(out, "2026-10-01") || !strings.Contains(out, "6") {
		t.Errorf("printed %q", out)
	}
	if !strings.Contains(errOut, "final through 2026-10-16") {
		t.Errorf("did not say where it is final: %q", errOut)
	}
	if code, _, _ := invokeAgainst(t, stub, signedIn(stub), "", "admin", "usage", "--from", "2026-10-01"); code == exitOK {
		t.Error("a read without --to was sent")
	}
}
