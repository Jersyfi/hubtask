// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const oneWorkspace = `{"id": "0192f000-0000-7000-8000-0000000000e1", "slug": "maria", "display_name": "Maria",
  "status": "ACTIVE", "created_at": "2026-10-01T08:00:00Z", "plan": "family", "version": 6,
  "external_reference": "cus_8f2a91"}`

// The reference travels in the body, never in the query string, which access logs keep.
func TestAWorkspaceIsFoundByItsReference(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data": [`+oneWorkspace+`], "page": {"next_cursor": "c2", "has_more": true}}`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "tenant", "find", "--external-reference", "cus_8f2a91")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPost || !strings.HasSuffix(stub.request.URL.Path, "/admin/tenants:query") ||
		stub.request.URL.RawQuery != "" || !strings.Contains(stub.body, `"external_reference":"cus_8f2a91"`) {
		t.Errorf("sent %s %s?%s with %s", stub.request.Method, stub.request.URL.Path, stub.request.URL.RawQuery, stub.body)
	}
	if !strings.Contains(out, "maria") || !strings.Contains(out, "family") || !strings.Contains(errOut, "--cursor c2") {
		t.Errorf("printed %q / %q", out, errOut)
	}
}

func TestAReferenceIsSetWithTheVersionRead(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneWorkspace)
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "tenant", "set-reference",
		"0192f000-0000-7000-8000-0000000000e1", "--reference", "cus_8f2a91", "--expected-version", "5")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPatch || stub.request.Header.Get("If-Match") != `"5"` ||
		!strings.Contains(stub.body, `"external_reference":"cus_8f2a91"`) {
		t.Errorf("sent %s with %q and %s", stub.request.Method, stub.request.Header.Get("If-Match"), stub.body)
	}
}

func TestAWorkspaceIsShownAlone(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneWorkspace)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "tenant", "show", "0192f000-0000-7000-8000-0000000000e1")
	if code != exitOK || !strings.Contains(out, "cus_8f2a91") {
		t.Errorf("exit %d, printed %q / %q", code, out, errOut)
	}
}
