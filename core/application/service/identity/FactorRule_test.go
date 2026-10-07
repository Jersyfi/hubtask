// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// UC-ID-12 check 3 and UC-ID-03 checks 5 and 6: turning one's own second factor off asks
// the same rule signing in asks, never the old boolean: that one speaks only for administrators, so
// under *Everyone* any member could otherwise remove a factor the workspace demands of them.

// armedUnder is a person with an armed factor in a workspace whose rule is the one given, holding
// the roles given.
func armedUnder(t *testing.T, requirement domain.MfaRequirement, roles ...domain.Role) *stepFixture {
	t.Helper()
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MfaRequiredFor: requirementOf(requirement)}
	fixture.session.withAccount("bert@example.org", "correct horse battery")
	fixture.session.writer.Encryptor = newEncryptor()
	fixture.session.writer.People = newAccounts(domain.Account{
		ID: account, TenantID: tenant, Kind: domain.AccountUser,
		Email: "bert@example.org", DisplayName: "Bert", Status: domain.AccountActive,
	})
	fixture.session.writer.Memberships = membershipsFake{roles: roles}
	enrolled(t, fixture.session)
	return fixture
}

func turnOff(t *testing.T, fixture *stepFixture) error {
	t.Helper()
	return DisableTotp{Writer: fixture.session.writer}.Execute(t.Context(), signedInActor(),
		DisableTotpCommand{Password: secret.New("correct horse battery")})
}

func TestAMemberCannotRemoveAFactorTheWorkspaceDemandsOfEveryone(t *testing.T) {
	fixture := armedUnder(t, domain.MfaForEveryone, domain.RoleMember)

	err := turnOff(t, fixture)
	if !errors.Is(err, shared.ErrForbidden) || !strings.Contains(err.Error(), "auth.mfa_required_by_tenant") {
		t.Fatalf("a member under Everyone turning the factor off answered %v", err)
	}
	if _, err := fixture.enroll.Find(t.Context(), account); err != nil {
		t.Errorf("the factor is gone after the refusal: %v", err)
	}
}

// Every combination of the rule and a role, against what signing in demands of the same person:
// the two doors answer the same question the same way.
func TestTurningOffAsksTheSameRuleSigningInAsks(t *testing.T) {
	cases := []struct {
		requirement domain.MfaRequirement
		role        domain.Role
		demanded    bool
	}{
		{domain.MfaForNobody, domain.RoleMember, false},
		{domain.MfaForNobody, domain.RoleAdmin, false},
		{domain.MfaForAdmins, domain.RoleMember, false},
		{domain.MfaForAdmins, domain.RoleAdmin, true},
		{domain.MfaForAdmins, domain.RoleOwner, true},
		{domain.MfaForEveryone, domain.RoleMember, true},
		{domain.MfaForEveryone, domain.RoleOwner, true},
	}
	for _, c := range cases {
		t.Run(string(c.requirement)+"/"+string(c.role), func(t *testing.T) {
			fixture := armedUnder(t, c.requirement, c.role)
			err := turnOff(t, fixture)
			refused := errors.Is(err, shared.ErrForbidden)
			if refused != c.demanded {
				t.Errorf("turning off answered %v, want refused=%v", err, c.demanded)
			}
			if !c.demanded && err != nil {
				t.Errorf("turning off where nothing demands a factor answered %v", err)
			}
		})
	}
}

// A row where the old boolean and the rule disagree: the boolean on, the rule off.
// Turning off asks the rule, as signing in always did, so an administrator may.
func TestTurningOffIgnoresTheOldBooleanWhereTheRuleSaysOtherwise(t *testing.T) {
	fixture := armedUnder(t, domain.MfaForNobody, domain.RoleAdmin)
	fixture.session.writer.Policy = policyFake{required: true}

	if err := turnOff(t, fixture); err != nil {
		t.Errorf("an administrator under a rule of nobody was refused by the old boolean: %v", err)
	}
}

// UC-ID-03 check 5: the profile learns in advance what turning the factor off would meet, from the
// same reading (P-05) - so it can say "your workspace requires it" instead of offering the control.
func TestTheOwnAccountSaysWhetherTheWorkspaceRequiresAFactor(t *testing.T) {
	for _, c := range []struct {
		requirement domain.MfaRequirement
		role        domain.Role
		required    bool
	}{
		{domain.MfaForNobody, domain.RoleOwner, false},
		{domain.MfaForAdmins, domain.RoleMember, false},
		{domain.MfaForAdmins, domain.RoleAdmin, true},
		{domain.MfaForEveryone, domain.RoleMember, true},
	} {
		fixture := armedUnder(t, c.requirement, c.role)
		handler := GetOwnAccount{
			Accounts: fixture.session.writer.People, UnitOfWork: &unitOfWork{},
			Recovery: fixture.session.writer.Recovery, Enrollments: fixture.enroll,
			Factor: fixture.session.writer,
		}
		actor := signedInActor()
		actor.Scopes = []string{accountsRead}
		own, err := handler.ExecuteWithRecovery(t.Context(), actor)
		if err != nil {
			t.Fatalf("%s/%s: reading: %v", c.requirement, c.role, err)
		}
		if own.SecondFactorRequired != c.required {
			t.Errorf("%s/%s: required reads %v, want %v", c.requirement, c.role, own.SecondFactorRequired, c.required)
		}
	}
}

// UC-ID-12 check 3, the reset's door: a member without a factor under *Everyone* who resets the
// password is led into setting one up, not signed straight in - the same rule signing in asks.
func TestAResetAsksTheRuleSigningInAsks(t *testing.T) {
	fixture := newResetFixture(now)
	fixture.instance.level.Policy.Patch = domain.PolicyPatch{MfaRequiredFor: requirementOf(domain.MfaForEveryone)}
	fixture.writer.Session.Rule = fixture.writer
	fixture.writer.Session.Enrollments = newEnrollments()
	fixture.writer.Session.Memberships = membershipsFake{roles: []domain.Role{domain.RoleMember}}
	// The old boolean says nobody - an older row - and must not be what is read.
	fixture.writer.Session.Policy = policyFake{required: false}
	token := fixture.mintedFor(t, now, domain.PendingReset)

	result, err := ResetPassword{Writer: fixture.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: secret.New(token), Password: secret.New("seven blue lanterns"),
	})
	if err != nil {
		t.Fatalf("the reset was refused: %v", err)
	}
	if result.Pair != nil {
		t.Fatal("a member under Everyone was signed in by a reset without a second factor")
	}
	if result.Challenge == nil || len(result.Challenge.Methods) != 1 || result.Challenge.Methods[0] != methodEnroll {
		t.Errorf("the reset answered %+v, want the setup step", result.Challenge)
	}
}
