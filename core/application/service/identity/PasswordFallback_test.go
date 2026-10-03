// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"slices"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// ADR-0076 §4: no workspace is left without a way in. When the only way into a workspace was a
// provider the installation offered and that offer has ended, the password opens again - for the
// accounts that hold one, under the workspace's own rules - until an administrator switches on
// another way. Nothing is weakened for an account without a password.

const fallbackRow = shared.ID("01936f2a-7c1e-7000-8000-0000000000e5")

// withdrawnOfferOnly is a workspace that switched the password off and took the installation's provider.
func withdrawnOfferOnly(settings domain.WorkspaceSettings) domain.WorkspaceSettings {
	methods := []string{domain.MethodOidc}
	settings.SignIn.Methods = &methods
	return settings.WithOffer(fallbackRow, true)
}

// offeredUntil is the installation's row, withdrawn at the moment given (zero: offered without end).
func offeredUntil(at time.Time) domain.IdentityProvider {
	return domain.IdentityProvider{
		ID: fallbackRow, Kind: domain.KindGeneric, DisplayName: "The platform",
		Issuer: "https://login.platform.example", Enabled: true, WithdrawAt: at,
	}
}

func TestAWorkspaceWhoseLastWayWasWithdrawnShowsThePasswordAgain(t *testing.T) {
	at := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	handler, workspace, _ := newRulesFixture()
	handler.Workspaces = workspace
	workspace.row.Settings = withdrawnOfferOnly(workspace.row.Settings)
	store := rulesProviders(offeredUntil(at.Add(time.Hour)))
	handler.Providers = store

	read := func(now time.Time) SignInRules {
		t.Helper()
		handler.Clock = clock.Fixed(now)
		rules, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
		if err != nil {
			t.Fatalf("reading the rules: %v", err)
		}
		return rules
	}

	// Until the date the provider is the way in, and nothing falls back.
	before := read(at)
	if before.PasswordFallback || slices.Contains(before.Methods, domain.MethodDirect) {
		t.Errorf("before the date the card offers %v (fallback %v)", before.Methods, before.PasswordFallback)
	}

	// From the date the password opens again, and the card says why.
	after := read(at.Add(time.Hour))
	if !after.PasswordFallback || !slices.Equal(after.Methods, []string{domain.MethodDirect}) {
		t.Errorf("after the date the card offers %v (fallback %v)", after.Methods, after.PasswordFallback)
	}

	// An administrator who switches on another way ends the fallback.
	store.rows = append(store.rows, domain.IdentityProvider{
		ID: rulesProviderRow, TenantID: rulesTenant, Kind: domain.KindGeneric,
		DisplayName: "id.acme.example", Issuer: "https://id.acme.example", Enabled: true,
	})
	if ended := read(at.Add(time.Hour)); ended.PasswordFallback ||
		!slices.Equal(ended.Methods, []string{domain.MethodOidc}) {
		t.Errorf("with another way on the card offers %v (fallback %v)", ended.Methods, ended.PasswordFallback)
	}
}

// The fallback answers an ended offer, not a workspace that simply has no provider: one that never
// took the installation's provider was never left by it.
func TestAWorkspaceThatNeverTookTheWithdrawnProviderDoesNotFallBack(t *testing.T) {
	at := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	handler, workspace, _ := newRulesFixture()
	handler.Workspaces = workspace
	workspace.row.Settings = withdrawnOfferOnly(workspace.row.Settings).WithOffer(fallbackRow, false)
	handler.Providers = rulesProviders(offeredUntil(at))
	handler.Clock = clock.Fixed(at)

	rules, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("reading the rules: %v", err)
	}
	if rules.PasswordFallback || len(rules.Methods) != 0 {
		t.Errorf("the card offers %v (fallback %v)", rules.Methods, rules.PasswordFallback)
	}
}

// fallbackFixture is a sign-in into a workspace whose only way in was withdrawn an hour ago.
func fallbackFixture(t *testing.T) *stepFixture {
	t.Helper()
	f := newStepFixture(now)
	f.passwords.workspace.row.Settings = withdrawnOfferOnly(f.passwords.workspace.row.Settings)
	f.passwords.writer.WaysIn = WaysIn{
		Providers:  rulesProviders(offeredUntil(now.Add(-time.Hour))),
		Workspaces: f.passwords.workspace,
		UnitOfWork: &unitOfWork{},
		Clock:      clock.Fixed(now),
	}
	f.session.writer.Rule = f.passwords.writer
	return f
}

func TestAPasswordSignInThroughTheFallbackIsRecordedInTheWorkspacesTrail(t *testing.T) {
	f := fallbackFixture(t)

	if result := f.signsIn(t, "correct horse battery"); result.Pair == nil {
		t.Fatal("an account with a password did not sign in through the fallback")
	}
	var recorded bool
	for _, entry := range f.session.audit.entries {
		if entry.Action == PasswordFallbackAction {
			recorded = entry.TenantID == tenant && entry.ActorID == account
		}
	}
	if !recorded {
		t.Errorf("the trail holds no fallback entry for the account: %v", f.session.audit.entries)
	}
}

// Not every password sign-in is a fallback: where the workspace keeps the password on, nothing is
// recorded beyond the sign-in itself.
func TestAPasswordSignInWhereThePasswordIsOnIsNoFallback(t *testing.T) {
	f := fallbackFixture(t)
	f.passwords.workspace.row.Settings.SignIn.Methods = &[]string{domain.MethodDirect, domain.MethodOidc}

	f.signsIn(t, "correct horse battery")
	for _, entry := range f.session.audit.entries {
		if entry.Action == PasswordFallbackAction {
			t.Errorf("a workspace with the password on recorded a fallback: %+v", entry)
		}
	}
}

func TestAnAccountWithoutAPasswordGainsNothingFromTheFallback(t *testing.T) {
	f := fallbackFixture(t)
	f.session.accounts.byEmail["ada@example.org"] = repository.SignInAccount{
		TenantStatus: domain.TenantActive,
		Account: domain.Account{
			ID: account, TenantID: tenant, Kind: domain.AccountUser,
			Email: "ada@example.org", DisplayName: "Ada", Status: domain.AccountActive,
		},
		TenantLocale: "en", TenantTimeZone: "UTC",
	}

	_, err := SignIn{Writer: f.session.writer}.Execute(t.Context(), SignInCommand{
		Email: "ada@example.org", Password: secret.New("anything at all"),
	})
	if !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("an account without a password answered %v, want the sign-in refused", err)
	}
	for _, entry := range f.session.audit.entries {
		if entry.Action == PasswordFallbackAction {
			t.Error("a refused sign-in recorded a fallback")
		}
	}
}
