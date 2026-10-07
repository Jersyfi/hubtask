// SPDX-License-Identifier: Apache-2.0
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
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// ADR-0076 §4: no workspace is left without a way in. When no way into a workspace works - an offer
// that ended, or any other cause (E2) - the password opens again, for the accounts that hold
// one, under the workspace's own rules, until an administrator switches on another way. Nothing is
// weakened for an account without a password.

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

// The owner's decision of 2026-10-04 (E2): the fallback answers a workspace with no way in
// that works, whatever brought it there - not only an offer that ended. Each cause below is one the
// guards at the workspace's own doors cannot see, because nobody in the workspace made the change.
func TestThePasswordOpensWheneverNoWayInWorks(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(f *wayInFixture)
	}{
		{"an offer that ended", func(f *wayInFixture) {
			f.passwords.workspace.row.Settings = withdrawnOfferOnly(f.passwords.workspace.row.Settings)
			f.passwords.providers.rows = []domain.IdentityProvider{offeredUntil(now.Add(-time.Hour))}
		}},
		{"an installation default without the password", func(f *wayInFixture) {
			f.passwords.instance.level.Policy.Patch.Methods = providersAlone()
		}},
		// A lock decides the methods, never which provider is on: the installation's provider offered
		// beside it is a way in nowhere until a workspace takes it (offeredHere reads no lock).
		{"an installation lock on the provider alone", func(f *wayInFixture) {
			f.passwords.workspace.row.Settings.SignIn.Methods = &[]string{domain.MethodDirect, domain.MethodOidc}
			f.passwords.instance.level.Policy.Patch.Methods = providersAlone()
			f.passwords.instance.level.Policy.Locks[domain.SwitchMethods] = true
			f.passwords.providers.rows = []domain.IdentityProvider{offeredUntil(time.Time{})}
		}},
		{"a restore or an import that brought the settings without the providers", func(f *wayInFixture) {
			f.passwords.workspace.row.Settings.SignIn.Methods = providersAlone()
			f.passwords.providers.rows = []domain.IdentityProvider{offeredUntil(time.Time{})}
		}},
		// Each administrator's guard saw the other way still on; together they switched off both.
		{"two administrators who switched off the last two ways at once", func(f *wayInFixture) {
			f.passwords.workspace.row.Settings.SignIn.Methods = providersAlone()
			own := workspaceProvider(rulesProviderRow)
			own.Enabled = false
			f.passwords.providers.rows = []domain.IdentityProvider{own}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newWayInFixture()
			c.arrange(f)

			if !f.fallsBack(t) {
				t.Fatal("a workspace with no way in that works did not fall back to the password")
			}
			if result := f.signsIn(t, "correct horse battery"); result.Pair == nil && result.Challenge == nil {
				t.Fatal("an account with a password did not sign in through the fallback")
			}
			if !f.recordedFallback() {
				t.Error("the sign-in through the fallback is not in the workspace's trail")
			}

			// It ends the moment a way in is switched on: the password is the workspace's choice again,
			// and the workspace chose to switch it off.
			f.passwords.providers.rows = append(f.passwords.providers.rows, workspaceProvider(switchedOnRow))
			if f.fallsBack(t) {
				t.Error("the fallback outlived a way in switched on")
			}
			_, err := SignIn{Writer: f.session.writer}.Execute(t.Context(), SignInCommand{
				Email: "bert@example.org", Password: secret.New("correct horse battery"),
			})
			if detailOf(err) != "auth.password_not_offered" {
				t.Errorf("with a way in switched on the password answered %v", err)
			}
		})
	}
}

// ADR-0077 §4's rescue for a workspace whose provider cannot be reached: the operator locks the
// methods with the password among them. Lifted after the workspace's provider went, the workspace's
// own rule returns with nothing behind it - and the password stays open, as the fallback now.
func TestARescueLockLiftedAfterTheProviderWentLeavesThePasswordOpen(t *testing.T) {
	f := newWayInFixture()
	f.passwords.workspace.row.Settings.SignIn.Methods = providersAlone()
	f.passwords.providers.rows = []domain.IdentityProvider{workspaceProvider(rulesProviderRow)}
	f.passwords.instance.level.Policy.Patch.Methods = &[]string{domain.MethodDirect, domain.MethodOidc}
	f.passwords.instance.level.Policy.Locks[domain.SwitchMethods] = true

	door := func() (bool, bool) {
		t.Helper()
		open, fallback, err := f.passwords.writer.PasswordOpen(t.Context(), tenant)
		if err != nil {
			t.Fatalf("asking the door: %v", err)
		}
		return open, fallback.Opens()
	}
	if open, fallback := door(); !open || fallback {
		t.Fatalf("under the rescue lock the door answered open %v, fallback %v", open, fallback)
	}
	f.passwords.providers.rows = nil
	if open, fallback := door(); !open || fallback {
		t.Fatalf("with the provider gone under the lock the door answered open %v, fallback %v", open, fallback)
	}

	f.passwords.instance.level.Policy = domain.PolicyLayer{Locks: map[domain.PolicySwitch]bool{}}
	if !f.fallsBack(t) {
		t.Error("the lifted lock left the workspace with no way in and no fallback")
	}
}

