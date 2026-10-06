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

// connectAt is the fixture clock the second factor's codes are computed against.
var connectAt = now

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
	// Every credential its own identifier, and the second factor's step finishing a connection
	// through the OIDC writer, as the composition root wires it.
	ids := &distinctIDs{}
	f.session.writer.IDs, f.writer.Session.IDs = ids, ids
	f.session.writer.Connector = f.writer
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

// completeConnect is the provider's return to a flow started from a connect link.
func completeConnect(t *testing.T, f *oidcFixture, state secret.Secret) (SignInResult, error) {
	t.Helper()
	return CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "the-code", State: state, UserAgent: "Firefox", RemoteAddr: "203.0.113.7",
	})
}

// linkedProof answers the proof the trail's link entry for the account records, or "" without one.
func linkedProof(f *oidcFixture) any {
	for _, entry := range f.session.audit.entries {
		if entry.Action == OidcLinkedAction && entry.TargetID == account {
			proof, _ := entry.Changes["proof"].(map[string]any)
			return proof["to"]
		}
	}
	return ""
}

// endedOffer is an installation offer this workspace took, withdrawn an hour before the fixture clock.
func endedOffer(t *testing.T, f *oidcFixture) domain.IdentityProvider {
	t.Helper()
	ended := offeredUntil(connectAt.Add(-time.Hour))
	f.store.rows = append(f.store.rows, ended)
	return ended
}

// The three kinds of account ADR-0078 §1 names - a password never connected, an identity only at an
// offer that ended, no credential at all - each connect by mail and are signed in, and the next
// arrival finds the account by its subject alone. No password is stored on the way.
func TestEachAccountNoProviderLetsInConnectsByMailAndSignsIn(t *testing.T) {
	cases := map[string]func(t *testing.T, f *oidcFixture){
		"a password holder never connected": func(_ *testing.T, f *oidcFixture) {
			f.session.withAccount("bert@example.org", "correct horse battery")
		},
		"an identity only at an ended offer": func(t *testing.T, f *oidcFixture) {
			ended := endedOffer(t, f)
			if _, err := f.external.LinkSubject(t.Context(), ended.ID, account, "bert-at-the-platform", connectAt); err != nil {
				t.Fatalf("linking: %v", err)
			}
		},
		"no credential at all": func(*testing.T, *oidcFixture) {},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			f := connectFixture(t)
			arrange(t, f)
			hashBefore, _ := f.session.writer.Accounts.PasswordHashOf(t.Context(), account)
			link, id := connectLinkFor(t, f, account, connectAt.Add(domain.ResetLifetime), 1)

			result, err := completeConnect(t, f, startConnect(t, f, link))
			if err != nil {
				t.Fatalf("connecting: %v", err)
			}
			if result.Pair == nil || result.Pair.Session.AccountID != account {
				t.Fatalf("connecting answered %+v, want Bert's session", result)
			}
			if !linkSpent(f, id) {
				t.Error("the link was not spent by the connection")
			}
			owner, err := f.external.FindBySubject(t.Context(), f.provider.ID, "bert-at-the-provider")
			if err != nil || owner.ID != account {
				t.Errorf("the provider identity is connected to %v (%v), want Bert", owner.ID, err)
			}
			if proof := linkedProof(f); proof != string(domain.LinkProofMailbox) {
				t.Errorf("the trail records the connection's proof as %v, want MAILBOX", proof)
			}
			if hashAfter, _ := f.session.writer.Accounts.PasswordHashOf(t.Context(), account); hashAfter.Reveal() != hashBefore.Reveal() {
				t.Error("connecting by mail changed the account's password")
			}

			// From now on the provider alone signs Bert in.
			again, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(),
				CompleteOidcSignInCommand{Code: "again", State: start(t, f)})
			if err != nil || again.Pair == nil || again.Pair.Session.AccountID != account {
				t.Errorf("the next arrival answered %+v (%v), want Bert's session", again, err)
			}
		})
	}
}

// The mailbox stands in for the password, never for the second factor: an armed one is asked, the
// connection waits for it, and the code connects the provider and opens the provider's session.
func TestAnArmedFactorIsStillAskedAfterTheMailbox(t *testing.T) {
	f := connectFixture(t)
	enrolled(t, f.session)
	link, id := connectLinkFor(t, f, account, connectAt.Add(domain.ResetLifetime), 1)

	result, err := completeConnect(t, f, startConnect(t, f, link))
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	if result.Pair != nil {
		t.Fatal("a session opened before the account's second factor was proven")
	}
	if result.Challenge == nil || result.Challenge.Methods[0] != methodTotp ||
		result.Challenge.Email != "bert@example.org" {
		t.Fatalf("connecting answered %+v, want the second factor's step for Bert", result.Challenge)
	}
	if len(f.external.links) != 0 {
		t.Fatalf("the provider was connected before the second factor: %v", f.external.links)
	}
	if !linkSpent(f, id) {
		t.Error("handing on to the factor left the link unspent")
	}

	material := f.session.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	pair, _, err := CompleteSignIn{Writer: f.session.writer}.Execute(t.Context(), CompleteSignInCommand{
		PendingToken: result.Challenge.Token,
		Code:         domain.TotpCode([]byte(material), domain.TotpStep(connectAt)+1),
	})
	if err != nil {
		t.Fatalf("the second factor's step: %v", err)
	}
	if pair.Session.AccountID != account || pair.Session.SignedInWith != domain.SignedInWithOidc {
		t.Errorf("the session is %+v, want Bert's, opened through the provider", pair.Session)
	}
	if len(f.external.links) != 1 || f.external.links[0] != "bert-at-the-provider" {
		t.Errorf("after the code the links are %v, want the provider's subject once", f.external.links)
	}
	if proof := linkedProof(f); proof != string(domain.LinkProofMailbox) {
		t.Errorf("the trail records the connection's proof as %v, want MAILBOX", proof)
	}
}

