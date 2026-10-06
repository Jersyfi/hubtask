// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// SC-33 (#1140, ADR-0078 §1, UC-ID-04 check 8): in a workspace that switched the password off,
// *Forgot your password?* mails an account that no provider switched on there lets in a link to
// connect one - instead of the provider mail that would point to nothing. An account a provider
// there does let in keeps that mail; where the password is open, SC-25's links stand unchanged.

// passwordOffWith switches the password off in the step fixture with the providers given in force -
// at least one of them on, so that the password is not open as the fallback - and connects the
// account to the providers named.
func passwordOffWith(f *stepFixture, rows []domain.IdentityProvider, connected ...shared.ID) {
	methods := []string{domain.MethodOidc}
	f.passwords.workspace.row.Settings.SignIn.Methods = &methods
	store := rulesProviders(rows...)
	f.passwords.writer.WaysIn.Providers = store
	f.passwords.writer.Session.StepUpProviders = ProviderStepUps{
		Providers: store, External: connectedTo{providers: connected}, Workspaces: f.passwords.workspace,
	}
}

// mintFor is MintResetToken on the fixture's password writer.
func mintFor(t *testing.T, f *stepFixture) ResetLink {
	t.Helper()
	link, err := MintResetToken{Writer: f.passwords.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	return link
}

// storedFor answers the pending credential a minted link names.
func storedFor(t *testing.T, f *stepFixture, link ResetLink) domain.PendingCredential {
	t.Helper()
	lookup, found := f.pending.rows[link.Token.Reveal()]
	if !found {
		t.Fatalf("the minted link %q names no stored credential", link.Token.Reveal())
	}
	return lookup.Credential
}

func TestAnAccountNoProviderLetsInIsMailedAConnectLinkWhereThePasswordIsOff(t *testing.T) {
	ended := offeredUntil(now.Add(-time.Hour))
	cases := map[string]func(*stepFixture){
		// (a) a password, never connected to the workspace's provider.
		"a password holder never connected": func(f *stepFixture) {
			passwordOffWith(f, []domain.IdentityProvider{ownProvider})
		},
		// (b) no password, its only identity at an installation offer that ended.
		"an identity only at an ended offer": func(f *stepFixture) {
			withoutPassword(f)
			passwordOffWith(f, []domain.IdentityProvider{ended, ownProvider}, fallbackRow)
		},
		// (c) no credential at all - its provider removed, its identity gone with it.
		"no credential at all": func(f *stepFixture) {
			withoutPassword(f)
			passwordOffWith(f, []domain.IdentityProvider{ownProvider})
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			f := newStepFixture(now)
			arrange(f)

			link := mintFor(t, f)
			if !link.Connect || link.HasPassword || link.First || link.Token.IsEmpty() {
				t.Fatalf("mailed %+v, want a link to connect the provider", link)
			}
			stored := storedFor(t, f, link)
			if stored.Purpose != domain.PendingConnect || stored.AccountID != account ||
				!stored.ExpiresAt.Equal(now.Add(domain.ResetLifetime)) {
				t.Errorf("the link is stored as %+v, want a CONNECT credential for thirty minutes", stored)
			}
		})
	}
}

// An account a provider switched on here lets in keeps the mail that points to it - with or without
// a password, which is not a way in here.
func TestAnAccountAProviderLetsInKeepsTheProviderMail(t *testing.T) {
	for name, arrange := range map[string]func(*stepFixture){
		"without a password": withoutPassword,
		"with a password":    func(*stepFixture) {},
	} {
		t.Run(name, func(t *testing.T) {
			f := newStepFixture(now)
			arrange(f)
			passwordOffWith(f, []domain.IdentityProvider{ownProvider}, ownProvider.ID)

			link := mintFor(t, f)
			if link.Connect || link.HasPassword || link.First || !link.Token.IsEmpty() {
				t.Errorf("an account its provider lets in was mailed %+v, want the provider mail", link)
			}
		})
	}
}

