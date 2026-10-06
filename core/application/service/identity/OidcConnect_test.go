// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"fmt"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// SC-33 (#1140, ADR-0078 §1, UC-ID-10 checks 1 and 6): in a workspace that switched the password off,
// the link *Forgot your password?* mailed and a fresh sign-in at the provider are together the
// account's proof at the provider's first arrival - in place of the password, never of the second
// factor. The link is checked when the flow starts and spent only by an arrival that connects.

var connectAt = time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)

// shutDoor is the workspace's rule as far as a connection asks it: whether the password is open.
type shutDoor struct{ open bool }

func (shutDoor) JudgeSignIn(context.Context, shared.ID, domain.Account, secret.Secret) (SignInVerdict, error) {
	return SignInVerdict{}, nil
}

func (d shutDoor) PasswordOpen(context.Context, shared.ID) (bool, bool, error) {
	return d.open, false, nil
}

// bertMember is the active account the link was mailed to.
func bertMember() domain.Account {
	return domain.Account{
		ID: account, TenantID: tenant, Kind: domain.AccountUser,
		Email: "bert@example.org", DisplayName: "Bert", Status: domain.AccountActive,
	}
}

// connectFixture is a workspace without the password whose provider is switched on, with Bert's
// account in it and the provider about to vouch for Bert's address, freshly signed in.
func connectFixture(t *testing.T) *oidcFixture {
	t.Helper()
	f := newOidcFixture(t, connectAt, bertMember())
	f.writer.Session.Rule = shutDoor{}
	f.relying.identity.Subject = "bert-at-the-provider"
	f.relying.identity.Email = "bert@example.org"
	f.relying.identity.DisplayName = "Bert"
	f.relying.identity.AuthTime = connectAt.Add(-30 * time.Second)
	return f
}

// connectLinkFor stores a CONNECT link for the account, as the reset mail mints it, and answers the
// token and the credential's identifier.
func connectLinkFor(t *testing.T, f *oidcFixture, accountID shared.ID, expiresAt time.Time, seed byte) (secret.Secret, shared.ID) {
	t.Helper()
	material := make([]byte, domain.TokenSecretBytes)
	for i := range material {
		material[i] = seed + byte(i)
	}
	token, err := domain.NewPendingToken(tenant, material)
	if err != nil {
		t.Fatalf("minting a link: %v", err)
	}
	id := shared.ID(fmt.Sprintf("018f2a1b-0000-7000-8000-0000000033%02x", seed))
	if err := f.session.writer.Pending.Insert(t.Context(), domain.PendingCredential{
		ID: id, TenantID: tenant, AccountID: accountID, Purpose: domain.PendingConnect,
		CreatedAt: connectAt, ExpiresAt: expiresAt,
	}, token); err != nil {
		t.Fatalf("storing the link: %v", err)
	}
	return secret.New(token.Secret()), id
}

// startConnect runs the first half from the connect card and answers the state it minted.
func startConnect(t *testing.T, f *oidcFixture, link secret.Secret) secret.Secret {
	t.Helper()
	authorization, err := StartOidcSignIn{Writer: f.writer}.Execute(t.Context(),
		StartOidcSignInCommand{ConnectToken: link})
	if err != nil {
		t.Fatalf("starting from the connect link: %v", err)
	}
	return authorization.State
}

// linkSpent answers whether the CONNECT credential has been spent.
func linkSpent(f *oidcFixture, id shared.ID) bool {
	lookup, err := f.session.writer.Pending.FindByID(context.Background(), id)
	return err == nil && !lookup.Credential.ConsumedAt.IsZero()
}

func TestAFlowStartedFromAConnectLinkRemembersItAndAsksForAFreshSignIn(t *testing.T) {
	f := connectFixture(t)
	link, id := connectLinkFor(t, f, account, connectAt.Add(domain.ResetLifetime), 1)

	state := startConnect(t, f, link)

	if flow := f.flows.byState[state.Reveal()]; flow.PendingID != id || !flow.InvitedAccountID.IsZero() {
		t.Errorf("the flow carries %+v, want the CONNECT link", flow)
	}
	if !f.relying.asked.Fresh {
		t.Error("the provider was not asked for a fresh sign-in")
	}
	if linkSpent(f, id) {
		t.Error("starting the flow spent the link")
	}

	// A sign-in that did not start from a link carries none, and asks for nothing fresh.
	plain := start(t, f)
	if !f.flows.byState[plain.Reveal()].PendingID.IsZero() || f.relying.asked.Fresh {
		t.Error("a plain sign-in carries a link, or asked for a fresh sign-in")
	}
}

