// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const roleGrantID = "0195f0a0-0000-7000-8000-0000000000aa"

// S-08 gap: a role of the workspace's own is defined from hubctl, its permissions as a list.
func TestDefiningARoleSendsTheDocumentedPayload(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, `{"key":"reviewer","name":"Reviewer","permissions":["READ"],"version":1}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"role", "create", "--key", "reviewer", "--name", "Reviewer", "--permissions", "READ, COMMENT")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPost || stub.request.URL.Path != APIPath+"/roles" {
		t.Errorf("%s %s", stub.request.Method, stub.request.URL.Path)
	}
	for _, fragment := range []string{`"key":"reviewer"`, `"permissions":["READ","COMMENT"]`} {
		if !strings.Contains(stub.body, fragment) {
			t.Errorf("the body %q is missing %s", stub.body, fragment)
		}
	}
	if !strings.Contains(out, "reviewer") {
		t.Errorf("the answer is not shown: %q", out)
	}
}

func TestARequiredMemberIsAskedForBeforeTheCall(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made without the role's key")
	})
	if code, _, _ := invokeAgainst(t, stub, signedIn(stub), "", "role", "create", "--name", "Reviewer"); code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
}

// The path's parameters are the positional arguments, an identifier checked before the call.
func TestRevokingAGrantNamesItInThePath(t *testing.T) {
	stub := serveJSON(t, http.StatusNoContent, "")
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "role", "revoke", roleGrantID); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodDelete || stub.request.URL.Path != APIPath+"/role-grants/"+roleGrantID {
		t.Errorf("%s %s", stub.request.Method, stub.request.URL.Path)
	}

	if code, _, _ := invokeAgainst(t, stub, signedIn(stub), "", "role", "revoke", "not-an-id"); code != exitUsage {
		t.Errorf("a malformed identifier exits %d, want %d", code, exitUsage)
	}
}

// S-05 gap: a link type changed with the version read, as a merge patch.
func TestChangingALinkTypeCarriesTheVersionRead(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"key":"blocks","name":"Blocks","version":3}`)
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"link-type", "set", "blocks", "--name", "Blocks", "--expect-version", "2")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPatch || stub.request.URL.Path != APIPath+"/link-types/blocks" {
		t.Errorf("%s %s", stub.request.Method, stub.request.URL.Path)
	}
	if got := stub.request.Header.Get("If-Match"); got != `"2"` {
		t.Errorf("If-Match %q", got)
	}
	if got := stub.request.Header.Get("Content-Type"); !strings.Contains(got, "merge-patch") {
		t.Errorf("Content-Type %q", got)
	}
}

// S-06 gap: a form is set from a file, a two-word verb under the hand-written `type` group.
func TestSettingAFormSendsTheFile(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"key":"bug","name":"Bug","version":2}`)
	file := filepath.Join(t.TempDir(), "form.json")
	if err := os.WriteFile(file, []byte(`{"form":{"sections":[{"title":"Triage","fields":["title"]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "type", "form", "set", "bug", "--body", file)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPut || stub.request.URL.Path != APIPath+"/entry-types/bug/form" {
		t.Errorf("%s %s", stub.request.Method, stub.request.URL.Path)
	}
	if !strings.Contains(stub.body, `"sections"`) {
		t.Errorf("the body %q is not the file", stub.body)
	}
}

func TestGettingAFormPrintsItFromTheList(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data":[{"key":"bug","name":"Bug","level":"TASK","form":{"sections":[{"key":"triage","name":"Triage"}]}}]}`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "type", "form", "get", "bug")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, `"Triage"`) {
		t.Errorf("the form is not printed: %q", out)
	}
}

// S-07 gap: a workflow used in a hub, and the list drawn by its columns.
func TestWorkflowCommandsReachTheirOperations(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data":[{"key":"triage","name":"Triage","version":1}]}`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "workflow", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+"/workflows" || !strings.Contains(out, "triage") {
		t.Errorf("%s answered %q", stub.request.URL.Path, out)
	}

	code, _, errOut = invokeAgainst(t, stub, signedIn(stub), `{"workflow":"triage"}`,
		"workflow", "scheme", "set", "--body", "-", "--confirm", "true")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPut || stub.request.URL.Path != APIPath+"/workflow-scheme" {
		t.Errorf("%s %s", stub.request.Method, stub.request.URL.Path)
	}
	for _, fragment := range []string{`"workflow":"triage"`, `"confirm":true`} {
		if !strings.Contains(stub.body, fragment) {
			t.Errorf("the body %q is missing %s", stub.body, fragment)
		}
	}
}

// A hand-written command of the same name is kept: `type ls` stays the table it was.
func TestAHandWrittenCommandWins(t *testing.T) {
	for _, g := range groups() {
		seen := map[string]bool{}
		for _, c := range g.commands {
			if seen[c.name] {
				t.Errorf("hubctl %s %s is there twice", g.name, c.name)
			}
			seen[c.name] = true
		}
	}
}

func TestEveryGeneratedCommandIsInTheTree(t *testing.T) {
	tree := map[string]bool{}
	for _, g := range groups() {
		for _, c := range g.commands {
			tree[g.name+" "+c.name] = true
		}
	}
	for _, d := range generatedCommands {
		if !tree[d.Group+" "+d.Verb] {
			t.Errorf("hubctl %s %s (%s) is not reachable", d.Group, d.Verb, d.OperationID)
		}
	}
}

// The configuration verbs the hand-written group left out: a version read by number, a plan
// proposed for a person to approve.
func TestConfigVersionAndProposeAreGenerated(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"version":7,"hash":"h"}`)
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "config", "version", "7"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+"/config/versions/7" {
		t.Errorf("path %q", stub.request.URL.Path)
	}

	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "config", "propose", "--plan-hash", "abc"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+"/config:propose" || !strings.Contains(stub.body, `"plan_hash":"abc"`) {
		t.Errorf("%s %q", stub.request.URL.Path, stub.body)
	}
}

// An answer that is a plain array is listed by its rows, and a door that answers nothing says
// what it did rather than staying silent.
func TestAPlainArrayIsListedAndNoAnswerIsSaid(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[{"id":"`+roleGrantID+`","target_url":"https://hooks.example.test","state":"ACTIVE"}]`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "webhook", "ls")
	if code != exitOK || !strings.Contains(out, roleGrantID) || !strings.Contains(out, "TARGET_URL") {
		t.Fatalf("exit %d, %q: %s", code, out, errOut)
	}

	gone := serveJSON(t, http.StatusNoContent, "")
	code, out, errOut = invokeAgainst(t, gone, signedIn(gone), "", "webhook", "rm", roleGrantID)
	if code != exitOK || out != "" || !strings.Contains(errOut, "done:") || !strings.Contains(errOut, roleGrantID) {
		t.Errorf("exit %d, out %q, err %q", code, out, errOut)
	}
}
