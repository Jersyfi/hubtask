// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// instanceSettings is the installation's level, in memory.
type instanceSettings struct {
	level   repository.InstanceLevel
	written []repository.InstanceLevel
	readErr error
}

func (s *instanceSettings) Read(context.Context) (repository.InstanceLevel, error) {
	return s.level, s.readErr
}

func (s *instanceSettings) Write(
	_ context.Context, level repository.InstanceLevel, _ shared.ID, _ time.Time,
) error {
	s.written = append(s.written, level)
	return nil
}

func newRulesFixture() (GetSignInRules, *workspaceStore, *instanceSettings) {
	workspace := &workspaceStore{row: domain.Workspace{
		Tenant: domain.Tenant{
			ID: tenant, Slug: "acme", DisplayName: "Acme", Status: domain.TenantActive,
		},
	}}
	instance := &instanceSettings{level: repository.InstanceLevel{
		Policy: domain.PolicyLayer{Locks: map[domain.PolicySwitch]bool{}},
		Legal:  domain.LegalLayer{Locks: map[domain.LegalLink]bool{}},
	}}
	handler := GetSignInRules{
		Resolver: SignInPolicyResolver{
			Workspaces: workspace, Instance: instance, UnitOfWork: &unitOfWork{},
		},
		Tenants:    tenantDirectory{single: tenant},
		Providers:  rulesProviders(),
		UnitOfWork: &unitOfWork{},
		Multi:      true,
	}
	return handler, workspace, instance
}

// An installation that has decided nothing answers the shipped rule: twelve characters, the two
// offline lists, and no composition rule anywhere.
func TestTheRulesAnswerTheShippedRuleWhereNobodyDecided(t *testing.T) {
	handler, _, _ := newRulesFixture()

	rules, err := handler.Execute(t.Context(), GetSignInRulesCommand{
		Host: "acme.hubtask.example", TenantSlug: "acme",
	})
	if err != nil {
		t.Fatalf("the rules were refused: %v", err)
	}

	if rules.Password.MinLength != domain.MinPasswordLength {
		t.Errorf("minimum length %d, want the shipped %d",
			rules.Password.MinLength, domain.MinPasswordLength)
	}
	if !rules.Password.CommonPasswords || !rules.Password.ContextWords {
		t.Error("the two offline lists are not on")
	}
	if rules.Password.MinUppercase != 0 || rules.Password.MinClasses != 0 || rules.Password.MaxRepeat != 0 {
		t.Error("a composition rule ships on")
	}
	if rules.WorkspaceHost != "acme.hubtask.example" {
		t.Errorf("host %q", rules.WorkspaceHost)
	}
}

// The three answers a guesser could use are not in it: the expiry, the history depth and the
// session bounds (ADR-0068 §7). The history is answered as zero to somebody with no account -
// a line under the field that can never be met is a line that only worries people.
func TestTheRulesWithholdWhatAGuesserCouldUse(t *testing.T) {
	handler, workspace, instance := newRulesFixture()
	instance.level.Policy.Patch = domain.PolicyPatch{
		MaxAgeDays: intOf(90), HistoryCount: intOf(5), SessionIdleMinutes: intOf(15),
	}
	workspace.row.Settings.SignIn = domain.PolicyPatch{HistoryCount: intOf(7)}

	rules, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("the rules were refused: %v", err)
	}

	if rules.Password.HistoryCount != 0 {
		t.Errorf("the history depth %d was answered to a caller with no account",
			rules.Password.HistoryCount)
	}
	if rules.Password.NotCurrent {
		t.Error("somebody with no password was told their password may not be the current one")
	}
}

// A workspace tightens, and what it tightened is what the screen predicts.
func TestTheWorkspacesOwnTighteningReachesTheScreen(t *testing.T) {
	handler, workspace, instance := newRulesFixture()
	instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(14)}
	workspace.row.Settings.SignIn = domain.PolicyPatch{MinLength: intOf(18), MinDigits: intOf(2)}

	rules, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("the rules were refused: %v", err)
	}
	if rules.Password.MinLength != 18 || rules.Password.MinDigits != 2 {
		t.Errorf("the rule answered %d characters and %d digits",
			rules.Password.MinLength, rules.Password.MinDigits)
	}
}

