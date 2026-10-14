// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"slices"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// UC-ID-03 check 8, identity.md §12: redeeming an invitation is a sign-in for the second-factor
// rule. Where the rule in force covers the invited person, the redemption answers the ENROLL step
// instead of a session; the session opens only once the factor is armed.

// redeemUnder redeems a waiting invitation in a workspace whose rule is the one given, for a person
// holding the roles given.
func redeemUnder(t *testing.T, requirement domain.MfaRequirement, roles ...domain.Role) (*stepFixture, SignInResult) {
	t.Helper()
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MfaRequiredFor: requirementOf(requirement)}
	fixture.session.writer.Memberships = membershipsFake{roles: roles}
	token := redemptionToken(t)
	fixture.session.accounts.redemption[token.Secret()] = waitingRedemption()
	passwords := fixture.passwords.writer

	result, err := RedeemInvitation{Writer: fixture.session.writer, Passwords: &passwords}.Execute(t.Context(),
		RedeemInvitationCommand{Token: secret.New(token.Secret()), Password: secret.New("seven blue lanterns over the harbour")})
	if err != nil {
		t.Fatalf("redeeming: %v", err)
	}
	return fixture, result
}

func TestARedemptionAsksTheSecondFactorRuleASignInAsks(t *testing.T) {
	cases := []struct {
		requirement domain.MfaRequirement
		role        domain.Role
		enrol       bool
	}{
		{domain.MfaForNobody, domain.RoleMember, false},
		{domain.MfaForNobody, domain.RoleOwner, false},
		{domain.MfaForAdmins, domain.RoleMember, false},
		{domain.MfaForAdmins, domain.RoleAdmin, true},
		{domain.MfaForAdmins, domain.RoleOwner, true},
		{domain.MfaForEveryone, domain.RoleMember, true},
	}
	for _, c := range cases {
		t.Run(string(c.requirement)+"/"+string(c.role), func(t *testing.T) {
			fixture, result := redeemUnder(t, c.requirement, c.role)

			// The invitation is accepted either way: the password is set and the token spent.
			if fixture.session.accounts.redeemedHash != "hash:seven blue lanterns over the harbour" {
				t.Errorf("stored %q, want the first password", fixture.session.accounts.redeemedHash)
			}
			if !c.enrol {
				if result.Pair == nil || result.Challenge != nil {
					t.Fatalf("answered %+v, want the pair", result)
				}
				return
			}
			if result.Challenge == nil || result.Pair != nil {
				t.Fatalf("answered %+v, want the ENROLL step", result)
			}
			if !slices.Equal(result.Challenge.Methods, []string{methodEnroll}) {
				t.Errorf("methods %v, want [ENROLL]", result.Challenge.Methods)
			}
			// The person typed no address on the invitation card: the step names the account.
			if result.Challenge.Email != "bert@example.org" {
				t.Errorf("the step names %q, want the invited address", result.Challenge.Email)
			}
			if result.Challenge.PasswordRules != nil {
				t.Error("the password the rule just accepted is asked to change")
			}
			if n := len(fixture.session.sessions.inserted); n != 0 {
				t.Errorf("%d sessions opened before the factor is armed", n)
			}
		})
	}
}

// The step a redemption answers is the sign-in's own: setting the factor up with its pending
// credential arms it and opens the session - and only then.
func TestTheEnrolmentARedemptionOwesOpensTheSession(t *testing.T) {
	fixture, result := redeemUnder(t, domain.MfaForEveryone, domain.RoleMember)
	if result.Challenge == nil {
		t.Fatalf("answered %+v, want the ENROLL step", result)
	}
	fixture.session.writer.Encryptor = newEncryptor()
	pending := result.Challenge.Token

	if _, err := (EnrollTotp{Writer: fixture.session.writer}).Execute(t.Context(), appshared.Anonymous("en", "UTC"),
		EnrollTotpCommand{PendingToken: pending}); err != nil {
		t.Fatalf("enrolling with the redemption's credential: %v", err)
	}
	material := fixture.session.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	confirmed, err := ConfirmTotp{Writer: fixture.session.writer}.Execute(t.Context(), appshared.Anonymous("en", "UTC"),
		ConfirmTotpCommand{PendingToken: pending, Code: domain.TotpCode([]byte(material), domain.TotpStep(now))})
	if err != nil {
		t.Fatalf("confirming: %v", err)
	}
	if confirmed.Pair == nil {
		t.Fatalf("the confirmation answered %+v, want the session", confirmed)
	}
	if n := len(fixture.session.sessions.inserted); n != 1 {
		t.Errorf("%d sessions, want the one the armed factor opened", n)
	}
}

