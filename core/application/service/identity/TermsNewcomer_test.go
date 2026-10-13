// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// A newcomer through a provider, where terms of use are in force, is made only once the terms are
// agreed (UC-ID-19 check 1, P-02): the arrival waits on the TERMS step, a person who declines leaves
// no account behind, and agreeing makes the account, connects the provider, gives the newcomer's
// access and opens the provider's session in one go.

// newcomerFixture is a provider that may make Ada an account, with terms in force and nobody here.
func newcomerFixture(t *testing.T) (*oidcFixture, *termsReaderFake, *termsStore) {
	t.Helper()
	f := newOidcFixture(t, now)
	reader, store := &termsReaderFake{terms: installationTerms("3")}, newTermsStore()
	withTermsOn(&f.writer.Session, reader, store)
	ids := &distinctIDs{}
	f.session.writer.IDs, f.writer.Session.IDs = ids, ids
	return f, reader, store
}

// arrives completes a provider sign-in and answers the TERMS step's token.
func arrives(t *testing.T, f *oidcFixture) secret.Secret {
	t.Helper()
	result, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(),
		CompleteOidcSignInCommand{Code: "the-code", State: start(t, f)})
	if err != nil {
		t.Fatalf("arriving: %v", err)
	}
	token := mustBeTermsStep(t, "the newcomer's arrival", result, len(f.session.sessions.inserted))
	if result.Challenge.Email != "ada@example.org" {
		t.Errorf("the step names %q, want the arriving address", result.Challenge.Email)
	}
	return token
}

func agreesAsNewcomer(t *testing.T, f *oidcFixture, token secret.Secret, version string) (SessionPair, error) {
	t.Helper()
	return AgreeToTerms{Writer: f.writer.Session, Arrivals: f.writer}.Execute(t.Context(), AgreeToTermsCommand{
		PendingToken: token, TermsURL: hostTerms, TermsVersion: version,
	})
}

func TestADeclinedNewcomerLeavesNoAccount(t *testing.T) {
	f, _, _ := newcomerFixture(t)
	arrives(t, f)

	// Declining sends nothing; what is asserted is what the arrival alone wrote.
	if len(f.accounts.inserted) != 0 {
		t.Errorf("the arrival made %d accounts before the terms were agreed", len(f.accounts.inserted))
	}
	if len(f.external.links) != 0 {
		t.Errorf("the arrival connected %v before the terms were agreed", f.external.links)
	}
	if containsAction(auditActions(f.session.audit.entries), OidcProvisionedAction) {
		t.Error("the trail records an account nobody agreed for")
	}
}

func TestAgreeingMakesTheNewcomerAndOpensTheProvidersSession(t *testing.T) {
	f, _, store := newcomerFixture(t)
	token := arrives(t, f)

	pair, err := agreesAsNewcomer(t, f, token, "3")
	if err != nil {
		t.Fatalf("agreeing: %v", err)
	}
	if len(f.accounts.inserted) != 1 {
		t.Fatalf("agreeing made %d accounts, want one", len(f.accounts.inserted))
	}
	made := f.accounts.inserted[0]
	if made.Email != "ada@example.org" || made.Status != domain.AccountActive || pair.Session.AccountID != made.ID {
		t.Errorf("made %+v, signed in %s", made, pair.Session.AccountID)
	}
	if pair.Session.SignedInWith != domain.SignedInWithOidc || pair.Session.SignedInVia != f.provider.ID {
		t.Errorf("the session was opened by %q via %q, want the provider's", pair.Session.SignedInWith, pair.Session.SignedInVia)
	}
	if len(f.external.links) != 1 {
		t.Errorf("the provider was connected %d times", len(f.external.links))
	}
	if agreement := store.agreements[made.ID]; agreement.Version != "3" {
		t.Errorf("the account keeps %+v", agreement)
	}
	actions := auditActions(f.session.audit.entries)
	if !containsAction(actions, OidcProvisionedAction) || !containsAction(actions, TermsAgreedAction) {
		t.Errorf("the trail reads %v", actions)
	}

	// The same token is spent.
	if _, err := agreesAsNewcomer(t, f, token, "3"); err == nil {
		t.Error("the spent step agreed again")
	}
}

