// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

// The one piece of grammar the CLI adds on top of the contract: --at takes a day or a moment, and
// which one was typed decides whether the entry is due all day or at a time (D-01, D-09).

const datedItem = `{"id":"` + itemID + `","type":"TASK","title":"Buy milk","version":3,
  "collection_id":"` + collectionID + `","completion":{"is_completed":false},
  "due_at":"2026-09-10T00:00:00Z","due_date_only":true,"due_time_zone":"Europe/Berlin"}`

func TestADayIsAnAllDayDueDateAndAMomentIsNot(t *testing.T) {
	cases := []struct {
		name     string
		at       string
		wantDue  string
		wantFlag bool
	}{
		{"a day", "2026-09-10", `"due_at":"2026-09-10T00:00:00Z"`, true},
		{"a moment", "2026-09-10T09:00:00Z", `"due_at":"2026-09-10T09:00:00Z"`, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := serveJSON(t, http.StatusOK, datedItem)

			code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
				"due", "set", itemID, "--at", c.at, "--zone", "Europe/Berlin")
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			if stub.request.Method != http.MethodPut ||
				!strings.HasSuffix(stub.request.URL.Path, "/items/"+itemID+"/due") {
				t.Fatalf("called %s %s", stub.request.Method, stub.request.URL.Path)
			}
			if !strings.Contains(stub.body, c.wantDue) {
				t.Errorf("the body carries %s", stub.body)
			}
			if carried := strings.Contains(stub.body, `"due_date_only":true`); carried != c.wantFlag {
				t.Errorf("the all-day flag is %v in %s", carried, stub.body)
			}
			if !strings.Contains(stub.body, `"due_time_zone":"Europe/Berlin"`) {
				t.Errorf("the zone did not travel: %s", stub.body)
			}
		})
	}
}

// Something that is neither a day nor a moment is a usage error, and the message says both
// spellings rather than only the one the server would have named.
func TestADateThatIsNeitherIsRefusedBeforeTheCall(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with an unreadable date")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"due", "set", itemID, "--at", "next monday")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "2026-09-10") || !strings.Contains(errOut, "T09:00:00Z") {
		t.Errorf("the message %q does not show both spellings", errOut)
	}
}

// The expected version travels as an entity tag, which is what makes a clear safe to script.
func TestClearingADateSendsThePreconditionAndPrintsNothingToAPipe(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"--json", "due", "clear", itemID, "--expect-version", "3")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodDelete {
		t.Errorf("called %s", stub.request.Method)
	}
	if got := stub.request.Header.Get("If-Match"); got != `"3"` {
		t.Errorf("If-Match %q", got)
	}
	if out != "" {
		t.Errorf("--json wrote %q into the pipe for an answer that has no payload", out)
	}
	if !strings.Contains(errOut, itemID) {
		t.Errorf("the confirmation %q does not name the entry", errOut)
	}
}

// A date can be given at creation, which is where a person usually knows it.
func TestAnEntryCanBeCreatedWithItsDate(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, datedItem)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"item", "create", "--collection", collectionID, "--type", "TASK", "--title", "Buy milk",
		"--due", "2026-09-10", "--due-zone", "Europe/Berlin")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"due_at":"2026-09-10T00:00:00Z"`) ||
		!strings.Contains(stub.body, `"due_date_only":true`) ||
		!strings.Contains(stub.body, `"due_time_zone":"Europe/Berlin"`) {
		t.Errorf("the create carried %s", stub.body)
	}
}

// A zone with no date is refused here rather than by the server, which saves a round trip for a
// mistake the flags themselves can see.
func TestAZoneWithoutADateIsAUsageError(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with a zone and no date")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"item", "create", "--collection", collectionID, "--type", "TASK", "--title", "Buy milk",
		"--due-zone", "Europe/Berlin")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--due-zone") {
		t.Errorf("the message %q does not name the flag", errOut)
	}
}
