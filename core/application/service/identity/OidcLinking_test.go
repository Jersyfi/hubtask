// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"fmt"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The owner of the fixture's workspace: a password, and - where a test arms it - a second factor.
var ownerAccount = domain.Account{
	ID: account, TenantID: tenant, Kind: domain.AccountUser,
	Email: "bert@example.org", DisplayName: "Bert", Status: domain.AccountActive,
}

// linkFixture is a workspace whose administrator configured a provider they control, set to admit
// anybody it knows, and made it vouch for the owner's address - the scenario the concept review of
// 2026-09-30 named (ADR-0071's addendum, E2).
func linkFixture(t *testing.T, armed bool) *oidcFixture {
	t.Helper()
	// The fixture clock `enrolled` computes its code against.
	f := newOidcFixture(t, now, ownerAccount)
	f.session.withAccount("bert@example.org", "correct horse battery")
	if armed {
		enrolled(t, f.session)
	}
	f.store.rows[0].Provisioning = domain.ProvisionAny
	f.relying.identity = provider.Identity{
		Subject: "attacker-made-subject", Email: "bert@example.org", EmailVerified: true,
		AddressAuthoritative: true, DisplayName: "Bert",
	}
	// Every credential its own identifier: a step here mints several, and the shared fixture's
	// sequence runs out after the session's two.
	ids := &distinctIDs{}
	f.session.writer.IDs, f.writer.Session.IDs = ids, ids
	// The second factor's step finishes a link through the OIDC writer, as the composition root
	// wires it.
	f.session.writer.Connector = f.writer
	return f
}

// distinctIDs answers a new identifier on every call.
type distinctIDs struct{ n int }

func (d *distinctIDs) NewID() shared.ID {
	d.n++
	return shared.ID(fmt.Sprintf("01936f2a-7c1e-7000-8000-%012d", d.n))
}

func arrive(t *testing.T, f *oidcFixture) (SignInResult, error) {
	t.Helper()
	return CompleteOidcSignIn{Writer: f.writer}.
		Execute(t.Context(), CompleteOidcSignInCommand{Code: "x", State: start(t, f)})
}

// E2's failing test, kept as the regression: before SC-01 the arrival was linked to the owner's
// account and a session opened, without the owner's password and without their second factor.
func TestAProviderCannotOpenAnAccountThatHoldsAPasswordAndAFactor(t *testing.T) {
	f := linkFixture(t, true)

	result, err := arrive(t, f)
	if err != nil {
		t.Fatalf("the arrival was refused rather than asked for the account's proof: %v", err)
	}
	if result.Pair != nil {
		t.Fatalf("a session for %s was opened on the provider's word alone", result.Pair.Session.AccountID)
	}
	if len(f.external.links) != 0 {
		t.Errorf("the provider's subject was linked to the existing account: %v", f.external.links)
	}
	challenge := result.Challenge
	if challenge == nil || len(challenge.Methods) != 1 || challenge.Methods[0] != methodLink {
		t.Fatalf("the arrival answered %+v, want one LINK challenge", challenge)
	}
	if challenge.Email != "bert@example.org" || challenge.ProviderName == "" {
		t.Errorf("the card cannot say whose account or which provider: %+v", challenge)
	}
}

