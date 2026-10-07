// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// UC-ID-12 check 6: where a workspace has switched the password off, the server refuses it at every
// door - not only the sign-in card. The refusal is the same for every address, stored passwords are
// kept, and ADR-0076 §4's fallback is the one exception (PasswordFallback_test).

// passwordOff switches the workspace the fixture resolves to providers only - and switches its own
// provider on, which is what the last-way-in guard demands of the screen. Without it the workspace has
// no way in that works, and the password opens as the fallback (E2).
func passwordOff(f *passwordFixture) {
	methods := []string{domain.MethodOidc}
	f.workspace.row.Settings.SignIn.Methods = &methods
	f.providers.rows = append(f.providers.rows, domain.IdentityProvider{
		ID: rulesProviderRow, TenantID: tenant, Kind: domain.KindGeneric,
		DisplayName: "id.acme.example", Issuer: "https://id.acme.example", Enabled: true,
	})
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

// The step-up neither offers nor takes a password the workspace switched off: an account signed in
// through its provider proves itself there, or with its factor (ADR-0075).
func TestTheStepUpTakesNoPasswordWhereThePasswordIsOff(t *testing.T) {
	f := newStepFixture(now)
	f.session.writer.StepUps = newStepUps()
	credential, _ := refreshCredential(now)
	f.session.sessions.sessions[sessionRowID] = repository.SessionCredential{
		TenantStatus: domain.TenantActive, Session: credential.Session, Account: credential.Account,
	}
	f.session.withAccount("bert@example.org", "correct horse battery")
	passwordOff(f.passwords)

	_, err := StepUp{Writer: f.session.writer}.Execute(t.Context(), signedInActor(),
		StepUpCommand{Password: secret.New("correct horse battery")})
	if detailOf(err) != "auth.password_not_offered" {
		t.Fatalf("a password step-up in a workspace without it answered %v", err)
	}
	methods, err := StepUpVerifier{Writer: f.session.writer}.Methods(t.Context(), tenant, account)
	if err != nil {
		t.Fatalf("reading the methods: %v", err)
	}
	for _, method := range methods {
		if method == stepupport.MethodPassword {
			t.Error("the step-up offers a password the workspace switched off")
		}
	}
}

// A sign-in that was owed a new password started with the password; once the password is off, the
// step that sets the new one and signs in is refused like every other password door.
func TestThePasswordChangeStepIsRefusedOnceThePasswordIsOff(t *testing.T) {
	f := newStepFixture(now)
	f.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(30)}
	challenge := f.signsIn(t, "correct horse battery").Challenge
	if challenge == nil {
		t.Fatal("no change step to walk")
	}
	passwordOff(f.passwords)

	_, err := SetPasswordAndSignIn{Writer: f.passwords.writer}.Execute(t.Context(), SetPasswordAndSignInCommand{
		PendingToken: challenge.Token, Password: secret.New("a much longer passphrase than thirty characters"),
	})
	if detailOf(err) != "auth.password_not_offered" {
		t.Fatalf("the change step answered %v", err)
	}
}

// ADR-0076 §4's fallback is the one exception at every door, not only at the sign-in: where the
// workspace's last way in was an offer that ended, the reset mails its link and an invitation is
// redeemed with a password.
func TestTheFallbackOpensTheResetAndTheInvitationToo(t *testing.T) {
	f := fallbackFixture(t)

	link, err := MintResetToken{Writer: f.passwords.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if !link.HasPassword || link.Token.IsEmpty() {
		t.Errorf("under the fallback the reset mailed no link: %+v", link)
	}

	token := redemptionToken(t)
	f.session.accounts.redemption[token.Secret()] = waitingRedemption()
	passwords := f.passwords.writer
	_, err = RedeemInvitation{Writer: f.session.writer, Passwords: &passwords}.Execute(t.Context(),
		RedeemInvitationCommand{Token: secret.New(token.Secret()), Password: secret.New("seven blue lanterns over the harbour")})
	if detailOf(err) == "auth.password_not_offered" {
		t.Errorf("under the fallback the invitation refused the password: %v", err)
	}
}
