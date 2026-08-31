// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

// The pull half of the event stream (G-13, G-04). What this client decides is the cursor: a poll
// without one asks the unbounded question, so the next one is printed after every call - and on
// standard error, where a pipe reading the payload does not carry it.

const onePolledEvent = `{"id":"01936f2a-7c1e-7000-8000-000000000d11",
  "type":"de.hubtask.work.item.completed.v1","time":"2026-08-20T08:00:00Z",
  "subject":"01936f2a-7c1e-7000-8000-000000000d12","data":{"title":"Buy milk"}}`

func TestPollingPassesTheCursorAndPrintsTheNextOne(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"data":[`+onePolledEvent+`],"page":{"has_more":true,"next_cursor":"c2"}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"events", "poll", "de.hubtask.work.item.completed.v1", "--since", "c1", "--limit", "50")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	if got := stub.request.URL.Query().Get("since"); got != "c1" {
		t.Errorf("since %q", got)
	}
	if got := stub.request.URL.Query().Get("limit"); got != "50" {
		t.Errorf("limit %q", got)
	}
	if !strings.Contains(stub.request.URL.Path, "de.hubtask.work.item.completed.v1") {
		t.Errorf("the path is %q", stub.request.URL.Path)
	}
	if !strings.Contains(out, "de.hubtask.work.item.completed.v1") {
		t.Errorf("the table is %q", out)
	}
	if !strings.Contains(errOut, "--since c2") {
		t.Errorf("the next cursor was not printed: %q", errOut)
	}
}

// The payload is the API's own and the cursor is not mixed into it: a poller that pipes one must
// not have to strip the other.
func TestTheCursorStaysOffTheJSONPayload(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"data":[`+onePolledEvent+`],"page":{"has_more":true,"next_cursor":"c2"}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"--json", "events", "poll", "de.hubtask.work.item.completed.v1")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Contains(out, "--since") {
		t.Errorf("the cursor reached standard output: %q", out)
	}
	if !strings.Contains(errOut, "--since c2") {
		t.Errorf("the cursor did not reach standard error: %q", errOut)
	}
}

// An event type comes first, because it is what is being polled - and a poll without one is a
// mistake this client names.
func TestPollingWithoutAnEventTypeIsRefusedHere(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data":[],"page":{"has_more":false,"next_cursor":null}}`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "events", "poll", "--since", "c1")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "event type") {
		t.Errorf("the complaint is %q", errOut)
	}
}

// A page with nothing after it says nothing: a cursor printed when there is no more to fetch would
// train a poller to keep asking.
func TestTheLastPageSaysNothingAboutACursor(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"data":[`+onePolledEvent+`],"page":{"has_more":false,"next_cursor":null}}`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"events", "poll", "de.hubtask.work.item.completed.v1")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if errOut != "" {
		t.Errorf("standard error carried %q", errOut)
	}
}
