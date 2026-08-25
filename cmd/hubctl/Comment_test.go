// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const commentID = "01936f2a-7c1e-7000-8000-0000000000e1"

const oneComment = `{"id":"` + commentID + `","item_id":"` + itemID + `",
  "author_id":"01936f2a-7c1e-7000-8000-0000000000a1","body":"On my way",
  "created_at":"2026-08-25T10:00:00Z","version":1}`

func TestListingCommentsWalksTheEntrysOwnCollection(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data":[`+oneComment+`],"page":{"has_more":false,"next_cursor":null}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "comment", "ls", itemID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := APIPath + "/items/" + itemID + "/comments"; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
	if !strings.Contains(out, "On my way") || !strings.Contains(out, "BODY") {
		t.Errorf("output %q is not the table", out)
	}
}

func TestAddingACommentSendsTheBodyAndTheParent(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneComment)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"comment", "add", itemID, "--body", "On my way", "--reply-to", commentID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPost {
		t.Errorf("method %s", stub.request.Method)
	}
	for _, fragment := range []string{`"body":"On my way"`, `"parent_comment_id":"` + commentID + `"`} {
		if !strings.Contains(stub.body, fragment) {
			t.Errorf("the body %q is missing %s", stub.body, fragment)
		}
	}
	if !strings.Contains(out, "On my way") {
		t.Errorf("the created comment was not shown: %q", out)
	}
}

func TestAddingACommentWithoutABodyIsRefusedBeforeTheCall(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made without a body")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "comment", "add", itemID)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	expected, _ := catalogue(t).Message("comments.body_required", nil)
	if !strings.Contains(errOut, expected) {
		t.Errorf("the message %q is not the catalogue's: %q", errOut, expected)
	}
}

// A deleted comment is a tombstone with a null body; the table must say so rather than crash or
// print an empty cell that looks like a comment about nothing.
func TestADeletedCommentReadsAsDeletedAndAMultilineBodyStaysOnItsRow(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data":[
	  {"id":"`+commentID+`","item_id":"`+itemID+`",
	   "author_id":"01936f2a-7c1e-7000-8000-0000000000a1","body":null,
	   "created_at":"2026-08-25T10:00:00Z","deleted_at":"2026-08-25T11:00:00Z","version":2},
	  {"id":"01936f2a-7c1e-7000-8000-0000000000e2","item_id":"`+itemID+`",
	   "author_id":"01936f2a-7c1e-7000-8000-0000000000a1","body":"two\nlines",
	   "created_at":"2026-08-25T10:05:00Z","version":1}
	],"page":{"has_more":false,"next_cursor":null}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "comment", "ls", itemID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "(deleted)") {
		t.Errorf("the tombstone does not read as deleted: %q", out)
	}
	if !strings.Contains(out, "two lines") {
		t.Errorf("the multi-line body did not stay on its row: %q", out)
	}
}
