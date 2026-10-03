// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// SC-25 (#1122, ADR-0077 §3): nobody is left without a way in. Where a workspace's last way in was an
// offer that ended, an account that only ever signed in through that provider holds no password the
// fallback could open - so *Forgot your password?* mails it a link to set one, and that is how it gets
// back in. Outside the fallback nothing changes: the provider mail, no link.

// withoutPassword empties the account's password in the password fixture's rows.
func withoutPassword(f *stepFixture) {
	held := f.passwords.accounts.rows[account]
	held.PasswordHash = secret.Secret{}
	f.passwords.accounts.rows[account] = held
}

func TestAProviderOnlyAccountSetsAPasswordUnderTheFallbackAndIsSignedIn(t *testing.T) {
	f := fallbackFixture(t)
	withoutPassword(f)

	link, err := MintResetToken{Writer: f.passwords.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if !link.First || link.HasPassword || link.Token.IsEmpty() {
		t.Fatalf("under the fallback a provider-only account was mailed %+v, want a link to set a first password", link)
	}

	result, err := ResetPassword{Writer: f.passwords.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: link.Token, Password: secret.New("seven blue lanterns over the harbour"),
	})
	if err != nil {
		t.Fatalf("setting the first password: %v", err)
	}
	if result.Pair == nil {
		t.Fatalf("setting the first password did not sign in: %+v", result)
	}
	if len(f.passwords.accounts.writes) != 1 {
		t.Errorf("%d password writes, want one", len(f.passwords.accounts.writes))
	}
	var recorded bool
	for _, entry := range append(f.passwords.audit.entries, f.session.audit.entries...) {
		recorded = recorded || entry.Action == PasswordFallbackAction
	}
	if !recorded {
		t.Error("a session the fallback opened through the reset left no fallback entry in the trail")
	}
}

// connectedTo is the one question the reset asks the external accounts: which providers an
// account is connected to. Everything else the interface carries is the sign-in's.
type connectedTo struct {
	repository.ExternalAccounts
	providers []shared.ID
}

func (c connectedTo) ProvidersOf(context.Context, shared.ID) ([]shared.ID, error) {
	return c.providers, nil
}

// ownProvider is a workspace's own provider, switched on.
var ownProvider = domain.IdentityProvider{
	ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e6"), TenantID: tenant, Kind: domain.KindGeneric,
	DisplayName: "Our directory", Issuer: "https://login.example.org", Enabled: true,
}

// passwordOnWith keeps the password on beside the ended offer, with the providers given in force,
// and connects the account to the providers named.
func passwordOnWith(f *stepFixture, rows []domain.IdentityProvider, connected ...shared.ID) {
	f.passwords.workspace.row.Settings.SignIn.Methods = &[]string{domain.MethodDirect, domain.MethodOidc}
	store := rulesProviders(rows...)
	f.passwords.writer.WaysIn.Providers = store
	f.passwords.writer.Session.StepUpProviders = ProviderStepUps{
		Providers: store, External: connectedTo{providers: connected}, Workspaces: f.passwords.workspace,
	}
}

// A provider that still lets the account in is where the mail points, as before - whether the
// workspace keeps the password on or has switched it off.
func TestAProviderOnlyAccountWhoseProviderWorksGetsTheProviderMail(t *testing.T) {
	ended := offeredUntil(now.Add(-time.Hour))

	on := fallbackFixture(t)
	withoutPassword(on)
	passwordOnWith(on, []domain.IdentityProvider{ended, ownProvider}, ownProvider.ID)

	// Password off, the workspace's own provider in force: no fallback, and the password is closed.
	off := fallbackFixture(t)
	withoutPassword(off)
	off.passwords.writer.WaysIn.Providers = rulesProviders(ended, ownProvider)

	for name, f := range map[string]*stepFixture{"the password on": on, "the password off": off} {
		link, err := MintResetToken{Writer: f.passwords.writer}.MintResetToken(t.Context(), tenant, account)
		if err != nil {
			t.Fatalf("%s: minting: %v", name, err)
		}
		if link.First || link.HasPassword || !link.Token.IsEmpty() {
			t.Errorf("%s: a provider-only account whose provider works was mailed %+v", name, link)
		}
	}
}

