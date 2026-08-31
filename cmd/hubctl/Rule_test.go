// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The automation surface as a person meets it (G-13). What is tested here is what this client
// decides: the shape it sends, the words it prints, and the one refusal it makes on its own.

const (
	ruleID    = "01936f2a-7c1e-7000-8000-0000000000f1"
	ruleRunID = "01936f2a-7c1e-7000-8000-0000000000f2"
)

const oneRule = `{"id":"` + ruleID + `","name":"Escalate overdue approvals",
  "scope":{"type":"TENANT","id":null},"enabled":false,"run_as":"` + itemID + `",
  "trigger":{"kind":"EVENT","event_type":"de.hubtask.work.item.overdue.v1"},
  "conditions":[],"actions":[{"kind":"ADD_LABEL"}],"on_error":"STOP","failure_count":0,
  "created_by":"` + itemID + `","created_at":"2026-08-20T08:00:00Z",
  "updated_at":"2026-08-20T08:00:00Z","version":1}`

// A rule is written switched off, and the client says so rather than enabling it as a kindness:
// writing what a rule would do and letting it loose on the workspace are two decisions.
func TestWritingARuleSendsItsShapeAndSaysItIsOff(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/me") {
			_, _ = w.Write([]byte(`{"id":"` + itemID + `","display_name":"Anna","email":"a@example.org"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(oneRule))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"rule", "add", "--name", "Escalate overdue approvals",
		"--trigger", "EVENT", "--event-type", "de.hubtask.work.item.overdue.v1",
		"--action", `ADD_LABEL:{"label_id":"`+collectionID+`"}`)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	trigger, _ := sent["trigger"].(map[string]any)
	if trigger["kind"] != "EVENT" || trigger["event_type"] != "de.hubtask.work.item.overdue.v1" {
		t.Errorf("trigger %v", sent["trigger"])
	}
	actions, _ := sent["actions"].([]any)
	if len(actions) != 1 {
		t.Fatalf("actions %v", sent["actions"])
	}
	action, _ := actions[0].(map[string]any)
	params, _ := action["params"].(map[string]any)
	if action["kind"] != "ADD_LABEL" || params["label_id"] != collectionID {
		t.Errorf("the action reached the server as %v", action)
	}
	// The account it acts as is answered by the installation rather than invented here.
	if sent["run_as"] != itemID {
		t.Errorf("run_as %v", sent["run_as"])
	}
	if !strings.Contains(out, "off") {
		t.Errorf("the table does not say the rule is off: %q", out)
	}
	if !strings.Contains(errOut, "rule enable") {
		t.Errorf("the note does not say how to switch it on: %q", errOut)
	}
}

// An action with no kind is a mistake this client names, because the round trip would add nothing.
func TestAnActionWithoutAKindIsRefusedHere(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneRule)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"rule", "add", "--name", "x", "--trigger", "MANUAL", "--action", ":{}")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "--action") {
		t.Errorf("the complaint does not name what is wrong: %q", errOut)
	}
}

// Parameters that are not a JSON object are the same kind of mistake, and are named as one rather
// than sent for the server to reject.
func TestActionParametersThatAreNotJSONAreRefusedHere(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneRule)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"rule", "add", "--name", "x", "--trigger", "MANUAL", "--action", "ADD_LABEL:not json")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "JSON object") {
		t.Errorf("the complaint is %q", errOut)
	}
}

// Enabling and disabling are two calls with two verbs, because the trail says which of them
// somebody did.
func TestEnablingAndDisablingUseTheirOwnVerbs(t *testing.T) {
	for _, verb := range []string{"enable", "disable"} {
		t.Run(verb, func(t *testing.T) {
			stub := serveJSON(t, http.StatusOK, oneRule)

			code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "rule", verb, ruleID)
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			if !strings.HasSuffix(stub.request.URL.Path, ":"+verb) {
				t.Errorf("the path is %q", stub.request.URL.Path)
			}
		})
	}
}

// The dry run says what would happen and says that nothing did.
func TestTheDryRunSaysNothingWasDone(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"matched":true,"condition_results":[],
		  "actions":[{"kind":"ADD_LABEL","path":"1","would_run":true}]}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "rule", "test", ruleID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "would run") {
		t.Errorf("the table is %q", out)
	}
	if !strings.Contains(errOut, "nothing was done") {
		t.Errorf("the note is %q", errOut)
	}
}

// A run is read back with the steps under it: the status answers "did it work", and the step that
// failed answers "why".
func TestARunIsPrintedWithItsSteps(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"id":"`+ruleRunID+`","rule_id":"`+ruleID+`","trigger":"EVENT","status":"FAILED",
		  "started_at":"2026-08-20T08:00:00Z","causation_depth":1,"condition_results":[],
		  "error_code":"automation.action_failed",
		  "action_results":[{"kind":"ADD_LABEL","status":"SUCCEEDED"},
		                    {"kind":"ASSIGN","status":"FAILED","error_code":"access.forbidden"}]}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "rule", "run", "show", ruleRunID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, want := range []string{"FAILED", "ADD_LABEL", "ASSIGN", "access.forbidden"} {
		if !strings.Contains(out, want) {
			t.Errorf("the output does not carry %q: %q", want, out)
		}
	}
}

// The inbound address is a credential, and a credential printed to a terminal says so once.
func TestTheRuleInboundTokenIsShownWithItsWarning(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"token":"`+ruleID+`.a-secret-nobody-else-has","rotated_at":"2026-08-20T08:00:00Z"}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "rule", "rotate-token", ruleID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "a-secret-nobody-else-has") {
		t.Errorf("the token is not in the answer: %q", out)
	}
	if !strings.Contains(errOut, "shown once") {
		t.Errorf("the warning is %q", errOut)
	}
}

// `--json` stays pipeable: the payload is the API's own, and the notes go to standard error.
func TestTheRuleListingIsPipeable(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"data":[`+oneRule+`],"page":{"has_more":true,"next_cursor":"c2"}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "--json", "rule", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var page map[string]any
	if err := json.Unmarshal([]byte(out), &page); err != nil {
		t.Fatalf("standard output is not the API's payload: %v", err)
	}
	if errOut != "" {
		t.Errorf("standard error carried %q, which would break a pipe reading both", errOut)
	}
}
