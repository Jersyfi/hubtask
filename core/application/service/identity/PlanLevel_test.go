// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// UC-ID-12 check 4, UC-ID-16 check 7 (multi-tenancy.md §4.2): a workspace's plan is the level
// between the installation and the workspace. It tightens what the installation left open, never
// loosens it, and the workspace tightens further below it.

// onPlan puts the workspace on a plan whose sign-in and legal layers are the ones given.
func onPlan(policy domain.PolicyLayer, legal domain.LegalLayer) domain.WorkspacePlan {
	if policy.Locks == nil {
		policy.Locks = map[domain.PolicySwitch]bool{}
	}
	if legal.Locks == nil {
		legal.Locks = map[domain.LegalLink]bool{}
	}
	return domain.WorkspacePlan{Name: "team", DisplayName: "Team", Policy: policy, Legal: legal}
}

func TestAPlanTightensBetweenTheInstallationAndTheWorkspace(t *testing.T) {
	cases := []struct {
		name         string
		installation *int
		plan         *int
		workspace    *int
		want         int
	}{
		{"the plan tightens what the installation left open", intOf(12), intOf(16), nil, 16},
		{"the plan cannot loosen the installation", intOf(16), intOf(12), nil, 16},
		{"the workspace tightens below the plan", intOf(12), intOf(16), intOf(20), 20},
		{"the workspace cannot loosen the plan", intOf(12), intOf(16), intOf(14), 16},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			handler, workspace, instance := newRulesFixture()
			instance.level.Policy.Patch = domain.PolicyPatch{MinLength: c.installation}
			workspace.row.Plan = onPlan(domain.PolicyLayer{Patch: domain.PolicyPatch{MinLength: c.plan}}, domain.LegalLayer{})
			workspace.row.Settings.SignIn = domain.PolicyPatch{MinLength: c.workspace}

			rules, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
			if err != nil {
				t.Fatalf("the rules were refused: %v", err)
			}
			if rules.Password.MinLength != c.want {
				t.Errorf("the rule answered %d characters, want %d", rules.Password.MinLength, c.want)
			}
		})
	}
}

// A plan's locked switch holds against the workspace; the installation's lock holds against both.
func TestAPlanLocksWhatTheInstallationLeftOpen(t *testing.T) {
	handler, workspace, _ := newRulesFixture()
	workspace.row.Plan = onPlan(domain.PolicyLayer{
		Patch: domain.PolicyPatch{MinLength: intOf(16)},
		Locks: map[domain.PolicySwitch]bool{domain.SwitchMinLength: true},
	}, domain.LegalLayer{})
	workspace.row.Settings.SignIn = domain.PolicyPatch{MinLength: intOf(20)}

	rules, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("the rules were refused: %v", err)
	}
	if rules.Password.MinLength != 16 {
		t.Errorf("under the plan's lock the rule answered %d characters, want the plan's 16",
			rules.Password.MinLength)
	}
}

// The legal links resolve workspace -> plan -> installation; a plan's lock holds the workspace's
// link off, and the installation's lock holds the plan's.
func TestTheLegalLinksResolveThroughThePlan(t *testing.T) {
	handler, workspace, instance := newRulesFixture()
	instance.level.Legal = domain.LegalLayer{
		Links: domain.LegalLinks{PrivacyURL: "https://host.example/privacy"},
		Locks: map[domain.LegalLink]bool{domain.LinkPrivacy: true},
	}
	workspace.row.Plan = onPlan(domain.PolicyLayer{}, domain.LegalLayer{
		Links: domain.LegalLinks{
			ImprintURL: "https://plan.example/imprint", PrivacyURL: "https://plan.example/privacy",
			TermsURL: "https://plan.example/terms",
		},
		Locks: map[domain.LegalLink]bool{domain.LinkTerms: true},
	})
	workspace.row.Settings.Legal = domain.LegalLinks{
		ImprintURL: "https://acme.example/imprint", TermsURL: "https://acme.example/terms",
	}

	resolved, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if resolved.Legal.ImprintURL != "https://acme.example/imprint" {
		t.Errorf("the open link answered %q, want the workspace's", resolved.Legal.ImprintURL)
	}
	if resolved.Legal.TermsURL != "https://plan.example/terms" {
		t.Errorf("the plan's locked link answered %q, want the plan's", resolved.Legal.TermsURL)
	}
	if resolved.Legal.PrivacyURL != "https://host.example/privacy" {
		t.Errorf("the installation's locked link answered %q, want the installation's", resolved.Legal.PrivacyURL)
	}
}

// identity.md §18: a plan that locks the ways in with providers among them turns the invite-only
// offer on in its workspaces as an installation lock does everywhere.
func TestAPlansLockToProvidersTurnsTheInviteOnlyOfferOn(t *testing.T) {
	f := newWayInFixture()
	methods := []string{domain.MethodOidc}
	f.passwords.workspace.row.Plan = onPlan(domain.PolicyLayer{
		Patch: domain.PolicyPatch{Methods: &methods},
		Locks: map[domain.PolicySwitch]bool{domain.SwitchMethods: true},
	}, domain.LegalLayer{})
	f.passwords.providers.rows = []domain.IdentityProvider{mandatedOffer()}

	if f.fallsBack(t) {
		t.Error("the fallback opened beside a provider the plan's lock turns on")
	}
	if !f.cardShows(t, fallbackRow) {
		t.Error("the card draws no button for the provider the plan's lock turns on")
	}
	if f.passwords.workspace.row.Settings.Offers(fallbackRow) {
		t.Error("the plan's mandate was written into the workspace's own switches")
	}
}

// Without the plan's lock the same offer stays the workspace's switch.
func TestWithoutAPlanLockTheInviteOnlyOfferStaysTheWorkspacesSwitch(t *testing.T) {
	f := newWayInFixture()
	methods := []string{domain.MethodOidc}
	f.passwords.workspace.row.Plan = onPlan(domain.PolicyLayer{
		Patch: domain.PolicyPatch{Methods: &methods},
	}, domain.LegalLayer{})
	f.passwords.providers.rows = []domain.IdentityProvider{mandatedOffer()}

	if f.cardShows(t, fallbackRow) {
		t.Error("an unlocked plan turned the invite-only offer on")
	}
}
