// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestMailAddressReadsAsLabelCodeDomain(t *testing.T) {
	address, err := NewMailAddress("a1", "t1", MailTarget{Kind: TargetJumble},
		[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, "p1", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(address.Code) != MailCodeLength || strings.ToLower(address.Code) != address.Code {
		t.Fatalf("code %q is not sixteen lowercase characters", address.Code)
	}
	got := address.Address("Büro & Co.", "IN.Example.test")
	if got != "buro-co."+address.Code+"@in.example.test" {
		t.Errorf("address %q", got)
	}
	code, ok := MailCodeOf("Anything."+strings.ToUpper(address.Code)+"+tag@in.example.test", "in.example.test")
	if !ok || code != address.Code {
		t.Errorf("code of the address: %q %t", code, ok)
	}
}

func TestMailCodeOfRefusesWhatIsNotAnAddressHere(t *testing.T) {
	for _, recipient := range []string{
		"buero.aaaaaaaaaaaaaaaa@elsewhere.test",  // another domain
		"buero.short@in.example.test",            // not a code's length
		"buero.aaaaaaaaaaaaaaa1@in.example.test", // 1 is not base32
		"no-at-sign",
	} {
		if _, ok := MailCodeOf(recipient, "in.example.test"); ok {
			t.Errorf("%q was read as an address here", recipient)
		}
	}
}

func TestMailLabel(t *testing.T) {
	cases := map[string]string{
		"Büro":     "buro",
		"Straße 7": "strasse-7",
		"  ":       "mail",
		"日本":       "mail",
		"A very long collection name that goes on": "a-very-long-collection-n",
	}
	for name, want := range cases {
		if got := MailLabel(name); got != want {
			t.Errorf("MailLabel(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestOpeningToAnyoneRecordsWho(t *testing.T) {
	address := MailAddress{Policy: SendersMembers}
	opened, err := address.WithPolicy(SendersAnyone, shared.ID("p1"))
	if err != nil || opened.OpenedBy != "p1" {
		t.Fatalf("opened %+v, %v", opened, err)
	}
	closed, _ := opened.WithPolicy(SendersMembers, "p2")
	if closed.OpenedBy != "" {
		t.Errorf("members-only still names %q", closed.OpenedBy)
	}
	if _, err := address.WithPolicy("SOME", "p1"); err == nil {
		t.Error("an unknown policy was taken")
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
