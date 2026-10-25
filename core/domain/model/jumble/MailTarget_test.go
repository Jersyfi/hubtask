// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"testing"
)

func TestAMailTargetNamesItsPlace(t *testing.T) {
	if _, err := NewMailTarget(TargetCollection, ""); err == nil {
		t.Error("a collection target without its collection was accepted")
	}
	target, err := NewMailTarget(TargetJumble, "")
	if err != nil || target.IsZero() {
		t.Fatalf("the jumble target: %+v, %v", target, err)
	}
	if !(MailTarget{}).IsZero() {
		t.Error("the zero target is not zero")
	}
}

func TestCutQuotedReply(t *testing.T) {
	cases := map[string]string{
		"Passt so.\n\nAm 03.02.2026 um 10:00 schrieb Anna <a@x.test>:\n> Hallo": "Passt so.",
		"Fine by me\r\n\r\nOn Mon, 3 Feb 2026 at 10:00, Ben wrote:\r\n> hi":     "Fine by me",
		"Sure\n> earlier\n> text\n":                      "Sure",
		"Top\n-----Original Message-----\nFrom: x":       "Top",
		"-----Original Message-----\nThe forwarded text": "-----Original Message-----\nThe forwarded text",
	}
	for in, want := range cases {
		if got := CutQuotedReply(in); got != want {
			t.Errorf("CutQuotedReply(%q) = %q, want %q", in, got, want)
		}
	}
}