// A sign-in that is not fresh, an address that is not the account's, an identity connected to
// somebody else here, a provider that would not admit the address, and the password open again -
// each connects nothing and leaves the link unspent for a second try. A refusal of the arrival is in
// the trail, written outside the rolled-back transaction.
func TestAConnectionThatIsNotProvenLeavesTheLinkUnspent(t *testing.T) {
	cases := map[string]struct {
		arrange  func(t *testing.T, f *oidcFixture)
		want     string
		recorded bool
	}{
		"a sign-in the provider kept": {
			arrange: func(_ *testing.T, f *oidcFixture) {
				f.relying.identity.AuthTime = connectAt.Add(-time.Hour)
			},
			want: "identity_provider.connect_not_fresh", recorded: true,
		},
		"no auth_time at all": {
			arrange: func(_ *testing.T, f *oidcFixture) { f.relying.identity.AuthTime = time.Time{} },
			want:    "identity_provider.connect_not_fresh", recorded: true,
		},
		"another address": {
			arrange: func(_ *testing.T, f *oidcFixture) { f.relying.identity.Email = "mallory@example.org" },
			want:    "identity_provider.connect_address_differs", recorded: true,
		},
		"an address the provider did not verify": {
			arrange: func(_ *testing.T, f *oidcFixture) { f.relying.identity.EmailVerified = false },
			want:    "identity_provider.connect_address_differs", recorded: true,
		},
		"an identity connected to somebody else": {
			arrange: func(t *testing.T, f *oidcFixture) {
				other := domain.Account{
					ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000c2"), TenantID: tenant,
					Kind: domain.AccountUser, Email: "carla@example.org", Status: domain.AccountActive,
				}
				f.accounts.byID[other.ID] = other
				if _, err := f.external.LinkSubject(t.Context(), f.provider.ID, other.ID, "bert-at-the-provider", connectAt); err != nil {
					t.Fatalf("linking: %v", err)
				}
			},
			want: "identity_provider.connect_identity_taken", recorded: true,
		},
		"an address outside the domains the provider admits": {
			arrange: func(_ *testing.T, f *oidcFixture) {
				f.store.rows[0].Provisioning = domain.ProvisionDomains
				f.store.rows[0].AllowedEmailDomains = []string{"elsewhere.example"}
			},
			want: "identity_provider.not_admitted", recorded: true,
		},
		"the password open again": {
			arrange: func(_ *testing.T, f *oidcFixture) { f.writer.Session.Rule = shutDoor{open: true} },
			want:    "auth.reset_failed",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := connectFixture(t)
			link, id := connectLinkFor(t, f, account, connectAt.Add(domain.ResetLifetime), 1)
			state := startConnect(t, f, link)
			c.arrange(t, f)

			result, err := completeConnect(t, f, state)
			if detailOf(err) != c.want {
				t.Fatalf("the arrival answered (%+v, %v), want %s", result, err, c.want)
			}
			if linkSpent(f, id) {
				t.Error("the refused arrival spent the link")
			}
			if owner, err := f.external.FindBySubject(t.Context(), f.provider.ID, "bert-at-the-provider"); err == nil && owner.ID == account {
				t.Error("the refused arrival connected the provider to Bert")
			}
			if containsAction(auditActions(f.session.audit.entries), OidcRefusedAction) != c.recorded {
				t.Errorf("the refusal in the trail: %v, want %v", auditActions(f.session.audit.entries), c.recorded)
			}
		})
	}
}

// A link is spent once: of two flows started from it, the first to come back connects and the second
// is refused as a spent link.
func TestAConnectLinkIsSpentOnce(t *testing.T) {
	f := connectFixture(t)
	link, _ := connectLinkFor(t, f, account, connectAt.Add(domain.ResetLifetime), 1)
	first, second := startConnect(t, f, link), startConnect(t, f, link)

	if result, err := completeConnect(t, f, first); err != nil || result.Pair == nil {
		t.Fatalf("the first return answered (%+v, %v)", result, err)
	}
	if _, err := completeConnect(t, f, second); detailOf(err) != "auth.reset_failed" {
		t.Fatalf("the second return answered %v, want the spent link's refusal", err)
	}
	if len(f.external.links) != 1 {
		t.Errorf("the links are %v, want one", f.external.links)
	}
}
