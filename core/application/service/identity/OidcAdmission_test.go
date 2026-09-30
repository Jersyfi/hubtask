// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
)

// E1, decided 2026-09-30 (docs/backlog/milestone-SC.md): the admission ladder is one axis - who
// comes in - and the three positions read, for a person arriving through the provider:
//
//   - INVITED_ONLY: only somebody who already has an account here; nobody gets one by arriving.
//   - DOMAINS: only somebody from the listed organisations; a newcomer from them gets an account.
//   - ANY: anybody the provider vouches for; a newcomer gets an account.
//
// And under every position an address the provider did not verify comes in nowhere. The table is
// the whole ladder end to end, through the use case rather than the domain predicate, because what
// a person meets is the session or the refusal - not `MayAdmit`.
func TestTheAdmissionLadderIsWhoComesIn(t *testing.T) {
	invited := domain.Account{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e1"), TenantID: tenant,
		Kind: domain.AccountUser, Email: "ada@example.org", DisplayName: "Ada",
		Status: domain.AccountActive,
	}

	type arrival struct {
		name     string
		email    string
		verified bool
	}
	var (
		theInvited      = arrival{"the invited person", "ada@example.org", true}
		aNewcomerInside = arrival{"a newcomer from the listed organisation", "grace@example.org", true}
		aStranger       = arrival{"a stranger from elsewhere", "eve@elsewhere.org", true}
		anUnverified    = arrival{"an address the provider did not verify", "ada@example.org", false}
	)

	const (
		signedIn    = "signed in to the existing account"
		provisioned = "given a new account"
		refused     = "refused"
	)
	cases := []struct {
		mode    domain.Provisioning
		arrival arrival
		want    string
	}{
		{domain.ProvisionInvitedOnly, theInvited, signedIn},
		{domain.ProvisionInvitedOnly, aNewcomerInside, refused},
		{domain.ProvisionInvitedOnly, aStranger, refused},
		{domain.ProvisionInvitedOnly, anUnverified, refused},

		{domain.ProvisionDomains, theInvited, signedIn},
		{domain.ProvisionDomains, aNewcomerInside, provisioned},
		{domain.ProvisionDomains, aStranger, refused},
		{domain.ProvisionDomains, anUnverified, refused},

		{domain.ProvisionAny, theInvited, signedIn},
		{domain.ProvisionAny, aNewcomerInside, provisioned},
		{domain.ProvisionAny, aStranger, provisioned},
		{domain.ProvisionAny, anUnverified, refused},
	}

	for _, tc := range cases {
		t.Run(string(tc.mode)+"/"+tc.arrival.name, func(t *testing.T) {
			f := newOidcFixture(t, now, invited)
			// Set on the stored row rather than through the configuration, which holds a
			// self-hosted issuer to its own rules: this is about what each position admits.
			f.store.rows[0].Provisioning = tc.mode
			f.relying.identity = provider.Identity{
				Subject: "subject-" + tc.arrival.email, Email: tc.arrival.email,
				EmailVerified: tc.arrival.verified, AddressAuthoritative: tc.arrival.verified,
				DisplayName: "Somebody",
			}
			before := len(f.accounts.byID)

			pair, err := CompleteOidcSignIn{Writer: f.writer}.
				Execute(t.Context(), CompleteOidcSignInCommand{Code: "x", State: start(t, f)})

			var got string
			switch {
			case err != nil:
				if !errors.Is(err, shared.ErrForbidden) && !errors.Is(err, shared.ErrUnauthenticated) {
					t.Fatalf("refused with an unexpected error: %v", err)
				}
				got = refused
			case len(f.accounts.byID) > before:
				got = provisioned
			case pair.Session.AccountID == invited.ID:
				got = signedIn
			default:
				t.Fatalf("a session for %s, which is neither the invited account nor a new one", pair.Session.AccountID)
			}
			if got != tc.want {
				t.Errorf("%s under %s was %s, want %s", tc.arrival.name, tc.mode, got, tc.want)
			}
			if got == refused && len(f.external.links) != 0 {
				t.Errorf("a refused arrival still linked %v", f.external.links)
			}
		})
	}
}