// A workspace provisioned under an installation default without the password, whose owner is still
// invited: nobody has switched a provider on yet, because nobody is in to switch it. The owner accepts
// the invitation with a password under the fallback, and the trail says so - in the redemption's own
// transaction, beside the redemption (E2).
func TestAnInvitedOwnerOfAWorkspaceWithNoWayInAcceptsWithAPassword(t *testing.T) {
	f := newWayInFixture()
	f.passwords.instance.level.Policy.Patch.Methods = providersAlone()

	f.redeems(t)
	if f.session.accounts.redeemedHash == "" {
		t.Error("the redemption stored no password")
	}
	if !f.recordedFallback() {
		t.Errorf("the redemption through the fallback is not in the trail: %v", f.session.audit.entries)
	}
}

func TestAnInvitationRedeemedWhereThePasswordIsOnIsNoFallback(t *testing.T) {
	f := newWayInFixture()

	f.redeems(t)
	for _, entry := range f.session.audit.entries {
		if entry.Action == PasswordFallbackAction {
			t.Errorf("a workspace with the password on recorded a fallback: %+v", entry)
		}
	}
}

// redeems accepts the fixture's waiting invitation with a password, through the password writer's door.
func (f *wayInFixture) redeems(t *testing.T) {
	t.Helper()
	token := redemptionToken(t)
	f.session.accounts.redemption[token.Secret()] = waitingRedemption()
	passwords := f.passwords.writer
	if _, err := (RedeemInvitation{Writer: f.session.writer, Passwords: &passwords}).Execute(t.Context(),
		RedeemInvitationCommand{
			Token: secret.New(token.Secret()), Password: secret.New("seven blue lanterns over the harbour"),
		}); err != nil {
		t.Fatalf("redeeming: %v", err)
	}
}

// switchedOnRow is a provider an administrator switches on to end the fallback.
const switchedOnRow = shared.ID("01936f2a-7c1e-7000-8000-0000000000e6")

func providersAlone() *[]string {
	methods := []string{domain.MethodOidc}
	return &methods
}

// workspaceProvider is a provider of the workspace's own, switched on.
func workspaceProvider(id shared.ID) domain.IdentityProvider {
	return domain.IdentityProvider{
		ID: id, TenantID: tenant, Kind: domain.KindGeneric,
		DisplayName: "id.acme.example", Issuer: "https://id.acme.example", Enabled: true,
	}
}

// wayInFixture is one workspace read by the sign-in card and by the password's doors through the same
// fakes, so a test can hold the two to one answer.
type wayInFixture struct {
	*stepFixture
	card GetSignInRules
}

func newWayInFixture() *wayInFixture {
	f := newStepFixture(now)
	return &wayInFixture{stepFixture: f, card: GetSignInRules{
		Resolver: f.passwords.writer.Resolver, Tenants: tenantDirectory{single: tenant},
		Providers: f.passwords.providers, Workspaces: f.passwords.workspace,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), Multi: true,
	}}
}

// fallsBack answers whether the password is open as the fallback, and fails the test where the card
// and the door disagree about it.
func (f *wayInFixture) fallsBack(t *testing.T) bool {
	t.Helper()
	rules, err := f.card.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("reading the card: %v", err)
	}
	open, fallback, err := f.passwords.writer.PasswordOpen(t.Context(), tenant)
	if err != nil {
		t.Fatalf("asking the door: %v", err)
	}
	if rules.PasswordFallback != fallback.Opens() {
		t.Errorf("the card says fallback %v and the door %q", rules.PasswordFallback, fallback)
	}
	if fallback.Opens() && (!open || !slices.Equal(rules.Methods, []string{domain.MethodDirect})) {
		t.Errorf("under the fallback the door is open %v and the card offers %v", open, rules.Methods)
	}
	return fallback.Opens()
}

// recordedFallback answers whether the workspace's trail holds a fallback entry for the account, and
// says why the password was open.
func (f *wayInFixture) recordedFallback() bool {
	for _, entry := range f.session.audit.entries {
		if entry.Action == PasswordFallbackAction && entry.TenantID == tenant && entry.ActorID == account {
			return fallbackCause(entry) == string(FallbackCauseNoWayIn)
		}
	}
	return false
}

// fallbackCause reads the `cause` a fallback entry carries.
func fallbackCause(entry audit.Entry) any {
	cause, _ := entry.Changes["cause"].(map[string]any)
	return cause["to"]
}

// fallbackFixture is a sign-in into a workspace whose only way in was withdrawn an hour ago.
func fallbackFixture(t *testing.T) *stepFixture {
	t.Helper()
	f := newStepFixture(now)
	f.passwords.workspace.row.Settings = withdrawnOfferOnly(f.passwords.workspace.row.Settings)
	f.passwords.providers.rows = []domain.IdentityProvider{offeredUntil(now.Add(-time.Hour))}
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
			recorded = entry.TenantID == tenant && entry.ActorID == account &&
				fallbackCause(entry) == string(FallbackCauseNoWayIn)
		}
	}
	if !recorded {
		t.Errorf("the trail holds no fallback entry for the account, with its cause: %v",
			f.session.audit.entries)
	}
}

// One sign-in asks once whether the password is open as the fallback, and records what that one read
// answered: the door's read and the trail's cannot disagree, because there is no second (E2).
func TestASignInReadsTheFallbackOnce(t *testing.T) {
	f := fallbackFixture(t)

	if result := f.signsIn(t, "correct horse battery"); result.Pair == nil {
		t.Fatal("an account with a password did not sign in through the fallback")
	}
	if reads := f.passwords.providers.lists; reads != 1 {
		t.Errorf("one sign-in read the ways in %d times, want once", reads)
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
