// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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

			rules, err := handler.Execute(t.Context(), GetSignInRulesCommand{Host: "acme.hubtask.example"})
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

	rules, err := handler.Execute(t.Context(), GetSignInRulesCommand{Host: "acme.hubtask.example"})
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

	resolved, err := handler.Execute(t.Context(), GetSignInRulesCommand{Host: "acme.hubtask.example"})
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

// The workspace's screen names the plan and says which values and locks are the plan's, so "Set by
// your plan" refers to something a person can find (UC-ID-12 check 4).
func TestTheWorkspacePolicyProjectionNamesThePlan(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	fixture.store.row.Plan = onPlan(domain.PolicyLayer{
		Patch: domain.PolicyPatch{MinLength: intOf(16)},
		Locks: map[domain.PolicySwitch]bool{domain.SwitchMinLength: true},
	}, domain.LegalLayer{Links: domain.LegalLinks{TermsURL: "https://plan.example/terms"}})

	resolved, err := fixture.writer.Resolver.Resolve(t.Context(), tenant)
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	out := signInPolicyOutput(resolved)

	if out["plan"] != "Team" {
		t.Errorf("the plan reads %v, want its display name", out["plan"])
	}
	password, _ := out["password"].(usecase.Output)
	locked, _ := password["min_length"].(usecase.Output)
	if locked["value"] != 16 || locked["lock"] != "PLAN" || locked["installation_source"] != "PLAN" {
		t.Errorf("the plan's locked switch reads %v", locked)
	}
	legal, _ := out["legal"].(usecase.Output)
	terms, _ := legal["terms_url"].(usecase.Output)
	if terms["value"] != "https://plan.example/terms" || terms["installation_source"] != "PLAN" {
		t.Errorf("the plan's terms read %v", terms)
	}

	fixture.store.row.Plan = domain.WorkspacePlan{}
	resolved, err = fixture.writer.Resolver.Resolve(t.Context(), tenant)
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if plan := signInPolicyOutput(resolved)["plan"]; plan != nil {
		t.Errorf("a workspace on no plan names %v", plan)
	}
}

// UC-ID-12 check 4: a workspace's write is held to its plan as to the installation - a value looser
// than the plan's is refused, and a switch the plan locks is refused naming the plan.
func TestAWorkspacesWriteIsHeldToItsPlan(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	registry := workspaceRegistry(t, fixture)
	fixture.store.row.Plan = onPlan(domain.PolicyLayer{
		Patch: domain.PolicyPatch{MinLength: intOf(16), ManagedAccounts: boolOf(false), PrivateHubs: boolOf(false)},
		Locks: map[domain.PolicySwitch]bool{domain.SwitchMinDigits: true},
	}, domain.LegalLayer{})

	write := func(change map[string]any) error {
		_, err := registry.Invoke(t.Context(), UpdateWorkspaceName, workspaceActor(),
			usecase.Input{"sign_in_policy": change, "step_up_token": "hbt_stp_x"})
		return err
	}
	for name, change := range map[string]map[string]any{
		"a length below the plan's":     {"min_length": 12},
		"managed accounts the plan off": {"managed_accounts": true},
		"private hubs the plan off":     {"private_hubs": true},
	} {
		if detail := detailOf(write(change)); detail != "auth.policy_loosens" {
			t.Errorf("%s answered %q, want auth.policy_loosens", name, detail)
		}
	}
	err := write(map[string]any{"min_digits": 2})
	if detailOf(err) != "auth.policy_locked" || shared.AsError(err).Params["origin"] != string(domain.LockPlan) {
		t.Errorf("a switch the plan locks answered %v, want auth.policy_locked naming PLAN", err)
	}
	if len(fixture.store.updates) != 0 {
		t.Errorf("%d writes from refused changes", len(fixture.store.updates))
	}
	if err := write(map[string]any{"min_length": 20}); err != nil {
		t.Errorf("tightening below the plan was refused: %v", err)
	}
}

// UC-ID-20 check 8: a plan that switches managed accounts off refuses the next one.
func TestAPlanThatSwitchesManagedAccountsOffRefusesTheNext(t *testing.T) {
	f := newManagedFixture(t)
	f.policy.store.row.Plan = onPlan(domain.PolicyLayer{
		Patch: domain.PolicyPatch{ManagedAccounts: boolOf(false)},
	}, domain.LegalLayer{})

	_, err := f.create(t, admin(), lenaInput())
	if got := detailOf(err); got != "accounts.managed_accounts_off" {
		t.Fatalf("answered %q (%v), want accounts.managed_accounts_off", got, err)
	}
	if len(f.accounts.inserted) != 0 {
		t.Error("something was written")
	}
}

// UC-ID-16 check 7: a plan that switches private hubs off withdraws the offer of a private hub.
func TestAPlanThatSwitchesPrivateHubsOffWithdrawsTheOffer(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	fixture.store.row.Plan = onPlan(domain.PolicyLayer{
		Patch: domain.PolicyPatch{PrivateHubs: boolOf(false)},
	}, domain.LegalLayer{})
	resolved, err := fixture.writer.Resolver.Resolve(t.Context(), tenant)
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	if privateHubsAllowed(&resolved) {
		t.Error("private hubs are offered under a plan that switched them off")
	}
}

// UC-INS-13 check 4: moving a workspace to a plan with other terms puts other terms in force, which
// is what asks everybody once more; the provider's terms answer as the installation's do.
func TestAMoveToAPlanWithOtherTermsPutsThemInForce(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	terms := func(url string) domain.WorkspacePlan {
		return onPlan(domain.PolicyLayer{}, domain.LegalLayer{Links: domain.LegalLinks{TermsURL: url}})
	}
	fixture.store.row.Plan = terms("https://provider.example/family-terms")
	before, err := fixture.writer.Resolver.TermsInForce(t.Context(), tenant)
	if err != nil {
		t.Fatalf("reading the terms: %v", err)
	}
	fixture.store.row.Plan = terms("https://provider.example/business-terms")
	after, err := fixture.writer.Resolver.TermsInForce(t.Context(), tenant)
	if err != nil {
		t.Fatalf("reading the terms: %v", err)
	}
	if before.URL != "https://provider.example/family-terms" || after.URL != "https://provider.example/business-terms" {
		t.Errorf("the terms in force read %q, then %q", before.URL, after.URL)
	}
	if after.SetBy != domain.TermsByInstallation {
		t.Errorf("a plan's terms answer set by %q, want the installation's", after.SetBy)
	}
}