// Unknown, expired, spent, another kind of credential, another workspace's, malformed - and a link
// whose reason has gone - are one refusal, the reset link's, and no flow is written for any of them.
func TestAConnectLinkThatCannotBeUsedStartsNoFlow(t *testing.T) {
	cases := map[string]func(t *testing.T, f *oidcFixture) secret.Secret{
		"unknown": func(t *testing.T, f *oidcFixture) secret.Secret {
			token, _ := domain.NewPendingToken(tenant, make([]byte, domain.TokenSecretBytes))
			return secret.New(token.Secret())
		},
		"expired": func(t *testing.T, f *oidcFixture) secret.Secret {
			link, _ := connectLinkFor(t, f, account, connectAt.Add(-time.Minute), 11)
			return link
		},
		"spent": func(t *testing.T, f *oidcFixture) secret.Secret {
			link, id := connectLinkFor(t, f, account, connectAt.Add(time.Hour), 21)
			if _, err := f.session.writer.Pending.Consume(t.Context(), id, connectAt); err != nil {
				t.Fatalf("spending: %v", err)
			}
			return link
		},
		"a reset link": func(t *testing.T, f *oidcFixture) secret.Secret {
			token, _ := domain.NewPendingToken(tenant, make([]byte, domain.TokenSecretBytes))
			_ = f.session.writer.Pending.Insert(t.Context(), domain.PendingCredential{
				ID: resetRowID, TenantID: tenant, AccountID: account, Purpose: domain.PendingReset,
				CreatedAt: connectAt, ExpiresAt: connectAt.Add(time.Hour),
			}, token)
			return secret.New(token.Secret())
		},
		"another workspace's": func(t *testing.T, f *oidcFixture) secret.Secret {
			token, _ := domain.NewPendingToken(shared.ID("018f2a1b-0000-7000-8000-0000000000fe"),
				make([]byte, domain.TokenSecretBytes))
			return secret.New(token.Secret())
		},
		"malformed": func(*testing.T, *oidcFixture) secret.Secret {
			return secret.New("not-a-link")
		},
		"the password open again": func(t *testing.T, f *oidcFixture) secret.Secret {
			f.writer.Session.Rule = shutDoor{open: true}
			link, _ := connectLinkFor(t, f, account, connectAt.Add(time.Hour), 31)
			return link
		},
		"a provider here lets the account in by now": func(t *testing.T, f *oidcFixture) secret.Secret {
			if _, err := f.external.LinkSubject(t.Context(), f.provider.ID, account, "bert-elsewhere", connectAt); err != nil {
				t.Fatalf("linking: %v", err)
			}
			link, _ := connectLinkFor(t, f, account, connectAt.Add(time.Hour), 41)
			return link
		},
	}
	for name, link := range cases {
		t.Run(name, func(t *testing.T) {
			f := connectFixture(t)
			_, err := StartOidcSignIn{Writer: f.writer}.Execute(t.Context(),
				StartOidcSignInCommand{ConnectToken: link(t, f)})
			if detailOf(err) != "auth.reset_failed" {
				t.Fatalf("the start answered %v, want the reset link's one refusal", err)
			}
			if len(f.flows.byState) != 0 {
				t.Error("a refused link left a flow behind")
			}
		})
	}
}

// One link per sign-in: an invitation and a connection are never the same flow.
func TestAConnectLinkIsNotSentBesideAnInvitation(t *testing.T) {
	invited := invitedAda()
	f := connectFixture(t)
	f.accounts.byID[invited.ID] = invited
	invitation := invitationFor(t, f, invited, tenant, connectAt.Add(time.Hour), 51)
	link, _ := connectLinkFor(t, f, account, connectAt.Add(time.Hour), 61)

	_, err := StartOidcSignIn{Writer: f.writer}.Execute(t.Context(),
		StartOidcSignInCommand{InvitationToken: invitation, ConnectToken: link})
	if detailOf(err) != "usecase.input_invalid" {
		t.Fatalf("both links answered %v, want the input refused", err)
	}
	if len(f.flows.byState) != 0 {
		t.Error("a refused start left a flow behind")
	}
}

// Through the registry, which is what a request meets first: the descriptor declares the field the
// controller sends.
func TestTheConnectLinkReachesTheStartThroughTheRegistry(t *testing.T) {
	f := connectFixture(t)
	link, id := connectLinkFor(t, f, account, connectAt.Add(time.Hour), 71)

	registry, err := usecase.NewRegistry(nil, StartOidcSignIn{Writer: f.writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	out, err := registry.Invoke(t.Context(), StartOidcSignInName,
		appshared.ActorContext{Kind: shared.ActorAnonymous},
		usecase.Input{"connect_token": link.Reveal(), "tenant_header": tenant.String()})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	state, _ := out["state"].(string)
	if f.flows.byState[state].PendingID != id {
		t.Error("the link sent through the registry did not reach the flow")
	}
}
