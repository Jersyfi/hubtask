// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// An invited person accepts the invitation through the workspace's provider (SC-24) - and only with
// a second proof (SC-32, ADR-0078 §1): the person arrives through the invitation's own link, bound
// to the provider flow on the server, or the provider is authoritative for the address. A
// provider's `email_verified` alone activates nothing and connects nothing. Under INVITED_ONLY the
// link admits even a provider that is not authoritative, for that account only, and in every case
// the provider's verified address must be the invited one.
//
// The fixture's provider is a self-hosted issuer at login.example.org: verified, never
// authoritative.

func invitedAda() domain.Account {
	return domain.Account{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e1"), TenantID: tenant,
		Kind: domain.AccountUser, Email: "ada@example.org", DisplayName: "Ada",
		Status: domain.AccountInvited,
	}
}

// adaAtTheProvider is the invited person signing in at a provider that verified the address and
// is not authoritative for it.
func adaAtTheProvider() provider.Identity {
	return provider.Identity{
		Subject: "provider-subject-1", Email: "ada@example.org", EmailVerified: true, DisplayName: "Ada",
	}
}

// invitedFixture is the fixture with Ada invited, the provider in one mode, and her invitation's
// token minted for the card.
func invitedFixture(t *testing.T, mode domain.Provisioning) (*oidcFixture, secret.Secret) {
	t.Helper()
	at := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	invited := invitedAda()
	f := newOidcFixture(t, at, invited)
	f.store.rows[0].Provisioning = mode
	f.relying.identity = adaAtTheProvider()
	return f, invitationFor(t, f, invited, tenant, at.Add(14*24*time.Hour), 51)
}

// complete runs the second half of a flow.
func complete(t *testing.T, f *oidcFixture, state secret.Secret) (SignInResult, error) {
	t.Helper()
	return CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "the-code", State: state,
	})
}

// assertStillInvited is the acceptance's "nothing linked, nothing activated", and the invitation
// still waiting for the person.
func assertStillInvited(t *testing.T, f *oidcFixture, invitation secret.Secret) {
	t.Helper()
	invited := invitedAda()
	if status := f.accounts.byID[invited.ID].Status; status != domain.AccountInvited {
		t.Errorf("the account is %s, want it still INVITED", status)
	}
	if len(f.external.links) != 0 {
		t.Errorf("the refused arrival connected %v", f.external.links)
	}
	if containsAction(auditActions(f.session.audit.entries), InvitationRedeemedAction) {
		t.Error("the refused arrival recorded the invitation as redeemed")
	}
	if _, held := f.session.accounts.redemption[invitation.Reveal()]; !held {
		t.Error("the refused arrival spent the invitation")
	}
}

// The acceptance, first half: with the link it is accepted - under the permissive mode, under a
// self-hosted provider's domains, and under INVITED_ONLY from a provider that is not authoritative.
func TestAnInvitedPersonAcceptsTheInvitationThroughItsLink(t *testing.T) {
	for _, mode := range []domain.Provisioning{
		domain.ProvisionAny, domain.ProvisionDomains, domain.ProvisionInvitedOnly,
	} {
		t.Run(string(mode), func(t *testing.T) {
			f, invitation := invitedFixture(t, mode)
			invited := invitedAda()

			result, err := complete(t, f, startInvited(t, f, invitation))
			if err != nil {
				t.Fatalf("the arrival through the link answered %v", err)
			}
			if pair := pairOf(result); pair.Session.AccountID != invited.ID {
				t.Fatalf("the session belongs to %q, want the invited account", pair.Session.AccountID)
			}
			if status := f.accounts.byID[invited.ID].Status; status != domain.AccountActive {
				t.Errorf("the account is %s, want ACTIVE", status)
			}
			if len(f.external.links) != 1 || f.external.links[0] != "provider-subject-1" {
				t.Errorf("the account is connected as %v", f.external.links)
			}
			actions := auditActions(f.session.audit.entries)
			if !containsAction(actions, InvitationRedeemedAction) || !containsAction(actions, OidcLinkedAction) {
				t.Errorf("the trail holds %v, want the redemption and the connection", actions)
			}
		})
	}
}

