// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const (
	collectionID = "01936f2a-7c1e-7000-8000-0000000000c1"
	itemID       = "01936f2a-7c1e-7000-8000-0000000000d1"
)

// oneItem is what the API returns for a single entry.
const oneItem = `{"id":"` + itemID + `","type":"TASK","title":"Buy milk","version":2,
  "collection_id":"` + collectionID + `","completion":{"is_completed":false}}`

func serveJSON(t *testing.T, status int, payload string) *installation {
	t.Helper()
	return serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(payload))
	})
}

func TestListingItemsNeedsACollectionAndPassesItOn(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data":[`+oneItem+`],"page":{"has_more":false,"next_cursor":null}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "item", "ls", "--collection", collectionID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.URL.Query().Get("collection_id"); got != collectionID {
		t.Errorf("collection_id %q", got)
	}
	if !strings.Contains(out, "Buy milk") || !strings.Contains(out, "DONE") {
		t.Errorf("output %q is not the table", out)
	}
	// Nothing more to fetch, so nothing is said about a cursor.
	if errOut != "" {
		t.Errorf("standard error carried %q", errOut)
	}
}

func TestListingItemsWithoutACollectionIsAUsageError(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made without the required filter")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "item", "ls")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--collection") {
		t.Errorf("the message %q does not name what is missing", errOut)
	}
}

func TestCreatingAnItemSendsTheDocumentedPayload(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneItem)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"item", "create", "--collection", collectionID, "--type", "TASK", "--title", "Buy milk")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, fragment := range []string{`"title":"Buy milk"`, `"type":"TASK"`, `"collection_id":"` + collectionID + `"`} {
		if !strings.Contains(stub.body, fragment) {
			t.Errorf("the body %q is missing %s", stub.body, fragment)
		}
	}
	if !strings.Contains(out, "Buy milk") {
		t.Errorf("the created entry was not shown: %q", out)
	}
}

func TestAnItemTypeThatIsNotOneIsRefusedBeforeTheCall(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with an item type that does not exist")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"item", "create", "--collection", collectionID, "--type", "EPIC", "--title", "x")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	expected, _ := catalogue(t).Message("items.type_unknown", map[string]string{"value": "EPIC"})
	if !strings.Contains(errOut, expected) {
		t.Errorf("the message %q is not the catalogue's: %q", errOut, expected)
	}
}

func TestCompletingAnItemCallsTheActionSuffix(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"id":"`+itemID+`","type":"TASK","title":"Buy milk","version":3,
		  "collection_id":"`+collectionID+`","completion":{"is_completed":true}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "item", "complete", itemID, "--cascade")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := APIPath + "/items/" + itemID + ":complete"; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
	if !strings.Contains(stub.body, `"cascade_children":true`) {
		t.Errorf("the body %q does not carry the cascade", stub.body)
	}
	if !strings.Contains(out, "yes") {
		t.Errorf("the answer does not show the entry as done: %q", out)
	}
}

func TestMovingAnItemNeedsADestination(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made without a destination")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "item", "move", itemID)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--parent") {
		t.Errorf("the message %q does not say what a destination is", errOut)
	}
}

// Invariant I-W6: what a move could not carry over is reported, never dropped in silence.
func TestAMoveReportsWhatItCouldNotCarryOver(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"item":`+oneItem+`,
	  "dropped_references":[{"kind":"LABEL","id":"green","code":"items.label_not_in_destination"}]}`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"item", "move", itemID, "--collection", collectionID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(errOut, "LABEL") || !strings.Contains(errOut, "green") {
		t.Errorf("standard error %q does not name what was dropped", errOut)
	}
}

func TestTrashingAnItemSendsThePreconditionAndWritesNothingToAPipe(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"--json", "item", "rm", itemID, "--expect-version", "2")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodDelete {
		t.Errorf("method %s", stub.request.Method)
	}
	if got := stub.request.Header.Get("If-Match"); got != `"2"` {
		t.Errorf("If-Match %q, want the ETag of version 2", got)
	}
	// A deletion has no payload, so --json prints nothing rather than an invented document.
	if out != "" {
		t.Errorf("standard output carried %q", out)
	}
	if !strings.Contains(errOut, itemID) {
		t.Errorf("nothing confirmed the deletion: %q", errOut)
	}
}

func TestACommandThatTakesOneIdentifierSaysSoWhenGivenNone(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made without an identifier")
	})

	for _, args := range [][]string{{"item", "complete"}, {"item", "rm"}, {"item", "move", itemID, itemID}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", args...)
			if code != exitUsage {
				t.Fatalf("exit %d, want %d", code, exitUsage)
			}
			if !strings.Contains(errOut, "identifier") {
				t.Errorf("the message %q does not say what is expected", errOut)
			}
		})
	}
}
