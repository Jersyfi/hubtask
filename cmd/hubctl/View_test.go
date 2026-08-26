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

const viewID = "01936f2a-7c1e-7000-8000-000000000201"

const oneView = `{"id":"` + viewID + `","name":"Due this week","scope_type":"COLLECTION",
  "scope_id":"` + collectionID + `","owner_id":"` + itemID + `","layout":"KANBAN",
  "sharing":"PRIVATE","query":{},"grouping":{},"visible_fields":[],"version":1}`

const queryDocument = `{"scope_container_id":"` + collectionID + `",
  "filter":{"field":"due_at","op":"LTE","value":"@today+P7D"}}`

func TestAViewsQueryIsReadFromAFileOrAPipe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "due.json")
	if err := os.WriteFile(path, []byte(queryDocument), 0o600); err != nil {
		t.Fatal(err)
	}

	stub := serveJSON(t, http.StatusCreated, oneView)
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"view", "create", "--name", "Due this week", "--scope", "COLLECTION",
		"--container", collectionID, "--layout", "KANBAN", "--query", path, "--share")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"op":"LTE"`) {
		t.Errorf("the query did not travel: %s", stub.body)
	}
	if !strings.Contains(stub.body, `"sharing":"SCOPE"`) {
		t.Errorf("--share did not reach the body: %s", stub.body)
	}
}

// The export is bytes, not a payload: what comes back is written as it is, whatever --json says.
func TestTheExportIsWrittenAsTheFileItIs(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("id,title\n1,Buy milk\n"))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"--json", "view", "export", viewID, "--format", "csv", "--zone", "Europe/Berlin")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.HasSuffix(stub.request.URL.Path, "/views/"+viewID+":export") {
		t.Errorf("called %s", stub.request.URL.Path)
	}
	if !strings.Contains(stub.body, `"format":"CSV"`) {
		t.Errorf("the format came out as %s, and a lower-case one is still a format", stub.body)
	}
	if !strings.Contains(stub.body, `"time_zone":"Europe/Berlin"`) {
		t.Errorf("the zone did not travel: %s", stub.body)
	}
	if out != "id,title\n1,Buy milk\n" {
		t.Errorf("the file came out as %q", out)
	}
}

// A truncation is announced on standard error, so a redirected file is exactly the export.
func TestATruncatedExportSaysSoBesideTheFile(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Export-Truncated", "true")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("id\n1\n"))
	})

	target := filepath.Join(t.TempDir(), "export.csv")
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"view", "export", viewID, "--format", "CSV", "--out", target)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if out != "" {
		t.Errorf("the file was written to the terminal as well: %q", out)
	}
	if !strings.Contains(errOut, "row cap") {
		t.Errorf("the truncation was not announced: %q", errOut)
	}
	written, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != "id\n1\n" {
		t.Errorf("the file holds %q", written)
	}
}

func TestAnUnknownExportFormatIsRefusedBeforeTheCall(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with an unknown format")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"view", "export", viewID, "--format", "PDF")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "CSV") {
		t.Errorf("the message %q does not name the formats", errOut)
	}
}

// Deleting a view says what it does to a feed over it - the question somebody asks afterwards.
func TestDeletingAViewMentionsTheFeed(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "view", "rm", viewID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(errOut, "calendar feed") {
		t.Errorf("the confirmation %q says nothing about the feed", errOut)
	}
}
