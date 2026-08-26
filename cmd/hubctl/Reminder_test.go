// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const reminderID = "01936f2a-7c1e-7000-8000-0000000000e1"

const oneReminder = `{"id":"` + reminderID + `","item_id":"` + itemID + `",
  "offset_spec":"REL:-PT30M","fire_at":"2026-09-10T08:30:00Z","state":"PENDING",
  "channels":["EMAIL"],"recipients":[],"created_at":"2026-09-01T09:00:00Z","version":1}`

// The convenience this group adds: the contract spells an offset REL:… or ABS:…, and a person
// types the offset. Both spellings reach the API as the contract's own.
func TestTheOffsetPrefixIsFilledInAndNeverOverwritten(t *testing.T) {
	cases := map[string]string{
		"-PT30M":                "REL:-PT30M",
		"P1D":                   "REL:P1D",
		"2026-09-10T09:00:00Z":  "ABS:2026-09-10T09:00:00Z",
		"REL:-PT30M":            "REL:-PT30M",
		"ABS:2026-09-10T09:00Z": "ABS:2026-09-10T09:00Z",
	}

	for typed, wanted := range cases {
		t.Run(typed, func(t *testing.T) {
			stub := serveJSON(t, http.StatusCreated, oneReminder)

			code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
				"remind", "add", itemID, "--at", typed)
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			if !strings.Contains(stub.body, `"offset_spec":"`+wanted+`"`) {
				t.Errorf("the body carries %s, want %s", stub.body, wanted)
			}
		})
	}
}

func TestARemindersRecipientsAndChannelsTravelAsLists(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneReminder)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"remind", "add", itemID, "--at", "-PT30M",
		"--to", collectionID+","+itemID, "--channel", "EMAIL")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"recipients":["`+collectionID+`","`+itemID+`"]`) {
		t.Errorf("the recipients came out as %s", stub.body)
	}
	if !strings.Contains(stub.body, `"channels":["EMAIL"]`) {
		t.Errorf("the channels came out as %s", stub.body)
	}
}

// One unreadable identifier refuses the whole list: reminding three of the four people somebody
// named would be doing something they did not ask for.
func TestOneBadRecipientRefusesTheWholeList(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with an unreadable recipient")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"remind", "add", itemID, "--at", "-PT30M", "--to", collectionID+",not-an-id")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--to") {
		t.Errorf("the message %q does not name the flag", errOut)
	}
}

func TestTheReminderListReadsAsATable(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[`+oneReminder+`]`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "remind", "ls", itemID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.HasSuffix(stub.request.URL.Path, "/items/"+itemID+"/reminders") {
		t.Errorf("called %s", stub.request.URL.Path)
	}
	if !strings.Contains(out, "REL:-PT30M") || !strings.Contains(out, "PENDING") {
		t.Errorf("the table is %q", out)
	}
}

func TestRemovingAReminderNamesBothIdentifiers(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"remind", "rm", itemID, reminderID, "--expect-version", "1")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.HasSuffix(stub.request.URL.Path, "/reminders/"+reminderID) {
		t.Errorf("called %s", stub.request.URL.Path)
	}
	if got := stub.request.Header.Get("If-Match"); got != `"1"` {
		t.Errorf("If-Match %q", got)
	}
}
