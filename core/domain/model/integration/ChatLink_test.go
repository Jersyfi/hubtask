// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"bytes"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestParseChatCommand(t *testing.T) {
	for _, c := range []struct {
		text string
		bare bool
		want ChatCommand
		ok   bool
	}{
		{"!link ab23cd34", false, ChatCommand{Verb: LinkCommand, Arg: "AB23CD34"}, true},
		{"  !APPROVE 7f3a2c10\nthanks", false, ChatCommand{Verb: ApproveCommand, Arg: "7F3A2C10"}, true},
		{"!decline 7f3a2c10 Over this quarter's budget.", false, ChatCommand{Verb: DeclineCommand, Arg: "7F3A2C10", Reason: "Over this quarter's budget."}, true},
		{"!decline 7f3a2c10", false, ChatCommand{Verb: DeclineCommand, Arg: "7F3A2C10"}, true},
		{"approve 7f3a2c10", true, ChatCommand{Verb: ApproveCommand, Arg: "7F3A2C10"}, true},
		{"approve 7f3a2c10", false, ChatCommand{}, false},
		{"approve the budget", true, ChatCommand{}, false},
		{"!link 12345678", false, ChatCommand{}, false},
		{"!approve", false, ChatCommand{}, false},
		{"!task approve the budget", false, ChatCommand{}, false},
		{"please !approve 7f3a2c10", false, ChatCommand{}, false},
	} {
		got, ok := ParseChatCommand(c.text, c.bare)
		if ok != c.ok || got != c.want {
			t.Errorf("ParseChatCommand(%q) = %+v, %v; want %+v, %v", c.text, got, ok, c.want, c.ok)
		}
	}
}

func TestChatLinkCodeIsReadLoosely(t *testing.T) {
	code := ChatLinkCode([]byte{1, 2, 3, 4, 5})
	if len(code) != 8 {
		t.Fatalf("code %q is not eight characters", code)
	}
	if !bytes.Equal(ChatLinkCodeHash(code), ChatLinkCodeHash(" "+code[:4]+" "+toLower(code[4:]))) {
		t.Fatal("a code said with a space or in lower case must be the same code")
	}
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func TestApprovalRefIsTheRandomTail(t *testing.T) {
	id := shared.ID("01890a5d-ac96-774b-bcce-b302099a8057")
	if got := ApprovalRef(id); got != "099A8057" {
		t.Fatalf("ApprovalRef = %q", got)
	}
}
