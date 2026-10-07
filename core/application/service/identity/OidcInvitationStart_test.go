// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// A sign-in begun on the invitation card carries the invitation, and the flow remembers the account
// it invites (ADR-0078 §1). The token is checked without being spent; an invitation that cannot be
// redeemed is refused before the browser leaves, with the one sentence the redemption answers.

// invitationFor mints an invitation for an invited account and stores it as the redemption lookup
// answers it. Expiry relative to the fixture's clock.
func invitationFor(
	t *testing.T, f *oidcFixture, invited domain.Account, tenantID shared.ID, expiresAt time.Time, seed byte,
) secret.Secret {
	t.Helper()
	material := make([]byte, domain.TokenSecretBytes)
	for i := range material {
		material[i] = seed + byte(i)
	}
	token, err := domain.NewRedemptionToken(tenantID, material)
	if err != nil {
		t.Fatalf("minting an invitation: %v", err)
	}
	f.session.accounts.redemption[token.Secret()] = repository.RedemptionAccount{
		Account: invited, ExpiresAt: expiresAt, TenantStatus: domain.TenantActive,
	}
	return secret.New(token.Secret())
}

// startInvited runs the first half from the invitation card and answers the state it minted.
func startInvited(t *testing.T, f *oidcFixture, invitation secret.Secret) secret.Secret {
	t.Helper()
	authorization, err := StartOidcSignIn{Writer: f.writer}.Execute(t.Context(),
		StartOidcSignInCommand{InvitationToken: invitation})
	if err != nil {
		t.Fatalf("starting from the invitation: %v", err)
	}
	return authorization.State
}

func TestAFlowStartedFromAnInvitationRemembersTheInvitedAccount(t *testing.T) {
	at := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	invited := invitedAda()
	f := newOidcFixture(t, at, invited)
	invitation := invitationFor(t, f, invited, tenant, at.Add(24*time.Hour), 7)

	state := startInvited(t, f, invitation)

	flow := f.flows.byState[state.Reveal()]
	if flow.InvitedAccountID != invited.ID {
		t.Errorf("the flow carries %q, want the invited account", flow.InvitedAccountID)
	}
	// Checked, not spent: the account is still invited, and the invitation still answers.
	if f.accounts.byID[invited.ID].Status != domain.AccountInvited {
		t.Error("starting the flow accepted the invitation")
	}
	if _, held := f.session.accounts.redemption[invitation.Reveal()]; !held {
		t.Error("starting the flow spent the invitation")
	}

	// And a sign-in that did not start from an invitation carries none.
	plain := start(t, f)
	if !f.flows.byState[plain.Reveal()].InvitedAccountID.IsZero() {
		t.Error("a plain sign-in carries an invitation")
	}
}

// Unknown, expired, already accepted, another workspace's and malformed are one refusal - the
// redemption's - and no flow is written for any of them.
func TestAnInvitationThatCannotBeRedeemedStartsNoFlow(t *testing.T) {
	at := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	other := shared.ID("018f2a1b-0000-7000-8000-0000000000ff")

	cases := map[string]func(t *testing.T, f *oidcFixture) secret.Secret{
		"unknown": func(t *testing.T, f *oidcFixture) secret.Secret {
			token, _ := domain.NewRedemptionToken(tenant, make([]byte, domain.TokenSecretBytes))
			return secret.New(token.Secret())
		},
		"lapsed": func(t *testing.T, f *oidcFixture) secret.Secret {
			return invitationFor(t, f, invitedAda(), tenant, at.Add(-time.Minute), 11)
		},
		"spent": func(t *testing.T, f *oidcFixture) secret.Secret {
			accepted := invitedAda()
			accepted.Status = domain.AccountActive
			return invitationFor(t, f, accepted, tenant, at.Add(time.Hour), 21)
		},
		"another workspace's": func(t *testing.T, f *oidcFixture) secret.Secret {
			return invitationFor(t, f, invitedAda(), other, at.Add(time.Hour), 31)
		},
		"malformed": func(t *testing.T, f *oidcFixture) secret.Secret {
			return secret.New("not-an-invitation")
		},
	}
	for name, invitation := range cases {
		t.Run(name, func(t *testing.T) {
			f := newOidcFixture(t, at, invitedAda())
			_, err := StartOidcSignIn{Writer: f.writer}.Execute(t.Context(),
				StartOidcSignInCommand{InvitationToken: invitation(t, f)})
			if detailOf(err) != "auth.redemption_failed" {
				t.Fatalf("the start answered %v, want the redemption's one refusal", err)
			}
			if len(f.flows.byState) != 0 {
				t.Error("a refused invitation left a flow behind")
			}
		})
	}
}

// Through the registry, which is what a request meets first: the descriptor declares the field the
// controller sends.
func TestTheInvitationReachesTheStartThroughTheRegistry(t *testing.T) {
	at := time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)
	invited := invitedAda()
	f := newOidcFixture(t, at, invited)
	invitation := invitationFor(t, f, invited, tenant, at.Add(time.Hour), 41)

	start := StartOidcSignIn{Writer: f.writer}
	registry, err := usecase.NewRegistry(nil, start.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	out, err := registry.Invoke(t.Context(), StartOidcSignInName,
		appshared.ActorContext{Kind: shared.ActorAnonymous},
		usecase.Input{"invitation_token": invitation.Reveal(), "tenant_header": tenant.String()})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	state, _ := out["state"].(string)
	if f.flows.byState[state].InvitedAccountID != invited.ID {
		t.Error("the invitation sent through the registry did not reach the flow")
	}
}
