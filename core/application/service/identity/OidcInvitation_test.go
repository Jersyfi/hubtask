// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// An invited person accepts the invitation through the workspace's provider (SC-24). The account
// holds no credential yet, so the provider is connected at once (E2) - and the account becomes
// ACTIVE in the same act, as redeeming the invitation would make it. Before, it was connected and
// left INVITED, and the arrival was refused as an account that may not act: in a workspace that
// switched the password off, an invited person had no way in at all (the owner's rule: nobody is
// locked out).

func invitedAda() domain.Account {
	return domain.Account{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e1"), TenantID: tenant,
		Kind: domain.AccountUser, Email: "ada@example.org", DisplayName: "Ada",
		Status: domain.AccountInvited,
	}
}

func TestAnInvitedPersonAcceptsTheInvitationThroughTheProvider(t *testing.T) {
	at := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	invited := invitedAda()
	f := newOidcFixture(t, at, invited)

	result, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "the-code", State: start(t, f),
	})
	if err != nil {
		t.Fatalf("the invited arrival answered %v", err)
	}
	if pair := pairOf(result); pair.Session.AccountID != invited.ID {
		t.Fatalf("the session belongs to %q, want the invited account", pair.Session.AccountID)
	}
	if status := f.accounts.byID[invited.ID].Status; status != domain.AccountActive {
		t.Errorf("the account is %s, want ACTIVE", status)
	}
	actions := auditActions(f.session.audit.entries)
	if !containsAction(actions, InvitationRedeemedAction) || !containsAction(actions, OidcLinkedAction) {
		t.Errorf("the trail holds %v, want the redemption and the connection", actions)
	}
}

// An invitation that ran out is not accepted this way either: the operator's offer lapsed, and a
// provider's word does not renew it.
func TestALapsedInvitationIsNotAcceptedThroughTheProvider(t *testing.T) {
	at := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	invited := invitedAda()
	f := newOidcFixture(t, at, invited)
	f.accounts.lapsed = map[shared.ID]bool{invited.ID: true}

	_, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "the-code", State: start(t, f),
	})
	if detailOf(err) != "identity_provider.invitation_lapsed" {
		t.Fatalf("a lapsed invitation answered %v", err)
	}
	if f.accounts.byID[invited.ID].Status != domain.AccountInvited || len(f.external.links) != 0 {
		t.Error("a lapsed invitation was accepted or connected")
	}
}

// An invited account the provider connected before this fix - connected, then refused as not active
// - is accepted on its next arrival through the same subject, rather than refused forever.
func TestAnInvitedAccountConnectedBeforeIsAcceptedOnItsNextArrival(t *testing.T) {
	at := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	invited := invitedAda()
	f := newOidcFixture(t, at, invited)
	f.external.bySubject[linkKey(oidcProviderRow, "provider-subject-1")] = invited

	result, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "the-code", State: start(t, f),
	})
	if err != nil {
		t.Fatalf("the connected invited arrival answered %v", err)
	}
	if pairOf(result).Session.AccountID != invited.ID || f.accounts.byID[invited.ID].Status != domain.AccountActive {
		t.Errorf("the account is %s, want it signed in and ACTIVE", f.accounts.byID[invited.ID].Status)
	}
}
