// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"slices"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// identity.md §8, UC-ID-02 and UC-ID-08 *Where it ends*: the workspace's second-factor rule covers
// those signing in with a password. A provider is trusted as a whole, so no provider arrival is
// routed into ENROLL, whatever the rule. A factor the account holds is asked only where a provider
// is connected to an existing account - at the LINK step and at a connect link's arrival - never at
// a later sign-in through a connected provider.

// ruleOf is the workspace's rule as the rule reader answers it, with nothing else set.
func ruleOf(requirement domain.MfaRequirement) PasswordWriter {
	passwords := newPasswordFixture(now)
	passwords.instance.level.Policy.Patch = domain.PolicyPatch{MfaRequiredFor: requirementOf(requirement)}
	return passwords.writer
}

// shutRequiring is a workspace that switched the password off, whose rule demands a factor of the
// requirement given - the connect link's setting.
type shutRequiring struct {
	shutDoor
	requirement domain.MfaRequirement
}

func (d shutRequiring) JudgeSignIn(context.Context, shared.ID, domain.Account, secret.Secret) (SignInVerdict, error) {
	return SignInVerdict{FactorRequired: d.requirement}, nil
}

// under puts the rule and the person's roles on both writers the arrival and its steps run through.
func under(f *oidcFixture, rule SignInRuleReader, role domain.Role) {
	held := membershipsFake{roles: []domain.Role{role}}
	f.writer.Session.Rule, f.session.writer.Rule = rule, rule
	f.writer.Session.Memberships, f.session.writer.Memberships = held, held
}

// covered is whether the rule demands a factor of a person signing in with a password in the role.
func covered(requirement domain.MfaRequirement, role domain.Role) bool {
	switch requirement {
	case domain.MfaForEveryone:
		return true
	case domain.MfaForAdmins:
		return role == domain.RoleOwner || role == domain.RoleAdmin
	}
	return false
}

// assertSession fails unless the answer is a provider session for the account, no step before it.
func assertSession(t *testing.T, result SignInResult, err error, accountID shared.ID) {
	t.Helper()
	assertOpened(t, result, err, accountID)
	if result.Pair.Session.SignedInWith != domain.SignedInWithOidc {
		t.Errorf("the session records %q, want a provider sign-in", result.Pair.Session.SignedInWith)
	}
}

// assertOpened fails unless the answer is a session for the account, no step before it. The
// factor's step after the LINK step records the proofs it took (the password and the code), so
// that session is asserted opened rather than by its method.
func assertOpened(t *testing.T, result SignInResult, err error, accountID shared.ID) {
	t.Helper()
	if err != nil {
		t.Fatalf("the arrival answered %v, want a session", err)
	}
	if result.Challenge != nil {
		t.Fatalf("the arrival answered the step %v, want a session", result.Challenge.Methods)
	}
	if result.Pair == nil || result.Pair.Session.AccountID != accountID {
		t.Fatalf("the arrival answered %+v, want a session for %s", result, accountID)
	}
}

// assertFactorStep fails unless the answer is the step asking for the factor the account holds.
func assertFactorStep(t *testing.T, result SignInResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("the arrival answered %v, want the second factor's step", err)
	}
	if result.Pair != nil {
		t.Fatal("a session opened before the account's second factor was proven")
	}
	if result.Challenge == nil || !slices.Contains(result.Challenge.Methods, methodTotp) ||
		slices.Contains(result.Challenge.Methods, methodEnroll) {
		t.Fatalf("the arrival answered %+v, want the second factor's step", result.Challenge)
	}
}

// passwordLink completes the LINK step with the account's password.
func passwordLink(t *testing.T, f *oidcFixture, arrival SignInResult) (SignInResult, error) {
	t.Helper()
	if arrival.Challenge == nil || arrival.Challenge.Methods[0] != methodLink {
		t.Fatalf("the arrival answered %+v, want the LINK step", arrival)
	}
	return CompleteLink{Writer: f.writer}.Execute(t.Context(), CompleteLinkCommand{
		PendingToken: arrival.Challenge.Token, Password: secret.New("correct horse battery"),
	})
}

// codeOf answers the code of the authenticator `enrolled` armed, one step after enrolling spent the
// current one.
func codeOf(f *oidcFixture) string {
	material := f.session.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	return domain.TotpCode([]byte(material), domain.TotpStep(now)+1)
}

