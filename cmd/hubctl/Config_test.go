// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

// The configuration document's verbs from the command line reach the same REST use cases as the
// screen and MCP (F-6): a document read from standard input is planned, and applied by its hash.
func TestAConfigurationIsPlannedAndAppliedFromTheCommandLine(t *testing.T) {
	document := `{"types":{"incident":{"level":"TASK","name":"Incident"}}}`
	plan := `{"base_hash":"b","plan_hash":"p1","changes":[{"op":"ADD","path":"types.incident","fields":["level","name"]}],"refusals":[],"operations":[],"flags":{"hides_data":false,"changes_permissions":false},"counts":{"added":1,"changed":0,"removed":0}}`

	stub := serveJSON(t, http.StatusOK, plan)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), document, "config", "plan", "-f", "-")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPost || stub.request.URL.Path != APIPath+"/config:plan" ||
		!strings.Contains(stub.body, `"mode":"MERGE"`) || !strings.Contains(stub.body, "incident") {
		t.Errorf("%s %s with %s", stub.request.Method, stub.request.URL.Path, stub.body)
	}
	if !strings.Contains(out, "types.incident") || !strings.Contains(out, "p1") {
		t.Errorf("the plan is not shown: %q", out)
	}

	stub = serveJSON(t, http.StatusOK, `{"state":"PROPOSED","proposal":{"id":"0190a000-0000-7000-8000-000000000009"}}`)
	code, out, errOut = invokeAgainst(t, stub, signedIn(stub), document, "config", "apply", "-f", "-", "--plan", "p1")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+"/config:apply" || !strings.Contains(stub.body, `"plan_hash":"p1"`) {
		t.Errorf("%s with %s", stub.request.URL.Path, stub.body)
	}
	if !strings.Contains(out, "PROPOSED") {
		t.Errorf("the answer is not shown: %q", out)
	}

	stub = serveJSON(t, http.StatusOK, `{}`)
	if code, _, _ := invokeAgainst(t, stub, signedIn(stub), document, "config", "apply", "-f", "-"); code == exitOK {
		t.Error("an apply without its plan hash was sent")
	}
}

// A hub's own document is planned with --hub, which names the hub in the body, and a plan shows
// what it reaches.
func TestAHubsConfigurationIsPlannedWithItsHub(t *testing.T) {
	plan := `{"base_hash":"b","plan_hash":"p2","changes":[],"refusals":[],"operations":[],"reach":[{"path":"scheme.every_type","entries":4,"moves":[],"removed":[]}],"flags":{"hides_data":false,"changes_permissions":false},"counts":{"added":0,"changed":1,"removed":0}}`
	stub := serveJSON(t, http.StatusOK, plan)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), `{"scheme":{"every_type":{"workflow":"service"}}}`,
		"config", "plan", "-f", "-", "--hub", "0190a000-0000-7000-8000-0000000000f1")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"hub_id":"0190a000-0000-7000-8000-0000000000f1"`) {
		t.Errorf("the hub is not named: %s", stub.body)
	}
	if !strings.Contains(out, "scheme.every_type") || !strings.Contains(out, "4 entries move") {
		t.Errorf("the reach is not shown: %q", out)
	}

	stub = serveJSON(t, http.StatusOK, `{"data":[]}`)
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "config", "history", "--hub", "0190a000-0000-7000-8000-0000000000f1"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Query().Get("hub_id") != "0190a000-0000-7000-8000-0000000000f1" {
		t.Errorf("the history does not ask for the hub's: %s", stub.request.URL.RawQuery)
	}
}
