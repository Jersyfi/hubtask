// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"mime"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// M-01's acceptance, end to end: two recipients in one workspace, one speaking German and one
// English, receive the same notification in two languages - rendered by the real catalogues,
// delivered through the real SMTP adapter, read back from the wire.
//
// The workspace's other tenant, so that the records this leaves behind are not the ones the
// outage test counts (test/integration is one shared database).
func TestTwoRecipientsAreWrittenToInTwoLanguages(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	stack := newNotificationStack(ctx, t)

	inviter := seedPerson(ctx, t, tenantB, "Anna", freshName(t)+"@test.invalid", "en")
	germanAddress := freshName(t) + "@test.invalid"
	englishAddress := freshName(t) + "@test.invalid"
	german := seedPerson(ctx, t, tenantB, "Dora", germanAddress, "de-AT")
	english := seedPerson(ctx, t, tenantB, "Bert", englishAddress, "en")

	running, stop := context.WithCancel(ctx)
	defer stop()
	concurrency.Go(running, "test.worker", stack.runner.Run)

	for _, invited := range []string{german.String(), english.String()} {
		enqueue(ctx, t, queue.Request{
			Kind: queue.KindInvitationEmail, TenantID: tenantB,
			DedupeKey: invited,
			Payload:   map[string]any{"account_id": invited, "invited_by": inviter.String()},
		})
	}

	var toGerman, toEnglish string
	waitFor(t, 15*time.Second, "both invitations to be delivered", func() bool {
		toGerman, toEnglish = "", ""
		for _, message := range stack.mail.messages() {
			switch {
			case strings.Contains(message, "To: "+germanAddress):
				toGerman = message
			case strings.Contains(message, "To: "+englishAddress):
				toEnglish = message
			}
		}
		return toGerman != "" && toEnglish != ""
	})

	if subject := subjectOf(t, toGerman); subject != "Du wurdest zu Hubtask eingeladen" {
		t.Errorf("Dora, whose locale is de-AT, was written to with the subject %q", subject)
	}
	if subject := subjectOf(t, toEnglish); subject != "You have been invited to Hubtask" {
		t.Errorf("Bert, whose locale is en, was written to with the subject %q", subject)
	}
	if !strings.Contains(toGerman, "hat dich eingeladen") {
		t.Errorf("Dora's body is not German:\n%s", toGerman)
	}
}

// subjectOf reads the Subject header off the wire and decodes it: the adapter writes it as an
// RFC 2047 encoded word, because a subject is not ASCII in most languages.
func subjectOf(t *testing.T, message string) string {
	t.Helper()
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimRight(line, "\r")
		if raw, found := strings.CutPrefix(line, "Subject: "); found {
			decoded, err := new(mime.WordDecoder).DecodeHeader(raw)
			if err != nil {
				t.Fatalf("decoding the subject %q: %v", raw, err)
			}
			return decoded
		}
	}
	t.Fatalf("no subject in:\n%s", message)
	return ""
}