// A host no workspace answers at gets the installation's level - byte for byte what a workspace
// that has decided nothing answers, because which hosts hold workspaces is what a probe is after.
func TestAnUnknownHostIsIndistinguishableFromAnUndecidedWorkspace(t *testing.T) {
	handler, _, instance := newRulesFixture()
	instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(15)}

	known, err := handler.Execute(t.Context(), GetSignInRulesCommand{
		Host: "who.hubtask.example", TenantSlug: "acme",
	})
	if err != nil {
		t.Fatalf("the known host was refused: %v", err)
	}
	unknown, err := handler.Execute(t.Context(), GetSignInRulesCommand{
		Host: "who.hubtask.example", TenantSlug: "nobody",
	})
	if err != nil {
		t.Fatalf("an unknown host was refused rather than answered: %v", err)
	}

	if unknown.Password != known.Password {
		t.Errorf("an unknown host answered %+v, the known one %+v", unknown.Password, known.Password)
	}
	if unknown.WorkspaceHost != known.WorkspaceHost {
		t.Error("the two answers differ in the host they name")
	}
}

// The links resolve workspace -> instance -> nothing, and a private installation shows no line
// at all rather than four pointing nowhere.
func TestTheLegalLinksResolveThroughTheLevels(t *testing.T) {
	handler, workspace, instance := newRulesFixture()

	empty, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if !empty.Legal.IsEmpty() {
		t.Errorf("an installation that set nothing answered %+v", empty.Legal)
	}

	instance.level.Legal = domain.LegalLayer{
		Links: domain.LegalLinks{
			ImprintURL: "https://host.example/imprint", PrivacyURL: "https://host.example/privacy",
		},
		Locks: map[domain.LegalLink]bool{domain.LinkPrivacy: true},
	}
	workspace.row.Settings.Legal = domain.LegalLinks{
		ImprintURL: "https://acme.example/imprint", PrivacyURL: "https://acme.example/privacy",
	}

	resolved, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if resolved.Legal.ImprintURL != "https://acme.example/imprint" {
		t.Errorf("the open link answered %q, want the workspace's", resolved.Legal.ImprintURL)
	}
	if resolved.Legal.PrivacyURL != "https://host.example/privacy" {
		t.Errorf("the locked link answered %q, want the instance's", resolved.Legal.PrivacyURL)
	}
	if resolved.Legal.TermsURL != "" {
		t.Errorf("a link nobody set answered %q", resolved.Legal.TermsURL)
	}
}

// A provider nobody configured is not a way in, whatever the policy says: a button leading to a
// flow with no provider behind it is a button that answers an error.
func TestOidcIsNotOfferedWithoutAProvider(t *testing.T) {
	handler, _, _ := newRulesFixture()

	without, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	for _, method := range without.Methods {
		if method == domain.MethodOidc {
			t.Error("a provider was offered where none is configured")
		}
	}
	if len(without.Providers) != 0 {
		t.Errorf("providers %+v, want none", without.Providers)
	}

	handler.Providers = rulesProviders(domain.IdentityProvider{
		ID: rulesProviderRow, TenantID: rulesTenant, Kind: domain.KindMicrosoft,
		DisplayName: "login.microsoftonline.com",
		Issuer:      "https://login.microsoftonline.com/contoso/v2.0", Enabled: true,
	})
	with, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if len(with.Providers) != 1 || with.Providers[0].Kind != "MICROSOFT" {
		t.Fatalf("providers %+v", with.Providers)
	}
	if with.Providers[0].ID != rulesProviderRow.String() {
		t.Errorf("the button is keyed on %q, want the row", with.Providers[0].ID)
	}
	if with.Providers[0].Scope != "workspace" {
		t.Errorf("scope %q", with.Providers[0].Scope)
	}
	found := false
	for _, method := range with.Methods {
		found = found || method == domain.MethodOidc
	}
	if !found {
		t.Errorf("methods %v, want OIDC among them", with.Methods)
	}
}

