// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"slices"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// UC-INS-11 check 4 (identity.md §18): locking the installation's ways in with providers among them
// turns each offered provider admitted as *Only people invited here* on in every workspace; one
// admitted by domain or to anyone stays each workspace's switch. Read, never written: the
// workspace's own switch is left as it is, and lifting the lock returns each workspace to it.

// mandatedOffer is the installation's invite-only row, offered without an end and switched on in
// no workspace.
func mandatedOffer() domain.IdentityProvider {
	row := offeredUntil(time.Time{})
	row.Provisioning = domain.ProvisionInvitedOnly
	return row
}

// lockMethods locks the installation's ways in to the methods given.
func lockMethods(f *wayInFixture, methods ...string) {
	f.passwords.instance.level.Policy.Patch.Methods = &methods
	f.passwords.instance.level.Policy.Locks[domain.SwitchMethods] = true
}

// cardShows answers whether the sign-in card draws the provider's button.
func (f *wayInFixture) cardShows(t *testing.T, id shared.ID) bool {
	t.Helper()
	rules, err := f.card.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
	if err != nil {
		t.Fatalf("reading the card: %v", err)
	}
	return slices.ContainsFunc(rules.Providers, func(p ProviderSummary) bool { return p.ID == id.String() })
}

// Where the mandated provider is on, the password fallback does not open: the provider is a way in
// that works, in a workspace that never switched it on.
func TestALockToProvidersTurnsTheInviteOnlyOfferOnAndKeepsTheFallbackShut(t *testing.T) {
	f := newWayInFixture()
	lockMethods(f, domain.MethodOidc)
	f.passwords.providers.rows = []domain.IdentityProvider{mandatedOffer()}

	if f.fallsBack(t) {
		t.Error("the fallback opened beside a provider the installation's lock turns on")
	}
	if !f.cardShows(t, fallbackRow) {
		t.Error("the card draws no button for the provider the lock turns on")
	}
	_, err := SignIn{Writer: f.session.writer}.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
	})
	if detailOf(err) != "auth.password_not_offered" {
		t.Errorf("under the mandate the password answered %v, want auth.password_not_offered", err)
	}
	if f.passwords.workspace.row.Settings.Offers(fallbackRow) {
		t.Error("the mandate was written into the workspace's own switches")
	}
}

// A lock that keeps the password among the methods mandates the offer as well: it adds a way in and
// takes none.
func TestALockWithThePasswordAndProvidersAddsTheMandatedWay(t *testing.T) {
	f := newWayInFixture()
	lockMethods(f, domain.MethodDirect, domain.MethodOidc)
	f.passwords.providers.rows = []domain.IdentityProvider{mandatedOffer()}

	open, cause := f.door(t)
	if !open || cause.Opens() {
		t.Errorf("the password answered open %v, cause %q; want open as a method", open, cause)
	}
	if !f.cardShows(t, fallbackRow) {
		t.Error("the card draws no button for the provider the lock turns on")
	}
}

// Which locks and which admissions mandate, read where the card and the door read them.
func TestOnlyAnInviteOnlyOfferUnderALockWithProvidersIsOnEverywhere(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(f *wayInFixture)
		on      bool
	}{
		{"no lock", func(f *wayInFixture) {
			f.passwords.instance.level.Policy.Patch.Methods = providersAlone()
		}, false},
		{"a lock on the password alone", func(f *wayInFixture) { lockMethods(f, domain.MethodDirect) }, false},
		{"a lock on providers alone", func(f *wayInFixture) { lockMethods(f, domain.MethodOidc) }, true},
		{"a lock on both", func(f *wayInFixture) { lockMethods(f, domain.MethodDirect, domain.MethodOidc) }, true},
		{"a lock on providers alone, admitted by domain", func(f *wayInFixture) {
			lockMethods(f, domain.MethodOidc)
			f.passwords.providers.rows[0].Provisioning = domain.ProvisionDomains
		}, false},
		{"a lock on providers alone, admitting anyone", func(f *wayInFixture) {
			lockMethods(f, domain.MethodOidc)
			f.passwords.providers.rows[0].Provisioning = domain.ProvisionAny
		}, false},
		{"a lock on providers alone, the offer withdrawn", func(f *wayInFixture) {
			lockMethods(f, domain.MethodOidc)
			f.passwords.providers.rows[0].WithdrawAt = now.Add(-time.Hour)
		}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newWayInFixture()
			f.passwords.providers.rows = []domain.IdentityProvider{mandatedOffer()}
			c.arrange(f)
			if got := f.cardShows(t, fallbackRow); got != c.on {
				t.Errorf("the card draws the offer: %v, want %v", got, c.on)
			}
		})
	}
}

