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

const mediaID = "01936f2a-7c1e-7000-8000-0000000000b1"

// TestUploadingWalksTheThreeStepFlow drives the whole contract: stage, put the bytes where the
// staging answer said, confirm. The upload target is relative, as a local-storage installation
// answers, and carries the token that is its whole credential - so the PUT must go to it as
// given and must not add the bearer.
func TestUploadingWalksTheThreeStepFlow(t *testing.T) {
	var putBody []byte
	var putAuth, putPath string
	confirmed := false
	// Declared before the handler so the handler can read what the recording wrapper captured:
	// the wrapper consumes each request's body into stub.body before handing over.
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == APIPath+mediaPath:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"` + mediaID + `","status":"PENDING","usage":"ATTACHMENT",
			  "content_type":"text/plain","size":5,"ref_count":0,
			  "created_at":"2026-08-25T10:00:00Z","created_by":"` + mediaID + `",
			  "upload":{"method":"PUT","url":"/api/v1/media/` + mediaID + `:content?token=abc",
			            "expires_at":"2026-08-25T11:00:00Z"}}`))
		case r.Method == http.MethodPut:
			putBody = []byte(stub.body)
			putAuth = r.Header.Get("Authorization")
			putPath = r.URL.Path + "?" + r.URL.RawQuery
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":confirm"):
			confirmed = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"` + mediaID + `","status":"READY","usage":"ATTACHMENT",
			  "content_type":"text/plain","size":5,"ref_count":0,"file_name":"note.txt",
			  "created_at":"2026-08-25T10:00:00Z","created_by":"` + mediaID + `"}`))
		default:
			t.Errorf("unexpected call: %s %s", r.Method, r.URL)
		}
	})

	file := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "media", "upload", file)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if string(putBody) != "hello" {
		t.Errorf("the bytes that travelled: %q", putBody)
	}
	if putAuth != "" {
		t.Errorf("the bearer travelled to the capability URL: %q", putAuth)
	}
	if want := APIPath + "/media/" + mediaID + ":content?token=abc"; putPath != want {
		t.Errorf("the PUT went to %q, want %q", putPath, want)
	}
	if !confirmed {
		t.Error("the upload was never confirmed")
	}
	if !strings.Contains(out, "READY") || !strings.Contains(out, "note.txt") {
		t.Errorf("the confirmed object was not shown: %q", out)
	}
}

func TestAMediaUsageThatIsNotOneIsRefusedBeforeTheCall(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with a usage that does not exist")
	})
	file := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"media", "upload", file, "--usage", "AVATAR")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	expected, _ := catalogue(t).Message("media.usage_unknown", map[string]string{"value": "AVATAR"})
	if !strings.Contains(errOut, expected) {
		t.Errorf("the message %q is not the catalogue's: %q", errOut, expected)
	}
}

func TestAttachingPutsTheMediaUnderTheEntry(t *testing.T) {
	stub := serveJSON(t, http.StatusOK,
		`{"item_id":"`+itemID+`","media_ids":["`+mediaID+`"]}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"media", "attach", itemID, "--media", mediaID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := APIPath + "/items/" + itemID + "/attachments/" + mediaID; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
	if stub.request.Method != http.MethodPut {
		t.Errorf("method %s - attaching is idempotent and PUT says so", stub.request.Method)
	}
	if stub.body != "" {
		t.Errorf("an attach carries no body, sent %q", stub.body)
	}
	if !strings.Contains(out, mediaID) {
		t.Errorf("the attachments were not shown: %q", out)
	}
}

func TestAttachingNeedsTheMedia(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made without a media object")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "media", "attach", itemID)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--media") {
		t.Errorf("the message %q does not name what is missing", errOut)
	}
}
