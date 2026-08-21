// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

// containerPage is a page of one, the way the API returns it.
const containerPage = `{
  "data": [
    {"id":"01936f2a-7c1e-7000-8000-0000000000a1","type":"HUB","name":"Personal",
     "version":1,"effective_archived":false}
  ],
  "page": {"has_more": true, "next_cursor": "the-next-page"}
}`

func signedIn(stub *installation) map[string]string {
	return map[string]string{envURL: stub.server.URL, envToken: validToken}
}

func TestListingContainersRendersATableAndSaysHowToGoOn(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(containerPage))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"container", "ls", "--type", "HUB", "--include-archived")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	query := stub.request.URL.Query()
	if query.Get("type") != "HUB" || query.Get("include_archived") != "true" {
		t.Errorf("query %q does not carry the filters", stub.request.URL.RawQuery)
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "Personal") {
		t.Errorf("output %q is not the table", out)
	}
	// The cursor is a hint for a person, and it goes to standard error so that it never lands in
	// a pipe.
	if !strings.Contains(errOut, "the-next-page") {
		t.Errorf("standard error %q does not say how to continue", errOut)
	}
	if strings.Contains(out, "the-next-page") {
		t.Error("the hint reached standard output")
	}
}

func TestUnderJSONThePageIsTheOutputAndTheCursorIsInIt(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(containerPage))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "--json", "container", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, `"next_cursor": "the-next-page"`) {
		t.Errorf("the payload %q does not carry the cursor", out)
	}
	if errOut != "" {
		t.Errorf("standard error carried %q alongside a payload", errOut)
	}
}

func TestCreatingAContainerSendsTheDocumentedPayload(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"01936f2a-7c1e-7000-8000-0000000000a1","type":"COLLECTION",
			"name":"Errands","version":1,"parent_id":"01936f2a-7c1e-7000-8000-0000000000b1"}`))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"container", "create", "--type", "COLLECTION", "--name", "Errands",
		"--parent", "01936f2a-7c1e-7000-8000-0000000000b1")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	if stub.request.Method != http.MethodPost {
		t.Errorf("method %s", stub.request.Method)
	}
	for _, fragment := range []string{`"name":"Errands"`, `"type":"COLLECTION"`, `"parent_id":"01936f2a`} {
		if !strings.Contains(stub.body, fragment) {
			t.Errorf("the body %q is missing %s", stub.body, fragment)
		}
	}
	// Fields nobody typed are absent rather than empty: the API distinguishes the two.
	if strings.Contains(stub.body, `"description"`) || strings.Contains(stub.body, `"icon"`) {
		t.Errorf("the body %q carries fields that were never given", stub.body)
	}
	if !strings.Contains(out, "Errands") {
		t.Errorf("the created container was not shown: %q", out)
	}
}

func TestCreatingAContainerNeedsATypeAndAName(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made although the invocation was incomplete")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "container", "create", "--name", "Errands")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--type") {
		t.Errorf("the message %q does not say what is missing", errOut)
	}
}

// A misspelled kind is refused here with the catalogue's own sentence, rather than after a round
// trip that produces the same sentence from further away.
func TestAKindThatIsNotOneIsRefusedBeforeTheCall(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with a container type that does not exist")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"container", "create", "--type", "FOLDER", "--name", "Errands")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	expected, _ := catalogue(t).Message("containers.type_unknown", map[string]string{"value": "FOLDER"})
	if !strings.Contains(errOut, expected) {
		t.Errorf("the message %q is not the catalogue's: %q", errOut, expected)
	}
}

func TestAnIdentifierThatIsNotOneIsRefusedBeforeTheCall(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with an identifier that cannot be one")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "container", "ls", "--parent", "not-a-uuid")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--parent") {
		t.Errorf("the message %q does not name the flag that was wrong", errOut)
	}
}
