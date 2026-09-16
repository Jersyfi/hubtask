// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

// Microsoft To Do has no export: the verb without a file says which two requests the file comes
// from, and makes none of them itself.
func TestImportingMicrosoftTodoWithoutAFileExplainsTheTwoRequests(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{}`)
	for _, kind := range []string{"microsoft-todo", "todo"} {
		code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "import", kind)
		if code != exitOK {
			t.Fatalf("%s: exit %d: %s", kind, code, errOut)
		}
		if !strings.Contains(out, "/me/todo/lists") || !strings.Contains(out, "/tasks") {
			t.Errorf("%s: the requests are not named: %q", kind, out)
		}
	}
	if stub.body != "" {
		t.Errorf("something was sent: %s", stub.body)
	}
}

func TestImportRefusesAKindItDoesNotKnow(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{}`)
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "import", "wunderlist", "file.json", "--hub", itemID)
	if code == exitOK || !strings.Contains(errOut, "wunderlist") {
		t.Errorf("exit %d: %s", code, errOut)
	}
}
