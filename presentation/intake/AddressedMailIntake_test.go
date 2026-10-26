// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package intake

import (
	"context"
	"encoding/base64"
	"testing"

	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	mailmodel "github.com/Jersyfi/hubtask/core/domain/model/mail"
)

// The receiving domain's door (ADR-0092, UC-JUM-09 checks 7 and 8): the key opens it, the
// headers say where a message goes and whether a machine sent it.

func TestTheInboundKeyOpensTheDoor(t *testing.T) {
	door := AddressedMailIntake{Key: "s3cret"}
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("postmark:s3cret"))
	for header, want := range map[string]bool{
		"Bearer s3cret": true,
		basic:           true,
		"Bearer wrong":  false,
		"":              false,
		"Basic " + base64.StdEncoding.EncodeToString([]byte("s3cret")): false,
	} {
		if got := door.Authorized(header); got != want {
			t.Errorf("Authorized(%q) = %t", header, got)
		}
	}
	if (AddressedMailIntake{}).Authorized("Bearer ") {
		t.Error("a door without a key opened")
	}
}

type recordingDeliveries struct{ got jumbleservice.ReceivedMail }

func (r *recordingDeliveries) Execute(
	_ context.Context, mail jumbleservice.ReceivedMail,
) (jumbleservice.MailDeliveryResult, error) {
	r.got = mail
	return jumbleservice.MailDeliveryResult{Outcome: mailmodel.OutcomeEntry}, nil
}

func TestTheHeadersSayWhereAndWhatKind(t *testing.T) {
	deliveries := &recordingDeliveries{}
	door := AddressedMailIntake{Key: "k", Deliveries: deliveries}
	raw := "From: Anna Beispiel <anna@example.org>\r\n" +
		"To: buero.aaaaaaaaaaaaaaaa@in.example.test\r\n" +
		"Cc: other@example.org\r\n" +
		"Delivered-To: buero.bbbbbbbbbbbbbbbb@in.example.test\r\n" +
		"Auto-Submitted: auto-replied\r\n" +
		"X-Spam-Flag: YES\r\n" +
		"Authentication-Results: mx.example; spf=fail smtp.mailfrom=example.org\r\n" +
		"Subject: Hello\r\n\r\nBody\r\n"

	if _, err := door.Deliver(context.Background(), "first@in.example.test", []byte(raw)); err != nil {
		t.Fatal(err)
	}
	got := deliveries.got
	want := []string{
		"first@in.example.test", "buero.bbbbbbbbbbbbbbbb@in.example.test",
		"buero.aaaaaaaaaaaaaaaa@in.example.test", "other@example.org",
	}
	if len(got.Recipients) != len(want) {
		t.Fatalf("recipients %v", got.Recipients)
	}
	for index := range want {
		if got.Recipients[index] != want[index] {
			t.Errorf("recipient %d is %q, want %q", index, got.Recipients[index], want[index])
		}
	}
	if got.Sender != "anna@example.org" || got.SenderName != "Anna Beispiel" {
		t.Errorf("sender %q %q", got.Sender, got.SenderName)
	}
	if !got.Automatic || !got.Spam || !got.AuthFailed {
		t.Errorf("signals %+v", got)
	}
}

func TestOwnNotificationsAreAutomatic(t *testing.T) {
	parsed, err := ParseMail([]byte("From: a@b.test\r\nX-Hubtask-Notification: 1\r\n"+
		"Auto-Submitted: no\r\nSubject: x\r\n\r\nbody"), MailLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Signals.Automatic {
		t.Error("Hubtask's own notification was not recognised")
	}
	plain, _ := ParseMail([]byte("From: a@b.test\r\nAuto-Submitted: no\r\nSubject: x\r\n\r\nbody"), MailLimits{})
	if plain.Signals.Automatic || plain.Signals.Spam || plain.Signals.AuthFailed {
		t.Errorf("a person's mail reads as %+v", plain.Signals)
	}
}
