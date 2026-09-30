// SPDX-License-Identifier: BUSL-1.1
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

// UC-ID-12 check 3 and UC-ID-03 checks 5 and 6 (SC-06): turning one's own second factor off asks
// the same rule signing in asks. Before SC-06 it read the old boolean and only for administrators,
// so under *Everyone* any member could remove a factor the workspace demands of them.

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

// A row where the old boolean and the rule came apart before SC-06: the boolean on, the rule off.
// Turning off asks the rule, as signing in always did, so an administrator may.
func TestTurningOffIgnoresTheOldBooleanWhereTheRuleSaysOtherwise(t *testing.T) {
	fixture := armedUnder(t, domain.MfaForNobody, domain.RoleAdmin)
	fixture.session.writer.Policy = policyFake{required: true}

	if err := turnOff(t, fixture); err != nil {
		t.Errorf("an administrator under a rule of nobody was refused by the old boolean: %v", err)
	}
}
