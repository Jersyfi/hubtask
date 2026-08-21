// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const trashPage = `{
  "data": [
    {"id":"` + itemID + `","kind":"ITEM","subtype":"TASK","title":"Buy milk","version":3,
     "deleted_at":"2026-08-20T14:30:00Z","trash_batch_id":"01936f2a-7c1e-7000-8000-0000000000e1",
     "hub_id":null,"collection_id":"` + collectionID + `","parent_id":null}
  ],
  "page": {"has_more": false, "next_cursor": null}
}`

func TestTheTrashListingShowsWhichEndpointRestoresEachRow(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, trashPage)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "trash", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+trashPath {
		t.Errorf("path %q", stub.request.URL.Path)
	}
	// The kind is what `trash restore --kind` needs, so the listing has to print it.
	if !strings.Contains(out, "KIND") || !strings.Contains(out, "ITEM") {
		t.Errorf("output %q does not carry the kind", out)
	}
	if !strings.Contains(out, "Buy milk") {
		t.Errorf("output %q does not carry the title", out)
	}
}

func TestRestoringChoosesTheEndpointFromTheKind(t *testing.T) {
	for _, tc := range []struct {
		kind    string
		payload string
		path    string
	}{
		{"ITEM", oneItem, APIPath + "/items/" + itemID + ":restore"},
		{
			"CONTAINER",
			`{"id":"` + itemID + `","type":"COLLECTION","name":"Errands","version":3}`,
			APIPath + "/containers/" + itemID + ":restore",
		},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			stub := serveJSON(t, http.StatusOK, tc.payload)

			code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
				"trash", "restore", itemID, "--kind", tc.kind)
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			if stub.request.URL.Path != tc.path {
				t.Errorf("path %q, want %q", stub.request.URL.Path, tc.path)
			}
			if !strings.Contains(out, itemID) {
				t.Errorf("the restored entry was not shown: %q", out)
			}
		})
	}
}

func TestRestoringWithoutAKindSaysWhereToFindIt(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made without knowing which endpoint restores the row")
	})

	for _, args := range [][]string{
		{"trash", "restore", itemID},
		{"trash", "restore", itemID, "--kind", "ENTRY"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", args...)
			if code != exitUsage {
				t.Fatalf("exit %d, want %d", code, exitUsage)
			}
			if !strings.Contains(errOut, "trash ls") {
				t.Errorf("the message %q does not say where the kind comes from", errOut)
			}
		})
	}
}
