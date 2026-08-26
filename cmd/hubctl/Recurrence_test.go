// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const recurrenceID = "01936f2a-7c1e-7000-8000-0000000000f1"

const oneSeries = `{"id":"` + recurrenceID + `","item_id":"` + itemID + `",
  "rrule":"FREQ=WEEKLY;BYDAY=MO","time_zone":"Europe/Berlin","mode":"ON_SCHEDULE",
  "horizon_days":90,"created_at":"2026-09-01T09:00:00Z","version":1}`

func TestSettingASeriesSendsTheRuleAndTheZone(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneSeries)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"recur", "set", itemID, "--rule", "FREQ=WEEKLY;BYDAY=MO", "--zone", "Europe/Berlin",
		"--mode", "ON_COMPLETION", "--horizon", "30")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPut ||
		!strings.HasSuffix(stub.request.URL.Path, "/items/"+itemID+"/recurrence") {
		t.Fatalf("called %s %s", stub.request.Method, stub.request.URL.Path)
	}
	for _, want := range []string{
		`"rrule":"FREQ=WEEKLY;BYDAY=MO"`, `"time_zone":"Europe/Berlin"`,
		`"mode":"ON_COMPLETION"`, `"horizon_days":30`,
	} {
		if !strings.Contains(stub.body, want) {
			t.Errorf("the body carries %s, want %s", stub.body, want)
		}
	}
	if !strings.Contains(out, "FREQ=WEEKLY") || !strings.Contains(out, "Europe/Berlin") {
		t.Errorf("the table is %q", out)
	}
}

// Two ways to end a series is one too many, and the flags can see it without a round trip.
func TestASeriesCannotEndTwice(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with two endings")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"recur", "set", itemID, "--rule", "FREQ=DAILY", "--zone", "UTC",
		"--until", "2026-12-31T00:00:00Z", "--count", "10")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--until") || !strings.Contains(errOut, "--count") {
		t.Errorf("the message %q does not name both flags", errOut)
	}
}

func TestSkippingPostsToTheActionAndReadsTheRuleBack(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneSeries)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "recur", "skip", itemID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPost ||
		!strings.HasSuffix(stub.request.URL.Path, "/recurrence:skip") {
		t.Errorf("called %s %s", stub.request.Method, stub.request.URL.Path)
	}
}

// Removing a rule leaves what it produced, and the confirmation says so - that is the question
// somebody asks before typing it.
func TestRemovingASeriesSaysWhatItDoesNotTake(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "--json", "recur", "rm", itemID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if out != "" {
		t.Errorf("--json wrote %q into the pipe", out)
	}
	if !strings.Contains(errOut, "occurrences") {
		t.Errorf("the confirmation %q does not say what stays", errOut)
	}
}