// A provider a workspace switched off is not a way in either.
func TestADisabledProviderIsNotOffered(t *testing.T) {
	handler, _, _ := newRulesFixture()
	handler.Providers = rulesProviders(domain.IdentityProvider{
		ID: rulesProviderRow, TenantID: rulesTenant,
		Issuer: "https://id.acme.example", Enabled: false,
	})

	rules, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if len(rules.Providers) != 0 {
		t.Errorf("providers %+v, want none", rules.Providers)
	}
}

// `require_admin_totp` is what `mfa_required_for` derives from: a workspace that set the old
// boolean and never saw the new switch resolves to ADMINS, so no stored row has to move.
func TestTheOldBooleanStillDecidesTheNewSwitch(t *testing.T) {
	_, workspace, instance := newRulesFixture()
	workspace.row.Settings.RequireAdminTotp = true

	resolved := domain.Effective(
		instance.level.Policy, domain.PolicyLayer{}, workspace.row.Settings.SignInLayer())

	if resolved.Policy.MfaRequiredFor != domain.MfaForAdmins {
		t.Errorf("the requirement resolved to %q", resolved.Policy.MfaRequiredFor)
	}
}

func intOf(value int) *int { return &value }

// rulesProviderRow is the key of the row these fixtures configure, and `rulesTenant` is the
// workspace the slug resolves to - the row has to belong to it, or the fake's own read policy hides
// it, which is what the real policy would do as well.
const rulesProviderRow = shared.ID("01936f2a-7c1e-7000-8000-0000000000b1")

var rulesTenant = tenant

// rulesProviders is a store holding exactly these rows.
func rulesProviders(rows ...domain.IdentityProvider) *providerStore {
	store := newProviderStore(rulesTenant)
	store.rows = rows
	return store
}

// An installation's provider is **offered** to every workspace and is **on** in none of them until
// somebody there switches it on (SI-10, the concept's §8).
//
// That is the whole difference between offering and deciding: the installation says "this exists
// for you", and the workspace says whether its people see a button for it. A provider that appeared
// on by itself would be the installation deciding how a workspace signs in.
func TestAnInstallationsProviderIsNotAButtonUntilTheWorkspaceTakesIt(t *testing.T) {
	handler, workspace, _ := newRulesFixture()
	handler.Workspaces = workspace
	offered := shared.ID("01936f2a-7c1e-7000-8000-0000000000b2")
	handler.Providers = rulesProviders(
		domain.IdentityProvider{
			ID: offered, Kind: domain.KindGoogle,
			DisplayName: "Google", Issuer: "https://accounts.google.com", Enabled: true,
		},
		domain.IdentityProvider{
			ID: rulesProviderRow, TenantID: rulesTenant, Kind: domain.KindGeneric,
			DisplayName: "id.acme.example", Issuer: "https://id.acme.example", Enabled: true,
		},
	)

	rules, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if len(rules.Providers) != 1 || rules.Providers[0].Scope != ProviderScopeWorkspace {
		t.Fatalf("providers %+v, want only the workspace's own", rules.Providers)
	}

	// The workspace takes it, and now it is a button — saying which level it came from, so that a
	// settings screen can draw it as inherited rather than as this workspace's choice.
	workspace.row.Settings = workspace.row.Settings.WithOffer(offered, true)

	taken, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if len(taken.Providers) != 2 {
		t.Fatalf("providers %+v, want both levels", taken.Providers)
	}
	scopes := map[string]string{}
	for _, one := range taken.Providers {
		scopes[one.DisplayName] = one.Scope
	}
	if scopes["Google"] != ProviderScopeInstallation {
		t.Errorf("the installation's row is %q", scopes["Google"])
	}
	if scopes["id.acme.example"] != ProviderScopeWorkspace {
		t.Errorf("the workspace's own row is %q", scopes["id.acme.example"])
	}
}