// Through the registry, the way REST and MCP reach it: the challenge's projection is what the REST
// adapter turns into a 202.
func TestARedemptionOwingTheFactorProjectsTheChallenge(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MfaRequiredFor: requirementOf(domain.MfaForEveryone)}
	token := redemptionToken(t)
	fixture.session.accounts.redemption[token.Secret()] = waitingRedemption()
	passwords := fixture.passwords.writer
	registry, err := usecase.NewRegistry(nil,
		RedeemInvitation{Writer: fixture.session.writer, Passwords: &passwords}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}

	out, err := registry.Invoke(t.Context(), RedeemInvitationName, appshared.Anonymous("en", "UTC"), usecase.Input{
		"token": token.Secret(), "password": "seven blue lanterns over the harbour",
	})
	if err != nil {
		t.Fatalf("redeeming through the registry: %v", err)
	}
	if required, _ := out["mfa_required"].(bool); !required || out.String("pending_token") == "" {
		t.Fatalf("the output carries no challenge: %v", out)
	}
	if out.String("email") != "bert@example.org" {
		t.Errorf("the output names %q, want the invited address", out.String("email"))
	}
	if _, held := out["access_token"]; held {
		t.Error("the output carries a session beside the challenge")
	}
}

// Terms agreed on the invitation card are stored with the redemption, so the setup the rule owes
// ends in the session and not in a second terms step.
func TestTermsAgreedAtTheRedemptionAreNotAskedAgainAfterTheSetup(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MfaRequiredFor: requirementOf(domain.MfaForEveryone)}
	store := newTermsStore()
	fixture.session.writer.TermsIn, fixture.session.writer.Terms = &termsReaderFake{terms: installationTerms("3")}, store
	token := redemptionToken(t)
	fixture.session.accounts.redemption[token.Secret()] = waitingRedemption()
	passwords := fixture.passwords.writer

	result, err := RedeemInvitation{Writer: fixture.session.writer, Passwords: &passwords}.Execute(t.Context(),
		RedeemInvitationCommand{
			Token: secret.New(token.Secret()), Password: secret.New("seven blue lanterns over the harbour"),
			TermsURL: hostTerms, TermsVersion: "3",
		})
	if err != nil || result.Challenge == nil {
		t.Fatalf("redeeming answered (%+v, %v), want the ENROLL step", result, err)
	}
	if _, recorded := store.agreements[account]; !recorded {
		t.Fatal("the agreement waits for the setup instead of landing with the redemption")
	}

	fixture.session.writer.Encryptor = newEncryptor()
	pending := result.Challenge.Token
	if _, err := (EnrollTotp{Writer: fixture.session.writer}).Execute(t.Context(), appshared.Anonymous("en", "UTC"),
		EnrollTotpCommand{PendingToken: pending}); err != nil {
		t.Fatalf("enrolling: %v", err)
	}
	material := fixture.session.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	confirmed, err := ConfirmTotp{Writer: fixture.session.writer}.Execute(t.Context(), appshared.Anonymous("en", "UTC"),
		ConfirmTotpCommand{PendingToken: pending, Code: domain.TotpCode([]byte(material), domain.TotpStep(now))})
	if err != nil {
		t.Fatalf("confirming: %v", err)
	}
	if confirmed.Pair == nil || confirmed.Challenge != nil {
		t.Fatalf("the confirmation answered %+v, want the session without a terms step", confirmed)
	}
}
