// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The date of ADR-0076, on the workspace's side: honoured where the offer is read (§2), so what is
// asserted is every reader - the list, the sign-in card, the sign-in itself, and the switch. The
// count is counted in the database where the installation reads it (ADR-0077 §1), and its test is
// the integration test of the provider store.

// detailOf is the refusal's message code, and empty where nothing was refused.
func detailOf(err error) string {
	if err == nil {
		return ""
	}
	return shared.AsError(err).DetailCode
}

// withdrawAt announces the installation row's withdrawal, as the operator's use case would.
func (f *offerFixture) withdrawAt(id shared.ID, at time.Time) {
	for i, row := range f.store.rows {
		if row.ID == id {
			f.store.rows[i].WithdrawAt = at
		}
	}
}

func TestAWithdrawnProviderIsAWayInNowhereFromItsDate(t *testing.T) {
	at := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	f := newOfferFixture(at)
	f.own(t, true)
	id := f.installation(t)
	f.workspaces.row.Settings = f.workspaces.row.Settings.WithOffer(id, true)
	f.withdrawAt(id, at.Add(time.Hour))

	list := func(now time.Time) domain.IdentityProvider {
		t.Helper()
		writer := f.writer
		writer.Workspaces = f.workspaces
		writer.Session.Clock = clock.Fixed(now)
		rows, err := ListIdentityProviders{Writer: writer}.Execute(t.Context(), providerActor())
		if err != nil {
			t.Fatalf("listing: %v", err)
		}
		for _, row := range rows {
			if row.ID == id {
				return row
			}
		}
		t.Fatal("the installation's row is not listed")
		return domain.IdentityProvider{}
	}

	// Until the date it keeps working, and the workspace can read when it ends.
	before := list(at)
	if !before.OfferedHere {
		t.Error("a provider whose withdrawal is still ahead is not a way in")
	}
	if !before.WithdrawAt.Equal(at.Add(time.Hour)) {
		t.Errorf("the listing answers withdraw_at %v", before.WithdrawAt)
	}

	// From the date it is a way in nowhere - the workspace's switch is still on, and is not read.
	if list(at.Add(time.Hour)).OfferedHere {
		t.Error("a withdrawn provider is still a way in")
	}
	if !f.workspaces.row.Settings.Offers(id) {
		t.Error("the withdrawal wrote the workspace's switch; it is honoured by reading, not by a job")
	}
}

func TestAWithdrawnProviderCannotBeSwitchedOnAgain(t *testing.T) {
	at := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	f := newOfferFixture(at)
	f.own(t, true)
	id := f.installation(t)
	f.withdrawAt(id, at)

	_, err := f.offer.Execute(t.Context(), providerActor(), id, true, "")
	if !errors.Is(err, shared.ErrValidation) || detailOf(err) != "identity_provider.withdrawn" {
		t.Fatalf("switching on a withdrawn provider answered %v, want identity_provider.withdrawn", err)
	}
	if f.workspaces.row.Settings.Offers(id) {
		t.Error("the refused switch was written")
	}
}

// The sign-in card resolves the same reading: the button is there until the date, and not after.
func TestTheSignInCardDropsAWithdrawnProviderOnItsDate(t *testing.T) {
	at := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	handler, workspace, _ := newRulesFixture()
	handler.Workspaces = workspace
	offered := shared.ID("01936f2a-7c1e-7000-8000-0000000000b2")
	handler.Providers = rulesProviders(domain.IdentityProvider{
		ID: offered, Kind: domain.KindGoogle, DisplayName: "Google",
		Issuer: "https://accounts.google.com", Enabled: true, WithdrawAt: at.Add(time.Hour),
	})
	workspace.row.Settings = workspace.row.Settings.WithOffer(offered, true)

	for _, probe := range []struct {
		now  time.Time
		want int
	}{{at, 1}, {at.Add(time.Hour), 0}} {
		handler.Clock = clock.Fixed(probe.now)
		rules, err := handler.Execute(t.Context(), GetSignInRulesCommand{TenantSlug: "acme"})
		if err != nil {
			t.Fatalf("reading the rules: %v", err)
		}
		if len(rules.Providers) != probe.want {
			t.Errorf("at %v the card shows %d providers, want %d", probe.now, len(rules.Providers), probe.want)
		}
	}
}

