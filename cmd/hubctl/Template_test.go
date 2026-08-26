// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const templateID = "01936f2a-7c1e-7000-8000-000000000101"

const oneTemplate = `{"id":"` + templateID + `","name":"Move house","scope_type":"COLLECTION",
  "scope_id":"` + collectionID + `","root_type":"TASK","version":1,
  "created_at":"2026-09-01T09:00:00Z",
  "nodes":[{"type":"TASK","title":"Move house","children":[
    {"type":"WORK_PACKAGE","title":"Book the van"},
    {"type":"WORK_PACKAGE","title":"Pack the kitchen","due_offset":"P3D"}]}]}`

const nodeTree = `[{"type":"TASK","title":"Move house","children":[
  {"type":"WORK_PACKAGE","title":"Book the van"}]}]`

// A tree is a document, so it comes from a file or from standard input rather than from flags.
func TestATemplatesTreeIsReadFromAFileOrAPipe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "move-house.json")
	if err := os.WriteFile(path, []byte(nodeTree), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, source := range []string{path, "-"} {
		t.Run(source, func(t *testing.T) {
			stub := serveJSON(t, http.StatusCreated, oneTemplate)

			stdin := ""
			if source == "-" {
				stdin = nodeTree
			}
			code, _, errOut := invokeAgainst(t, stub, signedIn(stub), stdin,
				"template", "create", "--name", "Move house", "--scope", "COLLECTION",
				"--container", collectionID, "--root-type", "TASK", "--nodes", source)
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			if !strings.Contains(stub.body, `"title":"Move house"`) ||
				!strings.Contains(stub.body, `"title":"Book the van"`) {
				t.Errorf("the tree did not travel: %s", stub.body)
			}
			if !strings.Contains(stub.body, `"scope_type":"COLLECTION"`) {
				t.Errorf("the scope did not travel: %s", stub.body)
			}
		})
	}
}

func TestATreeThatIsNotOneIsRefusedBeforeTheCall(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with an unreadable tree")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "not json at all",
		"template", "create", "--name", "Move house", "--scope", "TENANT",
		"--root-type", "TASK", "--nodes", "-")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--nodes") {
		t.Errorf("the message %q does not name the flag", errOut)
	}
}

// The list counts the tree, because "how big is this" is what a list is asked.
func TestTheTemplateListCountsTheNodes(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"data":[`+oneTemplate+`],"page":{"has_more":false,"next_cursor":null}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"template", "ls", "--container", collectionID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.URL.Query().Get("container_id"); got != collectionID {
		t.Errorf("container_id %q", got)
	}
	if !strings.Contains(out, "Move house") || !strings.Contains(out, "3") {
		t.Errorf("the table %q does not show the name and the node count", out)
	}
}

func TestInstantiatingSendsTheAnchorAsADateAndReportsWhatWasDropped(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, `{"template_id":"`+templateID+`",
	  "root_item_id":"`+itemID+`","created":3,
	  "dropped_references":[{"item_id":"`+itemID+`","kind":"ASSIGNEE",
	    "id":"`+collectionID+`","code":"items.assignee_not_visible"}]}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"template", "instantiate", templateID, "--collection", collectionID,
		"--anchor", "2026-09-07", "--title", "Move to the new flat")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.HasSuffix(stub.request.URL.Path, "/templates/"+templateID+":instantiate") {
		t.Errorf("called %s", stub.request.URL.Path)
	}
	if !strings.Contains(stub.body, `"anchor_date":"2026-09-07"`) {
		t.Errorf("the anchor came out as %s", stub.body)
	}
	if !strings.Contains(stub.body, `"title":"Move to the new flat"`) {
		t.Errorf("the title did not travel: %s", stub.body)
	}
	if !strings.Contains(out, "3") {
		t.Errorf("the table %q does not say how many entries came out", out)
	}
	// I-W6: what the destination could not carry is said out loud rather than counted quietly.
	if !strings.Contains(errOut, "ASSIGNEE") {
		t.Errorf("the dropped reference was not reported: %q", errOut)
	}
}

func TestAnAnchorThatIsNotADayIsRefused(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with an unreadable anchor")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"template", "instantiate", templateID, "--collection", collectionID,
		"--anchor", "next monday")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--anchor") {
		t.Errorf("the message %q does not name the flag", errOut)
	}
}
