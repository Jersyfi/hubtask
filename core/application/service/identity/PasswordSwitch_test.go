// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// SC-24 (#1119, UC-ID-12 check 6): where a workspace has switched the password off, the server
// refuses it at every door - not only the sign-in card. The refusal is the same for every address,
// stored passwords are kept, and ADR-0076 §4's fallback is the one exception (PasswordFallback_test).

// passwordOff switches the workspace the fixture resolves to providers only.
func passwordOff(f *passwordFixture) {
	methods := []string{domain.MethodOidc}
	f.workspace.row.Settings.SignIn.Methods = &methods
}

func TestASwitchedOffPasswordIsRefusedForEveryAddress(t *testing.T) {
	f := newStepFixture(now)
	passwordOff(f.passwords)
	f.session.withAccount("bert@example.org", "correct horse battery")

	for _, address := range []string{"bert@example.org", "nobody@example.org"} {
		_, err := SignIn{Writer: f.session.writer}.Execute(t.Context(), SignInCommand{
			Email: address, Password: secret.New("correct horse battery"),
		})
		if detailOf(err) != "auth.password_not_offered" {
			t.Errorf("signing in as %s answered %v, want auth.password_not_offered", address, err)
		}
	}
	if len(f.session.sessions.inserted) != 0 {
		t.Error("a switched-off password opened a session")
	}

	// Switched back on, the stored password works again: nothing was taken from the account.
	f.passwords.workspace.row.Settings.SignIn.Methods = nil
	if result := f.signsIn(t, "correct horse battery"); result.Pair == nil && result.Challenge == nil {
		t.Error("the password switched back on did not sign in")
	}
}

func TestTheResetPointsToTheProviderWhereThePasswordIsOff(t *testing.T) {
	f := newResetFixture(now)
	passwordOff(f.passwordFixture)

	link, err := MintResetToken{Writer: f.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if link.HasPassword || !link.Token.IsEmpty() {
		t.Errorf("a workspace without the password was mailed a password link: %+v", link)
	}
	if link.Address != "bert@example.org" {
		t.Errorf("the mail goes to %q, want the account's own address", link.Address)
	}
}

func TestAResetLinkIsRefusedOnceThePasswordIsOff(t *testing.T) {
	f := newResetFixture(now)
	token := f.mintedFor(t, now, domain.PendingReset)
	passwordOff(f.passwordFixture)

	_, err := ResetPassword{Writer: f.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: secret.New(token), Password: secret.New("a brand new passphrase"),
	})
	if detailOf(err) != "auth.password_not_offered" {
		t.Fatalf("a reset in a workspace without the password answered %v", err)
	}
	if len(f.accounts.writes) != 0 {
		t.Error("the refused reset wrote a password")
	}
}

func TestAnInvitationIsNotRedeemedWithAPasswordWhereThePasswordIsOff(t *testing.T) {
	f := newStepFixture(now)
	passwordOff(f.passwords)
	token := redemptionToken(t)
	f.session.accounts.redemption[token.Secret()] = waitingRedemption()
	passwords := f.passwords.writer

	_, err := RedeemInvitation{Writer: f.session.writer, Passwords: &passwords}.Execute(t.Context(),
		RedeemInvitationCommand{Token: secret.New(token.Secret()), Password: secret.New("a long first password")})
	if detailOf(err) != "auth.password_not_offered" {
		t.Fatalf("redeeming with a password answered %v", err)
	}
	if f.session.accounts.redeemedHash != "" {
		t.Error("the refused redemption stored a password")
	}
}