func TestAProviderArrivalIsNeverRoutedIntoEnrolment(t *testing.T) {
	requirements := []domain.MfaRequirement{domain.MfaForNobody, domain.MfaForAdmins, domain.MfaForEveryone}
	roles := []domain.Role{domain.RoleMember, domain.RoleOwner}

	for _, requirement := range requirements {
		for _, role := range roles {
			name := string(requirement) + "/" + string(role)

			// The control: the same rule, read through the same reader, routes this person's
			// password sign-in into ENROLL wherever it covers them - so the rows below meet a rule
			// that is in force.
			t.Run(name+"/a password sign-in", func(t *testing.T) {
				f := linkFixture(t, false)
				under(f, ruleOf(requirement), role)
				result, err := SignIn{Writer: f.session.writer}.Execute(t.Context(), SignInCommand{
					Email: "bert@example.org", Password: secret.New("correct horse battery"),
				})
				if err != nil {
					t.Fatalf("signing in: %v", err)
				}
				enrol := result.Challenge != nil && slices.Equal(result.Challenge.Methods, []string{methodEnroll})
				if enrol != covered(requirement, role) {
					t.Errorf("the password sign-in answered %+v, want ENROLL=%v", result.Challenge, covered(requirement, role))
				}
			})

			t.Run(name+"/a newcomer, and the same person again", func(t *testing.T) {
				f := newOidcFixture(t, now)
				under(f, ruleOf(requirement), role)
				first, err := arrive(t, f)
				if err != nil || first.Pair == nil {
					t.Fatalf("the first arrival answered (%+v, %v), want a session", first, err)
				}
				assertSession(t, first, err, first.Pair.Session.AccountID)
				again, err := arrive(t, f)
				assertSession(t, again, err, first.Pair.Session.AccountID)
			})

			t.Run(name+"/an invited person through the invitation's link", func(t *testing.T) {
				f, invitation := invitedFixture(t, domain.ProvisionAny)
				under(f, ruleOf(requirement), role)
				result, err := complete(t, f, startInvited(t, f, invitation))
				assertSession(t, result, err, invitedAda().ID)
			})

			// The password at the LINK step proves whose account the provider is connected to; the
			// session it opens is the provider's, so no ENROLL follows it either.
			t.Run(name+"/the LINK step without a factor", func(t *testing.T) {
				f := linkFixture(t, false)
				under(f, ruleOf(requirement), role)
				arrival, err := arrive(t, f)
				if err != nil {
					t.Fatalf("arriving: %v", err)
				}
				linked, err := passwordLink(t, f, arrival)
				assertSession(t, linked, err, account)
			})

			// An armed factor is asked once, when the provider is connected, and never again at a
			// later arrival through it.
			t.Run(name+"/the LINK step with a factor, and the connected provider later", func(t *testing.T) {
				f := linkFixture(t, true)
				under(f, ruleOf(requirement), role)
				arrival, err := arrive(t, f)
				if err != nil {
					t.Fatalf("arriving: %v", err)
				}
				next, err := passwordLink(t, f, arrival)
				assertFactorStep(t, next, err)
				completed, _, err := CompleteSignIn{Writer: f.session.writer}.Execute(t.Context(),
					CompleteSignInCommand{PendingToken: next.Challenge.Token, Code: codeOf(f)})
				assertOpened(t, completed, err, account)

				later, err := arrive(t, f)
				assertSession(t, later, err, account)
			})

			t.Run(name+"/a connect link without a factor", func(t *testing.T) {
				f := connectFixture(t)
				under(f, shutRequiring{requirement: requirement}, role)
				link, _ := connectLinkFor(t, f, account, connectAt.Add(domain.ResetLifetime), 1)
				result, err := completeConnect(t, f, startConnect(t, f, link))
				assertSession(t, result, err, account)
			})

			t.Run(name+"/a connect link with a factor, and the connected provider later", func(t *testing.T) {
				f := connectFixture(t)
				under(f, shutRequiring{requirement: requirement}, role)
				enrolled(t, f.session)
				link, _ := connectLinkFor(t, f, account, connectAt.Add(domain.ResetLifetime), 1)
				result, err := completeConnect(t, f, startConnect(t, f, link))
				assertFactorStep(t, result, err)
				completed, _, err := CompleteSignIn{Writer: f.session.writer}.Execute(t.Context(),
					CompleteSignInCommand{PendingToken: result.Challenge.Token, Code: codeOf(f)})
				assertSession(t, completed, err, account)

				later, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(),
					CompleteOidcSignInCommand{Code: "again", State: start(t, f)})
				assertSession(t, later, err, account)
			})
		}
	}
}