// The operator's opening (ADR-0078 §3) answers a provider that is on and broken - a mandated one
// included: it overrides any installation lock.
func TestAnOperatorsOpeningStillOpensBesideAMandatedProvider(t *testing.T) {
	f := newWayInFixture()
	lockMethods(f, domain.MethodOidc)
	f.passwords.providers.rows = []domain.IdentityProvider{mandatedOffer()}
	operatorOpened(f.stepFixture, now.Add(time.Hour))

	if open, cause := f.door(t); !open || cause != FallbackCauseOperator {
		t.Errorf("the door answered open %v, cause %q; want the operator's opening", open, cause)
	}
}

// A withdrawal still ends a mandated provider, and the workspace falls back to the password; lifting
// the lock returns each workspace to its own switch at once, where nobody switched it on.
func TestTheMandateEndsWithTheOfferAndWithTheLock(t *testing.T) {
	f := newWayInFixture()
	lockMethods(f, domain.MethodOidc)
	f.passwords.providers.rows = []domain.IdentityProvider{mandatedOffer()}
	f.passwords.providers.rows[0].WithdrawAt = now.Add(-time.Hour)
	if !f.fallsBack(t) {
		t.Error("a withdrawn mandated provider kept the fallback shut")
	}

	lifted := newWayInFixture()
	lifted.passwords.workspace.row.Settings.SignIn.Methods = providersAlone()
	lifted.passwords.providers.rows = []domain.IdentityProvider{mandatedOffer()}
	if !lifted.fallsBack(t) {
		t.Error("without the lock the provider nobody switched on is still a way in")
	}
	lifted.passwords.workspace.row.Settings = lifted.passwords.workspace.row.Settings.WithOffer(fallbackRow, true)
	if lifted.fallsBack(t) {
		t.Error("the workspace's own switch is not read once the lock is lifted")
	}
}

// An unreadable installation level is an error, never "no lock": a reader that guessed would draw a
// card the door then contradicts.
func TestAnUnreadableInstallationLevelIsAnError(t *testing.T) {
	f := newWayInFixture()
	f.passwords.instance.readErr = errors.New("the database is gone")
	if _, err := readWaysHere(t.Context(), f.passwords.workspace, f.passwords.instance); err == nil {
		t.Error("an unreadable installation level read as no lock")
	}
}

// The provider sign-in reads the mandate as the card does: a flow through the mandated provider
// starts in a workspace that never switched it on, and one admitted by domain does not.
func TestASignInThroughAMandatedProviderStarts(t *testing.T) {
	at := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)
	installationRow := shared.ID("01936f2a-7c1e-7000-8000-0000000000c3")
	configureFixtureProvider(t, f, installationRow, "", "https://login.platform.example", at)
	f.writer.Workspaces = &workspaceStore{row: domain.Workspace{
		Tenant: domain.Tenant{ID: tenant, Slug: "acme", Status: domain.TenantActive},
	}}
	instance := &instanceSettings{}
	instance.level.Policy = domain.PolicyLayer{
		Patch: domain.PolicyPatch{Methods: providersAlone()},
		Locks: map[domain.PolicySwitch]bool{domain.SwitchMethods: true},
	}
	f.writer.Instance = instance

	startWith := func(mode domain.Provisioning) error {
		for i, row := range f.store.rows {
			if row.ID == installationRow {
				f.store.rows[i].Provisioning = mode
			}
		}
		_, err := StartOidcSignIn{Writer: f.writer}.
			Execute(t.Context(), StartOidcSignInCommand{ProviderID: installationRow})
		return err
	}
	if err := startWith(domain.ProvisionDomains); detailOf(err) != "identity_provider.disabled" {
		t.Errorf("a provider admitted by domain under the lock answered %v, want identity_provider.disabled", err)
	}
	if err := startWith(domain.ProvisionInvitedOnly); err != nil {
		t.Errorf("the mandated provider did not start: %v", err)
	}
}

// The step-up offers a mandated provider the account is connected to, as any provider on here.
func TestAMandatedProviderIsAStepUp(t *testing.T) {
	f := newWayInFixture()
	lockMethods(f, domain.MethodOidc)
	f.passwords.providers.rows = []domain.IdentityProvider{mandatedOffer()}
	steps := ProviderStepUps{
		Providers: f.passwords.providers, External: connectedTo{providers: []shared.ID{fallbackRow}},
		Workspaces: f.passwords.workspace, Instance: f.passwords.instance,
	}
	if _, reaches, err := steps.connectedHere(t.Context(), account, now); err != nil || !reaches {
		t.Errorf("the mandated provider is no step-up here: reaches %v, %v", reaches, err)
	}
}
