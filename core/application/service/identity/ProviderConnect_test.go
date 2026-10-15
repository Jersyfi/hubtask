// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// ADR-0078 §1, UC-ID-10 check 8: a signed-in person connects a provider identity whose address
// differs from the account's, after a step-up and a fresh sign-in at the provider; an identity
// connected to another account is refused. Through the registry, as a client reaches it.

// connectFromSession is Bert, signed in on a live session, with nothing connected yet, and a
// provider that will vouch for a different, verified address of his.
type connectFromSession struct {
	f        *oidcFixture
	stepUp   *stepUpFake
	registry *usecase.Registry
}

func newConnectFromSession(t *testing.T, held domain.Account) *connectFromSession {
	t.Helper()
	f := newOidcFixture(t, now, held)
	credential, _ := refreshCredential(now)
	f.session.sessions.sessions[sessionRowID] = repository.SessionCredential{
		TenantStatus: domain.TenantActive, Session: credential.Session, Account: credential.Account,
	}
	f.relying.identity.Subject = "bert-private"
	f.relying.identity.Email = "bert.private@example.org"
	f.relying.identity.EmailVerified = true
	f.relying.identity.AuthTime = now
	c := &connectFromSession{f: f, stepUp: &stepUpFake{satisfied: true}}
	registry, err := usecase.NewRegistry(nil,
		StartProviderConnect{Writer: f.writer, StepUp: c.stepUp}.Descriptor(),
		CompleteProviderConnect{Writer: f.writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	c.registry = registry
	return c
}

func bertWithAddress() domain.Account {
	return domain.Account{
		ID: account, TenantID: tenant, Kind: domain.AccountUser,
		Email: "bert@example.org", DisplayName: "Bert", Status: domain.AccountActive,
	}
}

func (c *connectFromSession) start(t *testing.T, actor appshared.ActorContext) error {
	t.Helper()
	_, err := c.registry.Invoke(t.Context(), StartProviderConnectName, actor, usecase.Input{
		"provider_id": c.f.provider.ID.String(), "step_up_token": "hbt_sup_fresh",
	})
	return err
}

func (c *connectFromSession) complete(t *testing.T, actor appshared.ActorContext) (usecase.Output, error) {
	t.Helper()
	return c.registry.Invoke(t.Context(), CompleteProviderConnectName, actor, usecase.Input{
		"state": c.f.relying.asked.State, "authorization_code": "the-code",
	})
}

func refusedWith(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), code) {
		t.Fatalf("answered %v, want %s", err, code)
	}
}

func TestASignedInPersonConnectsAProviderWithAnotherAddress(t *testing.T) {
	c := newConnectFromSession(t, bertWithAddress())

	if err := c.start(t, signedInActor()); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if !c.f.relying.asked.Fresh {
		t.Error("the provider was not asked for a fresh sign-in")
	}
	if len(c.stepUp.presented) != 1 {
		t.Errorf("the step-up was asked %d times, want once", len(c.stepUp.presented))
	}
	out, err := c.complete(t, signedInActor())
	if err != nil {
		t.Fatalf("completing: %v", err)
	}
	if out.String("provider_id") != c.f.provider.ID.String() || out.String("provider_name") != c.f.provider.DisplayName {
		t.Errorf("answered %v", out)
	}
	owner, err := c.f.external.FindBySubject(t.Context(), c.f.provider.ID, "bert-private")
	if err != nil || owner.ID != account {
		t.Fatalf("the identity is connected to %v (%v), want Bert", owner.ID, err)
	}
	// The trail says how the account proved itself, without the address.
	if proof := linkedProof(c.f); proof != string(domain.LinkProofSession) {
		t.Errorf("the link's proof is %v, want SESSION", proof)
	}
	// Once connected, the provider alone signs Bert in - by its subject, whatever the address.
	again, err := CompleteOidcSignIn{Writer: c.f.writer}.Execute(t.Context(),
		CompleteOidcSignInCommand{Code: "again", State: start(t, c.f)})
	if err != nil || again.Pair == nil || again.Pair.Session.AccountID != account {
		t.Errorf("the next arrival answered %+v (%v), want Bert's session", again, err)
	}
}

