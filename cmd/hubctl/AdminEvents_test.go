// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const suspendedEvent = `{
  "specversion": "1.0", "id": "0192f000-0000-7000-8000-0000000000e9",
  "type": "de.hubtask.installation.workspace.suspended.v1", "time": "2026-10-17T09:00:00Z",
  "data": {"workspace": {"id": "0192f000-0000-7000-8000-00000000000a", "external_reference": "cus_4711",
    "status": "SUSPENDED", "plan": "familie", "version": 7}}
}`

// The read sends the cursor it is given, shows each event's workspace state, and prints the cursor
// to continue with on standard error, where a pipe does not carry it.
func TestTheInstallationsEventsAreReadForward(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"data": [`+suspendedEvent+`], "page": {"next_cursor": "c2", "has_more": false}}`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "events", "--cursor", "c1", "--size", "20")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+adminEventsPath ||
		stub.request.URL.Query().Get("cursor") != "c1" || stub.request.URL.Query().Get("size") != "20" {
		t.Errorf("the read called %s", stub.request.URL)
	}
	for _, expected := range []string{"suspended", "SUSPENDED", "familie", "7", "cus_4711"} {
		if !strings.Contains(out, expected) {
			t.Errorf("%s is missing from %q", expected, out)
		}
	}
	if !strings.Contains(errOut, "continue with --cursor c2") || strings.Contains(out, "c2") {
		t.Errorf("the cursor went to %q / %q", errOut, out)
	}
}
