// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var connectedAt = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func TestAPresetFillsWhatAPersonDoesNotType(t *testing.T) {
	box, err := NewMailbox(NewMailboxInput{ID: "m1", TenantID: "t1", Preset: "gmail", Address: " Familie@Gmail.com ", Now: connectedAt})
	if err != nil {
		t.Fatal(err)
	}
	if box.Protocol != ProtocolIMAP || box.Endpoint() != "imap.gmail.com:993" || box.Username != "familie@gmail.com" ||
		box.Folder != "INBOX" || box.DoneFolder != DefaultDoneFolder || !box.Enabled {
		t.Errorf("got %+v", box)
	}
	jmap, err := NewMailbox(NewMailboxInput{Preset: "fastmail", Address: "a@fastmail.com", Now: connectedAt})
	if err != nil || jmap.Protocol != ProtocolJMAP || jmap.SessionURL == "" {
		t.Errorf("fastmail %+v, %v", jmap, err)
	}
	if _, ok := PresetNamed("nothing"); ok {
		t.Error("an unknown preset was found")
	}
}

func TestAWrongMailboxNamesItsFields(t *testing.T) {
	_, err := NewMailbox(NewMailboxInput{
		Protocol: ProtocolIMAP, Address: "no address", Host: "local host", Port: 70000,
		Username: "a\r\nb", Folder: "in*box", Now: connectedAt,
	})
	var failure *shared.Error
	if !errors.As(err, &failure) {
		t.Fatalf("got %v", err)
	}
	paths := map[string]bool{}
	for _, field := range failure.Fields {
		paths[field.Path] = true
	}
	for _, want := range []string{"/address", "/host", "/port", "/username", "/folder"} {
		if !paths[want] {
			t.Errorf("no error at %s: %v", want, failure.Fields)
		}
	}
	if _, err := NewMailbox(NewMailboxInput{Protocol: ProtocolJMAP, Address: "a@b.de", SessionURL: "http://x"}); err == nil {
		t.Error("a JMAP session over plain HTTP was accepted")
	}
	if _, err := NewMailbox(NewMailboxInput{Protocol: "POP3", Address: "a@b.de"}); err == nil {
		t.Error("an unknown protocol was accepted")
	}
}

func TestAFailureIsCountedFromItsStart(t *testing.T) {
	box := Mailbox{}
	box = box.Checked(connectedAt, "mail.mailbox_refused")
	box = box.Checked(connectedAt.Add(30*time.Minute), "mail.mailbox_refused")
	if box.FailingSince == nil || !box.FailingSince.Equal(connectedAt) || box.Failing(connectedAt.Add(45*time.Minute)) {
		t.Errorf("after half an hour %+v", box)
	}
	if !box.Failing(connectedAt.Add(61 * time.Minute)) {
		t.Error("an hour of failures is not failing")
	}
	box = box.Checked(connectedAt.Add(2*time.Hour), "")
	if box.Failure != "" || box.FailingSince != nil || box.Failing(connectedAt.Add(3*time.Hour)) {
		t.Errorf("a good read did not clear it: %+v", box)
	}
}