// Two tabs: the second agreement finds the account the first one made and signs that in.
func TestASecondAgreementSignsInTheAccountTheFirstMade(t *testing.T) {
	f, _, _ := newcomerFixture(t)
	first, second := arrives(t, f), arrives(t, f)

	one, err := agreesAsNewcomer(t, f, first, "3")
	if err != nil {
		t.Fatalf("agreeing first: %v", err)
	}
	two, err := agreesAsNewcomer(t, f, second, "3")
	if err != nil {
		t.Fatalf("agreeing second: %v", err)
	}
	if len(f.accounts.inserted) != 1 || one.Session.AccountID != two.Session.AccountID {
		t.Errorf("two agreements made %d accounts (%s, %s)", len(f.accounts.inserted),
			one.Session.AccountID, two.Session.AccountID)
	}
}

// What changed while the step was open is asked again at agreement, and nothing is made.
func TestANewcomerIsAdmittedAgainAtAgreement(t *testing.T) {
	cases := map[string]struct {
		change func(*oidcFixture, *termsReaderFake)
		code   string
	}{
		"the provider narrowed to invitations": {
			change: func(f *oidcFixture, _ *termsReaderFake) { f.store.rows[0].Provisioning = domain.ProvisionInvitedOnly },
			code:   "identity_provider.not_invited",
		},
		"an account took the address": {
			change: func(f *oidcFixture, _ *termsReaderFake) {
				invited := domain.Account{
					ID: "01936f2a-7c1e-7000-8000-0000000141b1", TenantID: tenant, Kind: domain.AccountUser,
					Email: "ada@example.org", DisplayName: "Ada", Status: domain.AccountInvited,
				}
				f.accounts.byID[invited.ID], f.accounts.byEmail[invited.Email] = invited, invited
			},
			code: "identity_provider.account_taken",
		},
		"the terms changed": {
			change: func(_ *oidcFixture, reader *termsReaderFake) { reader.terms = installationTerms("4") },
			code:   "auth.terms_changed",
		},
		"the workspace was suspended": {
			change: func(f *oidcFixture, _ *termsReaderFake) {
				rows := f.writer.Session.Pending.(*pendingStore).rows
				for key, lookup := range rows {
					lookup.TenantStatus = domain.TenantSuspended
					rows[key] = lookup
				}
			},
			code: "access.tenant_suspended",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f, reader, _ := newcomerFixture(t)
			token := arrives(t, f)
			tc.change(f, reader)

			_, err := agreesAsNewcomer(t, f, token, "3")
			if err == nil || shared.AsError(err).DetailCode != tc.code {
				t.Fatalf("agreeing answered %v, want %s", err, tc.code)
			}
			if len(f.accounts.inserted) != 0 || len(f.external.links) != 0 {
				t.Errorf("a refused agreement made %v and connected %v", f.accounts.inserted, f.external.links)
			}
		})
	}
}

// Terms removed while the step was open need no agreement: the account is made and signed in, and
// nothing is recorded as agreed.
func TestTermsRemovedMeanwhileStillMakeTheNewcomer(t *testing.T) {
	f, reader, store := newcomerFixture(t)
	token := arrives(t, f)
	reader.terms = domain.TermsInForce{}

	pair, err := agreesAsNewcomer(t, f, token, "3")
	if err != nil {
		t.Fatalf("agreeing: %v", err)
	}
	if len(f.accounts.inserted) != 1 || pair.Session.AccountID != f.accounts.inserted[0].ID {
		t.Errorf("made %v, signed in %s", f.accounts.inserted, pair.Session.AccountID)
	}
	if store.recorded != 0 {
		t.Error("an agreement to no terms was recorded")
	}
}

// Without the provider's half wired, a newcomer's step is an unknown credential, never an account
// made without the provider's access rules.
func TestANewcomersStepIsRefusedWhereNothingCanMakeTheAccount(t *testing.T) {
	f, _, _ := newcomerFixture(t)
	token := arrives(t, f)

	_, err := AgreeToTerms{Writer: f.writer.Session}.Execute(t.Context(), AgreeToTermsCommand{
		PendingToken: token, TermsURL: hostTerms, TermsVersion: "3",
	})
	if err == nil || shared.AsError(err).DetailCode != "auth.mfa_challenge_failed" {
		t.Fatalf("agreeing answered %v, want the unknown credential's refusal", err)
	}
	if len(f.accounts.inserted) != 0 {
		t.Error("an account was made")
	}
}
