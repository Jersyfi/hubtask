// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestSearchingSendsTheWordsAsOneQuery(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data":[`+oneItem+`],"page":{"has_more":false,"next_cursor":null}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"search", "buy", "milk", "--container", collectionID, "--include-trashed")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := APIPath + searchPath; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
	if stub.request.Method != http.MethodPost {
		t.Errorf("method %s - the query is a document, not a query string", stub.request.Method)
	}
	for _, fragment := range []string{`"q":"buy milk"`, `"container_id":"` + collectionID + `"`, `"include_trashed":true`} {
		if !strings.Contains(stub.body, fragment) {
			t.Errorf("the body %q is missing %s", stub.body, fragment)
		}
	}
	if !strings.Contains(out, "Buy milk") {
		t.Errorf("the hits were not shown: %q", out)
	}
}

func TestSearchingWithoutWordsIsRefusedBeforeTheCall(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made without words")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "search")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	expected, _ := catalogue(t).Message("search.words_required", nil)
	if !strings.Contains(errOut, expected) {
		t.Errorf("the message %q is not the catalogue's: %q", errOut, expected)
	}
}

func TestWordsAfterTheFlagsAreAMistakeWorthNaming(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with words the flag parser dropped")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"search", "buy", "--include-trashed", "milk")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "before the flags") {
		t.Errorf("the message %q does not say where the words go", errOut)
	}
}

func TestSearchPagingTravelsInThePageObject(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data":[],"page":{"has_more":false,"next_cursor":null}}`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"search", "milk", "--size", "5", "--cursor", "abc")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"page":{"cursor":"abc","size":5}`) {
		t.Errorf("the body %q does not carry the page", stub.body)
	}
}

// The one verb the group has beside searching (M-09): `hubctl search reindex` asks for the
// workspace's search documents to be brought current and prints the job and the count. The word
// on its own is the verb; with any other word it is words again.
func TestSearchReindexAsksAndPrintsTheJob(t *testing.T) {
	stub := serveJSON(t, http.StatusAccepted,
		`{"job_id":"0192f000-0000-7000-8000-0000000000c1","stale":42}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "search", "reindex")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := APIPath + searchReindexPath; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
	if !strings.Contains(out, "42") || !strings.Contains(out, "0192f000-0000-7000-8000-0000000000c1") {
		t.Errorf("the answer was not shown: %q", out)
	}

	search := serveJSON(t, http.StatusOK, `{"data":[],"page":{"has_more":false,"next_cursor":null}}`)
	if code, _, errOut := invokeAgainst(t, search, signedIn(search), "", "search", "reindex", "now"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if search.request.URL.Path != APIPath+searchPath || !strings.Contains(search.body, `"q":"reindex now"`) {
		t.Errorf("two words did not search: %s %s", search.request.URL.Path, search.body)
	}
}