// The acceptance, second half: under the permissive modes an arrival without the link is refused,
// links nothing and activates nothing, and the refusal is in the trail. Under INVITED_ONLY the
// provider is not even admitted - and the person is pointed at the link all the same.
func TestAnArrivalWithoutTheLinkActivatesNothing(t *testing.T) {
	for _, mode := range []domain.Provisioning{
		domain.ProvisionAny, domain.ProvisionDomains, domain.ProvisionInvitedOnly,
	} {
		t.Run(string(mode), func(t *testing.T) {
			f, invitation := invitedFixture(t, mode)

			_, err := complete(t, f, start(t, f))
			if detailOf(err) != "identity_provider.invitation_needs_link" {
				t.Fatalf("the arrival without the link answered %v", err)
			}
			assertStillInvited(t, f, invitation)
			if !containsAction(auditActions(f.session.audit.entries), OidcRefusedAction) {
				t.Error("the refusal is not in the trail")
			}
		})
	}
}

// The other second proof: a provider authoritative for the address activates the account without
// the link, in the permissive mode and in the one that needs authority to admit at all.
func TestAnAuthoritativeProviderActivatesWithoutTheLink(t *testing.T) {
	for _, mode := range []domain.Provisioning{domain.ProvisionAny, domain.ProvisionInvitedOnly} {
		t.Run(string(mode), func(t *testing.T) {
			f, _ := invitedFixture(t, mode)
			f.relying.identity.AddressAuthoritative = true

			result, err := complete(t, f, start(t, f))
			if err != nil {
				t.Fatalf("the authoritative arrival answered %v", err)
			}
			if pairOf(result).Session.AccountID != invitedAda().ID ||
				f.accounts.byID[invitedAda().ID].Status != domain.AccountActive {
				t.Error("the authoritative arrival did not activate the invited account")
			}
		})
	}
}

// The provider's verified address has to be the invited one, through the link as well: a person
// signed in there as somebody else is refused, and the invitation is still theirs to accept.
func TestAMismatchedAddressIsRefusedAndTheInvitationWaits(t *testing.T) {
	f, invitation := invitedFixture(t, domain.ProvisionInvitedOnly)
	f.relying.identity.Email = "eve@example.org"
	f.relying.identity.Subject = "provider-subject-eve"

	_, err := complete(t, f, startInvited(t, f, invitation))
	if detailOf(err) != "identity_provider.invitation_address_differs" {
		t.Fatalf("a mismatched address answered %v", err)
	}
	assertStillInvited(t, f, invitation)
	if !containsAction(auditActions(f.session.audit.entries), OidcRefusedAction) {
		t.Error("the refusal is not in the trail")
	}

	// Not spent: the same invitation, with the invited address this time, is accepted.
	f.relying.identity = adaAtTheProvider()
	if _, err := complete(t, f, startInvited(t, f, invitation)); err != nil {
		t.Fatalf("the invitation did not survive the mismatched arrival: %v", err)
	}
	if f.accounts.byID[invitedAda().ID].Status != domain.AccountActive {
		t.Error("the second arrival did not accept the invitation")
	}
}

// A different spelling of the same address is the same address: case is not a second person.
func TestTheInvitedAddressIsComparedAsAnAddress(t *testing.T) {
	f, invitation := invitedFixture(t, domain.ProvisionInvitedOnly)
	f.relying.identity.Email = "Ada@Example.ORG"

	if _, err := complete(t, f, startInvited(t, f, invitation)); err != nil {
		t.Fatalf("the invited address in capitals answered %v", err)
	}
}

// An address the provider did not verify is not admitted through the link either.
func TestTheLinkDoesNotAdmitAnUnverifiedAddress(t *testing.T) {
	f, invitation := invitedFixture(t, domain.ProvisionInvitedOnly)
	f.relying.identity.EmailVerified = false

	_, err := complete(t, f, startInvited(t, f, invitation))
	if detailOf(err) != "identity_provider.not_admitted" {
		t.Fatalf("an unverified address through the link answered %v", err)
	}
	assertStillInvited(t, f, invitation)
}

// An invitation accepted some other way, or run out, between the start and the return is the
// redemption's one refusal - and an arrival that fails spends nothing.
func TestAnInvitationThatLapsedDuringTheFlowIsOneRefusal(t *testing.T) {
	t.Run("accepted meanwhile", func(t *testing.T) {
		f, invitation := invitedFixture(t, domain.ProvisionAny)
		state := startInvited(t, f, invitation)
		accepted := invitedAda()
		accepted.Status = domain.AccountActive
		f.accounts.byID[accepted.ID] = accepted

		_, err := complete(t, f, state)
		if detailOf(err) != "auth.redemption_failed" {
			t.Fatalf("an invitation accepted meanwhile answered %v", err)
		}
		if len(f.external.links) != 0 {
			t.Errorf("the refused arrival connected %v", f.external.links)
		}
	})
	t.Run("ran out", func(t *testing.T) {
		f, invitation := invitedFixture(t, domain.ProvisionAny)
		state := startInvited(t, f, invitation)
		f.accounts.lapsed = map[shared.ID]bool{invitedAda().ID: true}

		_, err := complete(t, f, state)
		if detailOf(err) != "auth.redemption_failed" {
			t.Fatalf("an invitation that ran out answered %v", err)
		}
		assertStillInvited(t, f, invitation)
	})
}

