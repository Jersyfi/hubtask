// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

// UC-INS-21 checks 2 and 5 from the command line: a module is listed and switched through the
// same REST use case the screen uses, for the workspace or one hub, and an unknown module is
// refused before anything is sent.
func TestModulesAreListedAndSwitchedFromTheCommandLine(t *testing.T) {
	answer := `{"modules":[{"module":"PLANNING","on":true,"source":"WORKSPACE"},
	  {"module":"SERVICE_DESK","on":true,"source":"PLAN","locked_by":"PLAN"}]}`

	stub := serveJSON(t, http.StatusOK, answer)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "module", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodGet || stub.request.URL.Path != APIPath+modulesPath {
		t.Errorf("%s %s", stub.request.Method, stub.request.URL.Path)
	}
	if !strings.Contains(out, "service_desk") || !strings.Contains(out, "PLAN") {
		t.Errorf("the answer is not the table: %q", out)
	}

	hub := "0190a000-0000-7000-8000-000000000001"
	stub = serveJSON(t, http.StatusOK, answer)
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "module", "off", "--hub", hub, "planning"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPut || stub.request.URL.Path != APIPath+modulesPath+"/PLANNING" ||
		!strings.Contains(stub.body, `"switch":"OFF"`) || !strings.Contains(stub.body, hub) {
		t.Errorf("%s %s with %s", stub.request.Method, stub.request.URL.Path, stub.body)
	}

	stub = serveJSON(t, http.StatusOK, answer)
	if code, _, _ := invokeAgainst(t, stub, signedIn(stub), "", "module", "on", "gantt"); code == exitOK {
		t.Error("an unknown module was sent")
	}
}

// UC-INS-21 check 13 from the command line: switching on at the workspace names the existing hubs
// to switch it on in with --hubs, sent as `hub_ids`; without it none is named, and it goes with
// neither one hub's own switch nor switching off.
func TestSwitchingOnNamesTheChosenHubsFromTheCommandLine(t *testing.T) {
	answer := `{"modules":[{"module":"PLANNING","on":true,"source":"WORKSPACE"}]}`
	clients, office := "0190a000-0000-7000-8000-000000000001", "0190a000-0000-7000-8000-000000000002"

	stub := serveJSON(t, http.StatusOK, answer)
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "module", "on", "--hubs", clients+", "+office, "planning"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPut || stub.request.URL.Path != APIPath+modulesPath+"/PLANNING" ||
		!strings.Contains(stub.body, `"switch":"ON"`) || !strings.Contains(stub.body, `"hub_ids":["`+clients+`","`+office+`"]`) ||
		strings.Contains(stub.body, `"hub_id"`) {
		t.Errorf("%s %s with %s", stub.request.Method, stub.request.URL.Path, stub.body)
	}

	stub = serveJSON(t, http.StatusOK, answer)
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "module", "on", "planning"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Contains(stub.body, "hub_ids") {
		t.Errorf("no hub was chosen, and the request names some: %s", stub.body)
	}

	for _, args := range [][]string{
		{"module", "on", "--hub", clients, "--hubs", office, "planning"},
		{"module", "off", "--hubs", office, "planning"},
		{"module", "on", "--hubs", "office", "planning"},
	} {
		stub = serveJSON(t, http.StatusOK, answer)
		if code, _, _ := invokeAgainst(t, stub, signedIn(stub), "", args...); code == exitOK || stub.request != nil {
			t.Errorf("hubctl %s was sent", strings.Join(args, " "))
		}
	}
}