// The sign-in itself reads the offer the way the card does: an installation's provider this
// workspace did not take, or one whose withdrawal has come, opens no flow - whatever identifier a
// caller sends. The card not showing a button is not what refuses it.
func TestASignInThroughAnInstallationsProviderReadsTheWorkspacesOffer(t *testing.T) {
	at := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)
	installationRow := shared.ID("01936f2a-7c1e-7000-8000-0000000000c3")
	configureFixtureProvider(t, f, installationRow, "", "https://login.platform.example", at)
	workspaces := &workspaceStore{row: domain.Workspace{
		Tenant: domain.Tenant{ID: tenant, Slug: "acme", Status: domain.TenantActive},
	}}
	f.writer.Workspaces = workspaces

	startWith := func() error {
		_, err := StartOidcSignIn{Writer: f.writer}.
			Execute(t.Context(), StartOidcSignInCommand{ProviderID: installationRow})
		return err
	}

	if err := startWith(); detailOf(err) != "identity_provider.disabled" {
		t.Errorf("a provider this workspace never took answered %v, want identity_provider.disabled", err)
	}

	workspaces.row.Settings = workspaces.row.Settings.WithOffer(installationRow, true)
	if err := startWith(); err != nil {
		t.Errorf("a provider this workspace took did not start: %v", err)
	}

	for i, row := range f.store.rows {
		if row.ID == installationRow {
			f.store.rows[i].WithdrawAt = at
		}
	}
	if err := startWith(); detailOf(err) != "identity_provider.disabled" {
		t.Errorf("a withdrawn provider answered %v, want identity_provider.disabled", err)
	}
}

// UC-INS-11 check 5: withdrawing the offer turns it off everywhere and leaves the connected identities
// in place, so offering it again restores sign-in - the same person, through the same link, with
// nothing made anew.
func TestAWithdrawalKeepsTheConnectedIdentitiesAndOfferingAgainRestoresSignIn(t *testing.T) {
	at := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	ada := domain.Account{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e2"), TenantID: tenant,
		Kind: domain.AccountUser, Email: "ada@example.org", DisplayName: "Ada",
		Status: domain.AccountActive,
	}
	f := newOidcFixture(t, at, ada)
	f.store.rows = nil // the installation's provider is this workspace's only one
	installationRow := shared.ID("01936f2a-7c1e-7000-8000-0000000000c4")
	configureFixtureProvider(t, f, installationRow, "", "https://login.platform.example", at)
	// The identity connected while the offer stood: what a withdrawal must leave in place.
	f.external.bySubject[linkKey(installationRow, "provider-subject-1")] = ada
	workspaces := &workspaceStore{row: domain.Workspace{
		Tenant: domain.Tenant{ID: tenant, Slug: "acme", Status: domain.TenantActive},
	}}
	workspaces.row.Settings = workspaces.row.Settings.WithOffer(installationRow, true)
	f.writer.Workspaces = workspaces

	withdrawAt := func(moment time.Time) {
		for i, row := range f.store.rows {
			if row.ID == installationRow {
				f.store.rows[i].WithdrawAt = moment
			}
		}
	}
	arrive := func(code string) (SessionPair, error) {
		authorization, err := StartOidcSignIn{Writer: f.writer}.
			Execute(t.Context(), StartOidcSignInCommand{ProviderID: installationRow})
		if err != nil {
			return SessionPair{}, err
		}
		result, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(),
			CompleteOidcSignInCommand{Code: code, State: authorization.State})
		return pairOf(result), err
	}

	first, err := arrive("one")
	if err != nil {
		t.Fatalf("signing in through the offered provider: %v", err)
	}
	connected, accounts := len(f.external.bySubject), len(f.accounts.byID)

	// Withdrawn: a way in nowhere, and nothing connected to it is touched.
	withdrawAt(at)
	if _, err := arrive("two"); detailOf(err) != "identity_provider.disabled" {
		t.Fatalf("a withdrawn provider answered %v", err)
	}
	if len(f.external.bySubject) != connected {
		t.Errorf("the withdrawal removed connected identities: %d, were %d", len(f.external.bySubject), connected)
	}

	// Offered again - the cancellation clears the date - and the same person signs in.
	withdrawAt(time.Time{})
	again, err := arrive("three")
	if err != nil {
		t.Fatalf("signing in after the offer was restored: %v", err)
	}
	if again.Session.AccountID != first.Session.AccountID {
		t.Error("offering it again signed somebody else in")
	}
	if len(f.accounts.byID) != accounts {
		t.Errorf("restoring the offer made an account: %d, want %d", len(f.accounts.byID), accounts)
	}
}