// ADR-0077 §4: the password is on, but the one provider this account is connected to has ended.
// No fallback is needed for the workspace - its password works - and still the account is let in.
func TestAProviderOnlyAccountWhoseProviderEndedSetsAPasswordWhereThePasswordIsOn(t *testing.T) {
	f := fallbackFixture(t)
	withoutPassword(f)
	passwordOnWith(f, []domain.IdentityProvider{offeredUntil(now.Add(-time.Hour)), ownProvider}, fallbackRow)

	link, err := MintResetToken{Writer: f.passwords.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if !link.First || link.Token.IsEmpty() {
		t.Fatalf("an account whose provider ended was mailed %+v, want a link to set a password", link)
	}
	result, err := ResetPassword{Writer: f.passwords.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: link.Token, Password: secret.New("seven blue lanterns over the harbour"),
	})
	if err != nil || result.Pair == nil {
		t.Fatalf("setting the first password answered %+v (%v)", result, err)
	}
	for _, entry := range append(f.passwords.audit.entries, f.session.audit.entries...) {
		if entry.Action == PasswordFallbackAction {
			t.Error("a workspace whose password is on recorded a fallback")
		}
	}
}

// UC-ID-04 checks 5 and 6 on this path: an account with an armed factor gets the code step, not a
// session, and every session it had ends.
func TestAFirstPasswordUnderTheFallbackStillAsksForTheFactorAndEndsTheSessions(t *testing.T) {
	f := fallbackFixture(t)
	withoutPassword(f)
	armed := newEnrollments()
	armed.rows[account] = &repository.MfaEnrollment{AccountID: account, ConfirmedAt: now.Add(-time.Hour)}
	f.passwords.writer.Session.Enrollments = armed

	link, err := MintResetToken{Writer: f.passwords.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil || !link.First {
		t.Fatalf("minting: %+v (%v)", link, err)
	}
	result, err := ResetPassword{Writer: f.passwords.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: link.Token, Password: secret.New("seven blue lanterns over the harbour"),
	})
	if err != nil {
		t.Fatalf("setting the first password: %v", err)
	}
	if result.Pair != nil || result.Challenge == nil {
		t.Fatalf("an armed account was answered %+v, want the code step", result)
	}
	// Every session, none spared: the person asking has none of their own to keep (UC-ID-04 check 6).
	if kept := append(f.passwords.sessions.keptByOthers, f.session.sessions.keptByOthers...); len(kept) != 1 || !kept[0].IsZero() {
		t.Errorf("the sessions ended sparing %v, want none spared", kept)
	}
}

// A first-password link sets one only while no provider lets the account in: once the withdrawal is
// cancelled and its provider is a way in again, the link is refused like a spent one, and nothing
// is written. Switching the password back on does not end it - that gives this account no way in.
func TestAFirstPasswordLinkIsRefusedOnceItsProviderIsBack(t *testing.T) {
	f := fallbackFixture(t)
	withoutPassword(f)
	link, err := MintResetToken{Writer: f.passwords.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil || link.Token.IsEmpty() {
		t.Fatalf("minting: %+v (%v)", link, err)
	}
	// The withdrawal cancelled: the offer runs again, and the account is connected to it.
	passwordOnWith(f, []domain.IdentityProvider{offeredUntil(time.Time{})}, fallbackRow)

	_, err = ResetPassword{Writer: f.passwords.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: link.Token, Password: secret.New("seven blue lanterns over the harbour"),
	})
	if detailOf(err) != "auth.reset_failed" {
		t.Fatalf("a first-password link with the provider back answered %v, want auth.reset_failed", err)
	}
	if len(f.passwords.accounts.writes) != 0 {
		t.Error("the refused link wrote a password")
	}
}

// Every session the fallback opens is in the workspace's trail - an invitation redeemed with a
// password under it too, as the sign-in's and the reset's are.
func TestAnInvitationRedeemedUnderTheFallbackIsRecorded(t *testing.T) {
	f := fallbackFixture(t)
	token := redemptionToken(t)
	f.session.accounts.redemption[token.Secret()] = waitingRedemption()
	passwords := f.passwords.writer

	if _, err := (RedeemInvitation{Writer: f.session.writer, Passwords: &passwords}).Execute(t.Context(),
		RedeemInvitationCommand{Token: secret.New(token.Secret()), Password: secret.New("seven blue lanterns over the harbour")}); err != nil {
		t.Fatalf("redeeming under the fallback: %v", err)
	}
	var recorded bool
	for _, entry := range f.session.audit.entries {
		recorded = recorded || entry.Action == PasswordFallbackAction
	}
	if !recorded {
		t.Errorf("the redemption left no fallback entry: %v", f.session.audit.entries)
	}
}