// Only an active person is mailed a link to connect: an invited account accepts its invitation
// through the invitation's own link (SC-32), and a service account signs in with a token.
func TestOnlyAnActivePersonIsMailedAConnectLink(t *testing.T) {
	for name, change := range map[string]func(*domain.Account){
		"an invited account": func(held *domain.Account) { held.Status = domain.AccountInvited },
		"a service account":  func(held *domain.Account) { held.Kind = domain.AccountServiceAccount },
	} {
		t.Run(name, func(t *testing.T) {
			f := newStepFixture(now)
			passwordOffWith(f, []domain.IdentityProvider{ownProvider})
			held := f.passwords.accounts.rows[account]
			change(&held.Account)
			f.passwords.accounts.rows[account] = held

			if link := mintFor(t, f); link.Connect || !link.Token.IsEmpty() {
				t.Errorf("%s was mailed %+v", name, link)
			}
		})
	}
}

// Where the password is open - switched on, or open as the fallback - nothing changes: the reset
// link, or SC-25's link to set a first password. A connect link is for the workspace without it.
func TestWhereThePasswordIsOpenTheResetLinksStand(t *testing.T) {
	on := newStepFixture(now)
	on.passwords.writer.Session.StepUpProviders = ProviderStepUps{
		Providers: rulesProviders(ownProvider), External: connectedTo{}, Workspaces: on.passwords.workspace,
	}
	if link := mintFor(t, on); !link.HasPassword || link.Connect || link.First {
		t.Errorf("with the password on a password holder was mailed %+v, want the reset link", link)
	}

	fallback := fallbackFixture(t)
	if link := mintFor(t, fallback); !link.HasPassword || link.Connect {
		t.Errorf("under the fallback a password holder was mailed %+v, want the reset link", link)
	}

	first := fallbackFixture(t)
	withoutPassword(first)
	if link := mintFor(t, first); !link.First || link.Connect {
		t.Errorf("under the fallback an account without a password was mailed %+v, want SC-25's link", link)
	}
}

// A new link spends the account's earlier ones of either kind (UC-ID-04 check 2): whichever mail the
// person opens, only the newest works - and a reset link mailed before the switch is one of them.
func TestANewLinkSpendsTheEarlierResetAndConnectLinks(t *testing.T) {
	f := newStepFixture(now)
	passwordOffWith(f, []domain.IdentityProvider{ownProvider})
	// Every link its own token and row, as the real entropy and identifiers give them.
	f.passwords.writer.Session.Entropy = &countingEntropy{}
	f.passwords.writer.IDs = &idSequence{queue: []shared.ID{
		shared.ID("018f2a1b-0000-7000-8000-00000000cc11"), shared.ID("018f2a1b-0000-7000-8000-00000000cc12"),
	}}

	earlier := mintFor(t, f)
	reset, err := domain.NewPendingToken(tenant, make([]byte, domain.TokenSecretBytes))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if err := f.pending.Insert(t.Context(), domain.PendingCredential{
		ID: resetRowID, TenantID: tenant, AccountID: account, Purpose: domain.PendingReset,
		CreatedAt: now, ExpiresAt: now.Add(domain.ResetLifetime),
	}, reset); err != nil {
		t.Fatalf("inserting: %v", err)
	}

	latest := mintFor(t, f)
	if storedFor(t, f, earlier).Verify(now) == nil {
		t.Error("the earlier connect link still works beside the newest")
	}
	if f.pending.rows[reset.Secret()].Credential.Verify(now) == nil {
		t.Error("a reset link mailed before still works beside the connect link")
	}
	if storedFor(t, f, latest).Verify(now) != nil {
		t.Error("the newest link does not work")
	}
}

// A link is mailed only where a provider switched on here would admit the account bringing its own
// proof - every mode this build knows does, under DOMAINS outside the list too (2026-10-06); a mode a
// newer build wrote does not, and the provider mail stands.
func TestAConnectLinkNeedsAProviderThatAdmitsTheAccountsOwnProof(t *testing.T) {
	for name, mode := range map[string]domain.Provisioning{
		"only these domains": domain.ProvisionDomains,
		"anybody":            domain.ProvisionAny,
		"a newer mode":       "NEWER_MODE",
	} {
		t.Run(name, func(t *testing.T) {
			f := newStepFixture(now)
			row := ownProvider
			row.Provisioning = mode
			row.AllowedEmailDomains = []string{"elsewhere.example"}
			passwordOffWith(f, []domain.IdentityProvider{row})

			link := mintFor(t, f)
			if want := mode != "NEWER_MODE"; link.Connect != want {
				t.Errorf("under %s the mail is %+v, want a connect link: %v", name, link, want)
			}
		})
	}
}