// The whole step, for an account with a second factor: a wrong password connects nothing, the right
// one hands on to the second factor's step, and only the code connects the provider and opens the
// session.
func TestConnectingAnArmedAccountTakesItsPasswordAndItsCode(t *testing.T) {
	f := linkFixture(t, true)
	result, err := arrive(t, f)
	if err != nil || result.Challenge == nil {
		t.Fatalf("arriving: (%+v, %v)", result, err)
	}
	link := CompleteLink{Writer: f.writer}

	// A wrong password: refused as a sign-in is, and nothing connected.
	if _, err := link.Execute(t.Context(), CompleteLinkCommand{
		PendingToken: result.Challenge.Token, Password: secret.New("a guess"),
	}); err == nil {
		t.Fatal("a wrong password completed the LINK step")
	}
	if len(f.external.links) != 0 {
		t.Fatalf("a wrong password linked %v", f.external.links)
	}

	// The right password: the account has a second factor, so the answer is its step.
	next, err := link.Execute(t.Context(), CompleteLinkCommand{
		PendingToken: result.Challenge.Token, Password: secret.New("correct horse battery"),
	})
	if err != nil {
		t.Fatalf("the right password was refused: %v", err)
	}
	if next.Pair != nil {
		t.Fatal("a session opened before the account's second factor was proven")
	}
	if next.Challenge == nil || next.Challenge.Methods[0] != methodTotp {
		t.Fatalf("the right password answered %+v, want the second factor's step", next.Challenge)
	}
	if len(f.external.links) != 0 {
		t.Fatalf("the provider was connected before the second factor: %v", f.external.links)
	}

	// The LINK credential is spent: presenting it again connects nothing.
	if _, err := link.Execute(t.Context(), CompleteLinkCommand{
		PendingToken: result.Challenge.Token, Password: secret.New("correct horse battery"),
	}); err == nil {
		t.Error("a spent LINK credential was accepted a second time")
	}

	// The code: now the provider is connected, in the same step that opens the session. The next
	// step's code, because enrolling spent the current one.
	material := f.session.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	pair, _, err := CompleteSignIn{Writer: f.session.writer}.Execute(t.Context(), CompleteSignInCommand{
		PendingToken: next.Challenge.Token,
		Code:         domain.TotpCode([]byte(material), domain.TotpStep(now)+1),
	})
	if err != nil {
		t.Fatalf("the second factor's step: %v", err)
	}
	if pair.Session.AccountID != account {
		t.Errorf("the session belongs to %s", pair.Session.AccountID)
	}
	if len(f.external.links) != 1 || f.external.links[0] != "attacker-made-subject" {
		t.Errorf("after the code the links are %v, want the provider's subject once", f.external.links)
	}
	if !containsAction(auditActions(f.session.audit.entries), OidcLinkedAction) {
		t.Error("connecting the provider was not recorded")
	}
}

// Without a second factor, the password is the whole of the account's proof.
func TestConnectingAnAccountWithOnlyAPasswordTakesThePassword(t *testing.T) {
	f := linkFixture(t, false)
	result, err := arrive(t, f)
	if err != nil || result.Challenge == nil {
		t.Fatalf("arriving: (%+v, %v)", result, err)
	}

	done, err := CompleteLink{Writer: f.writer}.Execute(t.Context(), CompleteLinkCommand{
		PendingToken: result.Challenge.Token, Password: secret.New("correct horse battery"),
	})
	if err != nil {
		t.Fatalf("the right password was refused: %v", err)
	}
	if done.Pair == nil || done.Pair.Session.AccountID != account {
		t.Fatalf("the step answered %+v, want a session for the account", done)
	}
	if len(f.external.links) != 1 {
		t.Errorf("the links are %v, want the provider's subject once", f.external.links)
	}
}

// An account that signs in through another provider has no password to prove on this card. The
// arrival is refused with a sentence that names the way in the account has, and nothing links.
func TestAnAccountThatSignsInElsewhereIsNotConnectedHere(t *testing.T) {
	f := newOidcFixture(t, now, ownerAccount)
	f.store.rows[0].Provisioning = domain.ProvisionAny
	f.external.bySubject[linkKey(shared.ID("01936f2a-7c1e-7000-8000-0000000000b9"), "elsewhere")] = ownerAccount
	f.relying.identity = provider.Identity{
		Subject: "a-new-subject", Email: "bert@example.org", EmailVerified: true,
		AddressAuthoritative: true, DisplayName: "Bert",
	}

	_, err := arrive(t, f)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the arrival answered %v, want a refusal", err)
	}
	if len(f.external.links) != 0 {
		t.Errorf("an account with another provider was linked on this one's word: %v", f.external.links)
	}
}

// Each credential opens exactly the door it was minted for: a LINK credential does not complete the
// second factor's step, which would skip the password.
func TestALinkCredentialIsNotASecondFactorCredential(t *testing.T) {
	f := linkFixture(t, true)
	result, err := arrive(t, f)
	if err != nil || result.Challenge == nil {
		t.Fatalf("arriving: (%+v, %v)", result, err)
	}

	material := f.session.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	if _, _, err := (CompleteSignIn{Writer: f.session.writer}).Execute(t.Context(), CompleteSignInCommand{
		PendingToken: result.Challenge.Token,
		Code:         domain.TotpCode([]byte(material), domain.TotpStep(now)+1),
	}); err == nil {
		t.Error("a LINK credential completed the second factor's step, skipping the password")
	}
	if len(f.external.links) != 0 {
		t.Errorf("the provider was connected without the password: %v", f.external.links)
	}
}
