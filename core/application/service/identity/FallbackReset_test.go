// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

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

// Outside the fallback the same account gets the mail that points to its provider - whether the
// workspace keeps the password on or switched it off.
func TestAProviderOnlyAccountOutsideTheFallbackGetsTheProviderMail(t *testing.T) {
	for _, c := range []struct {
		name    string
		methods []string
	}{
		{"the password on", []string{domain.MethodDirect, domain.MethodOidc}},
		{"the password off and a provider in force", []string{domain.MethodOidc}},
	} {
		f := fallbackFixture(t)
		withoutPassword(f)
		methods := c.methods
		f.passwords.workspace.row.Settings.SignIn.Methods = &methods
		if c.name != "the password on" {
			// A way in that still works: the workspace's own provider, so nothing has ended.
			f.passwords.writer.WaysIn = WaysIn{}
		}

		link, err := MintResetToken{Writer: f.passwords.writer}.MintResetToken(t.Context(), tenant, account)
		if err != nil {
			t.Fatalf("%s: minting: %v", c.name, err)
		}
		if link.First || link.HasPassword || !link.Token.IsEmpty() {
			t.Errorf("%s: a provider-only account was mailed %+v, want the provider mail", c.name, link)
		}
	}
}

// A link minted under the fallback sets a first password only while the fallback stands: once an
// administrator switched another way on, it is refused like any spent link, and nothing is written.
func TestAFirstPasswordLinkIsRefusedOnceTheFallbackEnded(t *testing.T) {
	f := fallbackFixture(t)
	withoutPassword(f)
	link, err := MintResetToken{Writer: f.passwords.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil || link.Token.IsEmpty() {
		t.Fatalf("minting: %+v (%v)", link, err)
	}
	f.passwords.workspace.row.Settings.SignIn.Methods = &[]string{domain.MethodDirect, domain.MethodOidc}

	_, err = ResetPassword{Writer: f.passwords.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: link.Token, Password: secret.New("seven blue lanterns over the harbour"),
	})
	if detailOf(err) != "auth.reset_failed" {
		t.Fatalf("a first-password link after the fallback answered %v, want auth.reset_failed", err)
	}
	if len(f.passwords.accounts.writes) != 0 {
		t.Error("the refused link wrote a password")
	}
}
