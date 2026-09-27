// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// resetMinter is the identity service's seam, in memory.
type resetMinter struct {
	link  ResetLink
	asked []shared.ID
}

func (m *resetMinter) MintResetToken(
	_ context.Context, _, accountID shared.ID,
) (ResetLink, error) {
	m.asked = append(m.asked, accountID)
	return m.link, nil
}

// The mail is rendered from message codes in the recipient's language, and the token travels in
// the URL's fragment - so nothing between the mail client and the interface ever sees it.
func TestTheResetMailCarriesTheLinkInTheFragment(t *testing.T) {
	mailer := &mailbox{}
	minter := &resetMinter{link: ResetLink{
		Token: secret.New("hbt_mfa_0123_abc"), HasPassword: true,
		Address: "mara@contoso.example", Locale: "de",
	}}

	err := SendPasswordReset{
		Resets: minter, Mail: mailer, Renderer: catalogue{},
		FallbackLocale: "en", BaseURL: "https://acme.example/",
	}.Execute(t.Context(), tenant, anna)
	if err != nil {
		t.Fatalf("sending: %v", err)
	}

	if len(mailer.sent) != 1 {
		t.Fatalf("%d messages sent", len(mailer.sent))
	}
	sent := mailer.sent[0]
	if sent.To != "mara@contoso.example" {
		t.Errorf("the message went to %q", sent.To)
	}
	if !strings.Contains(sent.Body, "https://acme.example/reset#token=hbt_mfa_0123_abc") {
		t.Errorf("the body reads %q", sent.Body)
	}
	if !strings.Contains(sent.Subject, subjectPasswordReset) {
		t.Errorf("the subject was rendered from %q", sent.Subject)
	}
	// The recipient's own language, never the installation's: the renderer is handed one and
	// there is no call that can forget it.
	if !strings.HasPrefix(sent.Subject, "[de] ") {
		t.Errorf("the subject was rendered in %q", sent.Subject)
	}
}

// An account that signs in through a provider gets the other mail, and it carries no link: a reset
// screen for somebody with no password would be a second way in that nobody asked for.
func TestAProviderAccountGetsNoLink(t *testing.T) {
	mailer := &mailbox{}

	err := SendPasswordReset{
		Resets: &resetMinter{link: ResetLink{
			HasPassword: false, Address: "mara@contoso.example",
		}},
		Mail: mailer, Renderer: catalogue{}, FallbackLocale: "en", BaseURL: "https://acme.example",
	}.Execute(t.Context(), tenant, anna)
	if err != nil {
		t.Fatalf("sending: %v", err)
	}

	if len(mailer.sent) != 1 {
		t.Fatalf("%d messages sent", len(mailer.sent))
	}
	if !strings.Contains(mailer.sent[0].Subject, subjectPasswordResetProvider) {
		t.Errorf("the subject was rendered from %q", mailer.sent[0].Subject)
	}
	if strings.Contains(mailer.sent[0].Body, "reset#token=") {
		t.Errorf("a link was sent to an account with no password: %q", mailer.sent[0].Body)
	}
	// The installation's language, where the account has chosen none: the fake renders German
	// with a marker and everything else plain, so a plain subject is the fallback in use.
	if strings.HasPrefix(mailer.sent[0].Subject, "[de] ") {
		t.Errorf("the subject was rendered in %q, want the installation's", mailer.sent[0].Subject)
	}
}

// An account that is gone between the request and the job is finished business rather than a
// failure: there is nothing a retry would find.
func TestAnAccountThatIsGoneSendsNothing(t *testing.T) {
	mailer := &mailbox{}

	err := SendPasswordReset{
		Resets: &resetMinter{}, Mail: mailer, Renderer: catalogue{}, FallbackLocale: "en",
	}.Execute(t.Context(), tenant, anna)

	if err != nil {
		t.Fatalf("a missing account answered %v", err)
	}
	if len(mailer.sent) != 0 {
		t.Errorf("%d messages sent to nobody", len(mailer.sent))
	}
}