// The connection appears among the person's ways to sign in: the provider and since when, and
// nothing that identifies the person at the provider.
func TestAConnectedProviderIsListedAmongTheWaysToSignIn(t *testing.T) {
	c := newConnectFromSession(t, bertWithAddress())
	if err := c.start(t, signedInActor()); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if _, err := c.complete(t, signedInActor()); err != nil {
		t.Fatalf("completing: %v", err)
	}
	registry, err := usecase.NewRegistry(nil, ListOwnIdentities{Writer: c.f.writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	out, err := registry.Invoke(t.Context(), ListOwnIdentitiesName, signedInActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0].String("provider_id") != c.f.provider.ID.String() ||
		rows[0].String("provider_name") != c.f.provider.DisplayName {
		t.Fatalf("listed %v", rows)
	}
	for field := range rows[0] {
		if field != "provider_id" && field != "provider_name" && field != "linked_at" {
			t.Errorf("the list carries %q", field)
		}
	}
}

func TestAConnectionWithoutAStepUpIsRefused(t *testing.T) {
	c := newConnectFromSession(t, bertWithAddress())
	c.stepUp.satisfied = false

	refusedWith(t, c.start(t, signedInActor()), "auth.step_up_required")
	if c.f.relying.asked.State != "" {
		t.Error("the provider was asked without a step-up")
	}
}

// A personal access token has no session and no person at the keyboard.
func TestAConnectionNeedsASession(t *testing.T) {
	c := newConnectFromSession(t, bertWithAddress())
	token := signedInActor()
	token.TokenID = shared.ID("01936f2a-7c1e-7000-8000-0000000000aa")

	refusedWith(t, c.start(t, token), "auth.step_up_session_required")
}

// An account without an address is one no provider reaches (identity.md §4.1).
func TestAnAccountWithoutAnAddressConnectsNoProvider(t *testing.T) {
	managed := bertWithAddress()
	managed.Email, managed.SignInName = "", "bert"
	c := newConnectFromSession(t, managed)

	refusedWith(t, c.start(t, signedInActor()), "identity_provider.connect_needs_address")
	if len(c.stepUp.presented) != 0 {
		t.Error("the refusal spent the step-up")
	}
}

// One identity per account per provider, never re-pointed.
func TestAProviderAlreadyConnectedIsNotConnectedAgain(t *testing.T) {
	c := newConnectFromSession(t, bertWithAddress())
	if _, err := c.f.external.LinkSubject(t.Context(), c.f.provider.ID, account, "bert-work", now); err != nil {
		t.Fatalf("linking: %v", err)
	}

	refusedWith(t, c.start(t, signedInActor()), "identity_provider.already_connected")
}

func TestAnIdentityConnectedToSomebodyElseIsRefused(t *testing.T) {
	c := newConnectFromSession(t, bertWithAddress())
	other := shared.ID("01936f2a-7c1e-7000-8000-0000000000c9")
	if _, err := c.f.external.LinkSubject(t.Context(), c.f.provider.ID, other, "bert-private", now); err != nil {
		t.Fatalf("linking: %v", err)
	}
	if err := c.start(t, signedInActor()); err != nil {
		t.Fatalf("starting: %v", err)
	}
	links := len(c.f.external.links)

	_, err := c.complete(t, signedInActor())
	refusedWith(t, err, "identity_provider.connection_taken")
	if len(c.f.external.links) != links {
		t.Error("the refused identity was connected")
	}
}

func TestASignInThatIsNotFreshConnectsNothing(t *testing.T) {
	c := newConnectFromSession(t, bertWithAddress())
	c.f.relying.identity.AuthTime = now.Add(-domain.OidcFlowLifetime)
	if err := c.start(t, signedInActor()); err != nil {
		t.Fatalf("starting: %v", err)
	}

	_, err := c.complete(t, signedInActor())
	refusedWith(t, err, "identity_provider.connection_not_fresh")
	if len(c.f.external.links) != 0 {
		t.Error("a stale sign-in was connected")
	}
}

// The address need not be the account's, but the provider must have verified it.
func TestAnUnverifiedAddressConnectsNothing(t *testing.T) {
	c := newConnectFromSession(t, bertWithAddress())
	c.f.relying.identity.EmailVerified = false
	if err := c.start(t, signedInActor()); err != nil {
		t.Fatalf("starting: %v", err)
	}

	_, err := c.complete(t, signedInActor())
	refusedWith(t, err, "identity_provider.not_admitted")
}

// The lists bound who comes in new, not a member bringing their own proof (identity.md §10.4), as
// the LINK step and the link by mail read it today: an address outside the provider's domain list
// connects. Whether the lists should bound connections too is the owner's open question (§21 I-14);
// this pins the documented rule until it is answered.
func TestAnAddressOutsideTheProvidersListConnectsOnTheMembersOwnProof(t *testing.T) {
	c := newConnectFromSession(t, bertWithAddress())
	c.f.relying.identity.Email = "bert@elsewhere.example"
	if err := c.start(t, signedInActor()); err != nil {
		t.Fatalf("starting: %v", err)
	}
	if _, err := c.complete(t, signedInActor()); err != nil {
		t.Fatalf("an address outside the list was refused: %v", err)
	}
}

// Another session cannot finish this session's connection, and a sign-in callback cannot either.
func TestAConnectionIsFinishedOnlyByItsOwnSession(t *testing.T) {
	c := newConnectFromSession(t, bertWithAddress())
	if err := c.start(t, signedInActor()); err != nil {
		t.Fatalf("starting: %v", err)
	}
	credential, _ := refreshCredential(now)
	other := credential.Session
	other.ID = refreshRowID
	c.f.session.sessions.sessions[refreshRowID] = repository.SessionCredential{
		TenantStatus: domain.TenantActive, Session: other, Account: credential.Account,
	}
	thief := signedInActor()
	thief.TokenID = refreshRowID

	_, err := c.complete(t, thief)
	refusedWith(t, err, "auth.oidc_failed")
	if _, err := (CompleteOidcSignIn{Writer: c.f.writer}).Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "x", State: secret.New(c.f.relying.asked.State),
	}); err == nil {
		t.Error("a sign-in callback finished a connection's flow")
	}
	if len(c.f.external.links) != 0 {
		t.Error("something was connected")
	}
}
