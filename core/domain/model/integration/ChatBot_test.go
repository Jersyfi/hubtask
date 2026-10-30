// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func botCode(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

// UC-INT-18 check 1: a bot is set up on an https homeserver, in a room by its identifier, and does
// at least one thing.
func TestCheckChatBotRefusesWhatCannotBeRead(t *testing.T) {
	hs, room, locale, err := CheckChatBot("https://Matrix.Example.org/_matrix", "!abc:example.org", "de", true, false)
	if err != nil || hs != "https://matrix.example.org" || room != "!abc:example.org" || locale != "de" {
		t.Fatalf("%q %q %q %v", hs, room, locale, err)
	}
	for _, c := range []struct {
		hs, room        string
		unfurl, capture bool
		code            string
	}{
		{"http://matrix.example.org", "!a:b", true, true, "chatbot.homeserver_invalid"},
		{"https://matrix.example.org", "#team:example.org", true, true, "webhooks.room_malformed"},
		{"https://matrix.example.org", "", true, true, "webhooks.room_required"},
		{"https://matrix.example.org", "!a:b", false, false, "chatbot.nothing_to_do"},
	} {
		if _, _, _, err := CheckChatBot(c.hs, c.room, "", c.unfurl, c.capture); botCode(err) != c.code {
			t.Errorf("%+v: %v", c, err)
		}
	}
}

// UC-INT-18 check 3: `!task` and a title become an entry's title; anything else is no command.
func TestCapturedReadsTheTaskCommand(t *testing.T) {
	for in, want := range map[string]string{
		"!task Renew the certificate":          "Renew the certificate",
		"  !TASK   Call Ada\nwith the details": "Call Ada",
		"!task " + strings.Repeat("x", 600):    strings.Repeat("x", MaxCapturedTitle),
	} {
		if got, ok := Captured(in); !ok || got != want {
			t.Errorf("%q: %q %v", in, got, ok)
		}
	}
	for _, in := range []string{"!task", "!task   ", "!tasks are fun", "a !task inside", ""} {
		if _, ok := Captured(in); ok {
			t.Errorf("%q was taken for a command", in)
		}
	}
}

// UC-INT-18 check 2: a link to this workspace's entry is found, each once and at most three; a link
// elsewhere is not.
func TestLinkedEntriesFindsThisWorkspacesEntries(t *testing.T) {
	base := "https://hub.example.org"
	a, b := "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b", "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"
	body := "see " + base + "/items/" + a + " and " + base + "/items/" + strings.ToUpper(a) + "?tab=1, also " +
		"https://other.example.net/items/" + b + " and " + base + "/items/" + b + " " + base + "/items/not-an-id"
	got := LinkedEntries(body, base+"/")
	if len(got) != 2 || got[0] != shared.ID(a) || got[1] != shared.ID(b) {
		t.Fatalf("got %v", got)
	}
	many := ""
	for _, c := range "0123456789" {
		many += " " + base + "/items/0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5" + string(c)
	}
	if len(LinkedEntries(many, base)) != MaxUnfurlsPerPost {
		t.Fatal("more than three unfurled")
	}
	if len(LinkedEntries(body, "")) != 0 {
		t.Fatal("no base, no links")
	}
}