// Without the link, an invitation that ran out is named as such: an authoritative provider's word
// does not renew the operator's offer.
func TestALapsedInvitationIsNotAcceptedThroughAnAuthoritativeProvider(t *testing.T) {
	f, invitation := invitedFixture(t, domain.ProvisionAny)
	f.relying.identity.AddressAuthoritative = true
	f.accounts.lapsed = map[shared.ID]bool{invitedAda().ID: true}

	_, err := complete(t, f, start(t, f))
	if detailOf(err) != "identity_provider.invitation_lapsed" {
		t.Fatalf("a lapsed invitation answered %v", err)
	}
	assertStillInvited(t, f, invitation)
}

// A connection made earlier while the account was invited, without a second proof - SC-24's
// arrivals, or an earlier provider's word - activates nothing on the next arrival through it. Through
// the link it does, and the connection is made afresh: what was connected before the proof is
// dropped in the activation's transaction.
func TestAnEarlierLinkWithoutProofActivatesNothing(t *testing.T) {
	f, invitation := invitedFixture(t, domain.ProvisionAny)
	invited := invitedAda()
	f.external.bySubject[linkKey(oidcProviderRow, "provider-subject-1")] = invited

	_, err := complete(t, f, start(t, f))
	if detailOf(err) != "identity_provider.invitation_needs_link" {
		t.Fatalf("the connected invited arrival answered %v", err)
	}
	if f.accounts.byID[invited.ID].Status != domain.AccountInvited {
		t.Fatal("a link made without a second proof activated the account")
	}

	result, err := complete(t, f, startInvited(t, f, invitation))
	if err != nil {
		t.Fatalf("the arrival through the link answered %v", err)
	}
	if pairOf(result).Session.AccountID != invited.ID || f.accounts.byID[invited.ID].Status != domain.AccountActive {
		t.Error("the arrival through the link did not activate the account")
	}
	if len(f.external.unlinked) != 1 || f.external.unlinked[0] != invited.ID {
		t.Errorf("the earlier connections were dropped for %v, want the invited account", f.external.unlinked)
	}
	if len(f.external.links) != 1 || f.external.links[0] != "provider-subject-1" {
		t.Errorf("the account is connected as %v, want the arriving subject afresh", f.external.links)
	}
}

// And an earlier connection through another provider is not a credential that blocks the second
// proof: it is dropped, and the provider that brought the proof is the one connected.
func TestAnEarlierLinkAtAnotherProviderIsDroppedOnActivation(t *testing.T) {
	f, _ := invitedFixture(t, domain.ProvisionAny)
	f.relying.identity.AddressAuthoritative = true
	invited := invitedAda()
	elsewhere := shared.ID("01936f2a-7c1e-7000-8000-0000000000a9")
	f.external.bySubject[linkKey(elsewhere, "an-earlier-subject")] = invited

	if _, err := complete(t, f, start(t, f)); err != nil {
		t.Fatalf("the authoritative arrival answered %v", err)
	}
	if _, held := f.external.bySubject[linkKey(elsewhere, "an-earlier-subject")]; held {
		t.Error("the connection made without a proof survived the activation")
	}
	if _, held := f.external.bySubject[linkKey(oidcProviderRow, "provider-subject-1")]; !held {
		t.Error("the provider that brought the proof was not connected")
	}
	actions := auditActions(f.session.audit.entries)
	if !containsAction(actions, InvitationRedeemedAction) || !containsAction(actions, OidcLinkedAction) {
		t.Errorf("the trail holds %v", actions)
	}
}

// An invitation's flow finishes that invitation: an identity already connected to somebody else here
// is not the invited person, and signs nobody in through it.
func TestAnInvitationsFlowDoesNotSignInSomebodyElse(t *testing.T) {
	f, invitation := invitedFixture(t, domain.ProvisionAny)
	bert := domain.Account{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e2"), TenantID: tenant,
		Kind: domain.AccountUser, Email: "bert@example.org", DisplayName: "Bert",
		Status: domain.AccountActive,
	}
	f.external.bySubject[linkKey(oidcProviderRow, "provider-subject-1")] = bert

	_, err := complete(t, f, startInvited(t, f, invitation))
	if detailOf(err) != "identity_provider.invitation_address_differs" {
		t.Fatalf("somebody else's identity through the invitation answered %v", err)
	}
	assertStillInvited(t, f, invitation)
}
