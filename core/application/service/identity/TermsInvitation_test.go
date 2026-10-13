// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Terms of use at the invitation (UC-ID-19 check 1, UC-ID-07 check 4): the person cannot accept
// without agreeing, and the account keeps the version and the time.

func redeemThroughTheRegistry(t *testing.T, writer SessionWriter, extra usecase.Input) (usecase.Output, error) {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, RedeemInvitation{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	in := usecase.Input{"token": redemptionToken(t).Secret(), "password": "a long first password"}
	for key, value := range extra {
		in[key] = value
	}
	return registry.Invoke(t.Context(), RedeemInvitationName, appshared.Anonymous("en", "UTC"), in)
}

func TestAnInvitationIsAcceptedOnlyWithTheTermsAgreed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		terms   domain.TermsInForce
		sent    usecase.Input
		refusal string
	}{
		{name: "agreed to the terms in force", terms: installationTerms("3"),
			sent: usecase.Input{"terms_url": hostTerms, "terms_version": "3"}},
		{name: "agreed to terms without a version", terms: installationTerms(""),
			sent: usecase.Input{"terms_url": hostTerms}},
		{name: "not agreed", terms: installationTerms("3"),
			refusal: "auth.terms_not_agreed"},
		{name: "agreed to an older version", terms: installationTerms("3"),
			sent: usecase.Input{"terms_url": hostTerms, "terms_version": "2"}, refusal: "auth.terms_changed"},
		// Check 5: where no terms are set, nothing is asked and an agreement sent changes nothing.
		{name: "no terms set", terms: domain.TermsInForce{}},
		{name: "no terms set, an agreement sent anyway", terms: domain.TermsInForce{},
			sent: usecase.Input{"terms_url": hostTerms, "terms_version": "3"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newSessionFixture(now)
			fixture.accounts.redemption[redemptionToken(t).Secret()] = waitingRedemption()
			store := newTermsStore()
			fixture.writer.TermsIn, fixture.writer.Terms = &termsReaderFake{terms: tc.terms}, store

			out, err := redeemThroughTheRegistry(t, fixture.writer, tc.sent)
			if tc.refusal != "" {
				var problem *shared.Error
				if !errors.As(err, &problem) || problem.DetailCode != tc.refusal {
					t.Fatalf("err = %v, want %s", err, tc.refusal)
				}
				// Refused before the token was looked up: nothing spent, nothing activated.
				if fixture.accounts.redeemedHash != "" || len(fixture.sessions.inserted) != 0 {
					t.Error("a refused redemption activated the account or opened a session")
				}
				return
			}
			if err != nil || out.String("access_token") == "" {
				t.Fatalf("redeeming answered (%v, %v)", out, err)
			}
			agreement, recorded := store.agreements[account]
			if tc.terms.IsSet() != recorded {
				t.Fatalf("recorded %v, want %v", agreement, tc.terms.IsSet())
			}
			if recorded && (agreement.URL != hostTerms || agreement.Version != tc.terms.Version ||
				!agreement.AgreedAt.Equal(now)) {
				t.Errorf("the account keeps %+v", agreement)
			}
			if recorded != containsAction(auditActions(fixture.audit.entries), TermsAgreedAction) {
				t.Errorf("the trail holds %v", auditActions(fixture.audit.entries))
			}
		})
	}
}

// The provider half: the agreement given on the card rides on the flow, and the arrival that
// activates the invited account stores it - so no TERMS step follows. Without it, the arrival
// activates the account and the step asks; never a session without the terms agreed.
func TestAProviderInvitationCarriesTheAgreementOrMeetsTheStep(t *testing.T) {
	for name, agreed := range map[string]bool{"agreed on the card": true, "not agreed on the card": false} {
		t.Run(name, func(t *testing.T) {
			f, invitation := invitedFixture(t, domain.ProvisionInvitedOnly)
			store := newTermsStore()
			f.writer.Session.TermsIn, f.writer.Session.Terms = &termsReaderFake{terms: installationTerms("3")}, store
			invited := invitedAda()

			command := StartOidcSignInCommand{InvitationToken: invitation}
			if agreed {
				command.TermsURL, command.TermsVersion = hostTerms, "3"
			}
			authorization, err := StartOidcSignIn{Writer: f.writer}.Execute(t.Context(), command)
			if err != nil {
				t.Fatalf("starting: %v", err)
			}
			result, err := complete(t, f, authorization.State)
			if err != nil {
				t.Fatalf("arriving: %v", err)
			}
			if status := f.accounts.byID[invited.ID].Status; status != domain.AccountActive {
				t.Errorf("the account is %s, want ACTIVE", status)
			}
			if !agreed {
				mustBeTermsStep(t, name, result, len(f.session.sessions.inserted))
				return
			}
			if result.Pair == nil || result.Pair.Session.AccountID != invited.ID {
				t.Fatalf("the arrival answered %+v, want the invited person's session", result)
			}
			if agreement := store.agreements[invited.ID]; agreement.URL != hostTerms || agreement.Version != "3" {
				t.Errorf("the account keeps %+v", agreement)
			}
		})
	}
}

// An agreement to other terms than the ones in force is not carried, so the step asks.
func TestAProviderInvitationAgreedToOldTermsMeetsTheStep(t *testing.T) {
	f, invitation := invitedFixture(t, domain.ProvisionInvitedOnly)
	f.writer.Session.TermsIn, f.writer.Session.Terms = &termsReaderFake{terms: installationTerms("4")}, newTermsStore()

	authorization, err := StartOidcSignIn{Writer: f.writer}.Execute(t.Context(), StartOidcSignInCommand{
		InvitationToken: invitation, TermsURL: hostTerms, TermsVersion: "3",
	})
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	result, err := complete(t, f, authorization.State)
	if err != nil {
		t.Fatalf("arriving: %v", err)
	}
	mustBeTermsStep(t, "old terms", result, len(f.session.sessions.inserted))
}
