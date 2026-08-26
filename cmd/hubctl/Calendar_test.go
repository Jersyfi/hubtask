// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const feedID = "01936f2a-7c1e-7000-8000-000000000301"

const feedToken = "hbt_cal_01936f2a7c1e70008000000000000001_" +
	"AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"

// The minting is the only moment the credential exists outside the caller's hands, and the
// command says so - on standard error, so that the URL itself stays pipeable.
func TestMintingPrintsTheURLOnceAndWarnsBesideIt(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, `{"id":"`+feedID+`","account_id":"`+itemID+`",
	  "view_id":"`+viewID+`","created_at":"2026-09-01T09:00:00Z",
	  "token":"`+feedToken+`","url":"https://hubtask.example.com/api/v1/calendar/`+feedToken+`.ics"}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"calendar", "mint", "--view", viewID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"view_id":"`+viewID+`"`) {
		t.Errorf("the view did not travel: %s", stub.body)
	}
	if !strings.Contains(out, ".ics") {
		t.Errorf("the URL was not printed: %q", out)
	}
	if !strings.Contains(errOut, "shown once") {
		t.Errorf("nothing warned about the credential: %q", errOut)
	}
	// The warning is not in the pipe.
	if strings.Contains(out, "shown once") {
		t.Error("the warning landed on standard output")
	}
}

// The list cannot show a token, because the server keeps none - and this is the test that says so
// rather than a comment claiming it.
func TestTheFeedListCarriesNoCredential(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[{"id":"`+feedID+`","account_id":"`+itemID+`",
	  "view_id":"`+viewID+`","created_at":"2026-09-01T09:00:00Z"}]`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "calendar", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Contains(out, "hbt_cal_") || strings.Contains(errOut, "hbt_cal_") {
		t.Error("a token appeared in a list")
	}
	if !strings.Contains(out, feedID) {
		t.Errorf("the table %q does not name the feed", out)
	}
}

// A fetch is what a calendar client does: the URL, and no Authorization header on it.
func TestFetchingAFeedCarriesNoSecondCredential(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"calendar", "fetch", "--url", stub.server.URL+"/api/v1/calendar/"+feedToken+".ics")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.Header.Get("Authorization"); got != "" {
		t.Errorf("the fetch carried a bearer credential: %q", got)
	}
	if !strings.HasPrefix(out, "BEGIN:VCALENDAR") {
		t.Errorf("the document came out as %q", out)
	}
}

func TestAFetchNeedsAWholeURL(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made without a URL")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"calendar", "fetch", "--url", "/api/v1/calendar/"+feedToken+".ics")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "scheme") {
		t.Errorf("the message %q does not say what is missing", errOut)
	}
}

func TestRevokingSaysWhatItMeans(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "calendar", "revoke", feedID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.HasSuffix(stub.request.URL.Path, "/integrations/calendar-feeds/"+feedID) {
		t.Errorf("called %s", stub.request.URL.Path)
	}
	if !strings.Contains(errOut, "answers nothing") {
		t.Errorf("the confirmation %q does not say what changes", errOut)
	}
}
