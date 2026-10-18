// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A plan that leaves out the workspace's own sign-in providers (multi-tenancy.md §4.2): they sign
// nobody in, their rows and secrets stay, the installation's providers go on, and where nothing
// else is left the password opens (identity.md §17.1). Included again, they sign people in again.

var withoutOwnProviders = domain.WorkspacePlan{
	Name: "private", DisplayName: "Private", LeftOut: entitlement.LeftOut{entitlement.OwnSignInProviders},
}

// The workspace's own provider, its last way in, rests: the card and the door agree that the
// password opens, an account with one signs in through it, and the provider's row is untouched.
// Included again, the provider is the way in and the fallback ends.
func TestThePasswordOpensWhereThePlanLeftOutTheLastWayIn(t *testing.T) {
	f := newWayInFixture()
	f.passwords.workspace.row.Settings.SignIn.Methods = providersAlone()
	own := workspaceProvider(rulesProviderRow)
	f.passwords.providers.rows = []domain.IdentityProvider{own}
	f.passwords.workspace.row.Plan = withoutOwnProviders

	if !f.fallsBack(t) {
		t.Fatal("a workspace whose plan left out its only way in did not fall back to the password")
	}
	if result := f.signsIn(t, "correct horse battery"); result.Pair == nil && result.Challenge == nil {
		t.Fatal("an account with a password did not sign in through the fallback")
	}
	if !f.recordedFallback() {
		t.Error("the sign-in through the fallback is not in the workspace's trail")
	}
	if kept := f.passwords.providers.rows[0]; !kept.Enabled || kept.ID != own.ID {
		t.Errorf("resting changed the provider's row: %+v", kept)
	}

	f.passwords.workspace.row.Plan = domain.WorkspacePlan{}
	if f.fallsBack(t) {
		t.Error("included again, the fallback outlived the provider")
	}
}

// The sign-in itself refuses a resting own provider, whatever identifier a caller sends, as it
// refuses one switched off - and the installation's provider this workspace took goes on.
func TestARestingOwnProviderSignsNobodyIn(t *testing.T) {
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)
	ownRow := shared.ID("01936f2a-7c1e-7000-8000-0000000005a1")
	installationRow := shared.ID("01936f2a-7c1e-7000-8000-0000000005a2")
	configureFixtureProvider(t, f, ownRow, tenant, "https://id.acme.example", at)
	configureFixtureProvider(t, f, installationRow, "", "https://login.platform.example", at)
	workspaces := &workspaceStore{row: domain.Workspace{
		Tenant: domain.Tenant{ID: tenant, Slug: "acme", Status: domain.TenantActive},
		Plan:   withoutOwnProviders,
	}}
	workspaces.row.Settings = workspaces.row.Settings.WithOffer(installationRow, true)
	f.writer.Workspaces = workspaces

	startWith := func(id shared.ID) error {
		_, err := StartOidcSignIn{Writer: f.writer}.Execute(t.Context(), StartOidcSignInCommand{ProviderID: id})
		return err
	}
	if err := startWith(ownRow); detailOf(err) != "identity_provider.disabled" {
		t.Errorf("a resting own provider answered %v, want identity_provider.disabled", err)
	}
	if err := startWith(installationRow); err != nil {
		t.Errorf("the installation's provider stopped with the workspace's own: %v", err)
	}

	workspaces.row.Plan = domain.WorkspacePlan{}
	if err := startWith(ownRow); err != nil {
		t.Errorf("included again, the own provider did not start: %v", err)
	}
}

// Resting, an own provider is not switched on - refused naming the feature - and may still be
// switched off; the singular read says it is no way in here.
func TestARestingOwnProviderIsNotSwitchedOnAndReadsAsNoWayIn(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC))
	off := f.own(t, false)
	f.workspaces.row.Plan = withoutOwnProviders
	f.writer.Workspaces = f.workspaces

	_, err := f.offer.Execute(t.Context(), providerActor(), off, true, "")
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != entitlement.NotIncludedCode ||
		refusal.Params["feature"] != string(entitlement.OwnSignInProviders) {
		t.Fatalf("switching a resting own provider on answered %v", err)
	}
	if f.store.rows[0].Enabled {
		t.Fatal("the refusal switched the provider on")
	}

	f.store.rows[0].Enabled = true
	first, err := ReadIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor())
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if !first.Enabled || first.OfferedHere {
		t.Errorf("a resting own provider read as enabled %v, a way in %v", first.Enabled, first.OfferedHere)
	}

	// A password keeps the workspace reachable, so switching it off is not the last way in.
	f.workspaces.row.Settings.SignIn.Methods = &[]string{domain.MethodDirect, domain.MethodOidc}
	switched, err := f.offer.Execute(t.Context(), providerActor(), off, false, "")
	if err != nil || switched.Enabled || switched.OfferedHere {
		t.Fatalf("switching a resting own provider off answered %+v, %v", switched, err)
	}
}

// The password cannot be switched off where only a resting own provider is left: it signs nobody in,
// so it is no other way in.
func TestThePasswordIsTheLastWayInBesideARestingOwnProvider(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	providers := newProviderStore(tenant)
	providers.rows = append(providers.rows, workspaceProvider(rulesProviderRow))
	fixture.writer.Providers = providers
	fixture.store.row.Plan = withoutOwnProviders
	onlyProviders := []string{domain.MethodOidc}

	_, err := UpdateWorkspace{Writer: fixture.writer}.Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{
			SignIn:      WorkspacePolicyChange{Policy: domain.PolicyPatch{Methods: &onlyProviders}},
			StepUpToken: "hbt_stp_x",
		})
	refusedAsLastWayIn(t, err, "switching the password off beside a resting own provider")
}

// A flow begun at an own provider before the plan left own providers out comes back to nothing: the
// callback reads the plan again and refuses as for a provider switched off, and the identity
// connected meanwhile stays. Included again, the same person signs in through it.
func TestACallbackAtARestingOwnProviderSignsNobodyIn(t *testing.T) {
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	ada := domain.Account{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000005b2"), TenantID: tenant,
		Kind: domain.AccountUser, Email: "ada@example.org", DisplayName: "Ada", Status: domain.AccountActive,
	}
	f := newOidcFixture(t, at, ada)
	f.store.rows = nil
	ownRow := shared.ID("01936f2a-7c1e-7000-8000-0000000005b1")
	configureFixtureProvider(t, f, ownRow, tenant, "https://id.acme.example", at)
	f.external.bySubject[linkKey(ownRow, "provider-subject-1")] = ada
	workspaces := &workspaceStore{row: domain.Workspace{Tenant: domain.Tenant{ID: tenant, Slug: "acme", Status: domain.TenantActive}}}
	f.writer.Workspaces = workspaces

	authorization, err := StartOidcSignIn{Writer: f.writer}.Execute(t.Context(), StartOidcSignInCommand{ProviderID: ownRow})
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	workspaces.row.Plan = withoutOwnProviders
	_, err = CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{Code: "one", State: authorization.State})
	if detailOf(err) != "identity_provider.disabled" {
		t.Fatalf("the callback at a resting own provider answered %v", err)
	}
	if len(f.external.bySubject) != 1 {
		t.Errorf("resting removed the connected identity")
	}

	workspaces.row.Plan = domain.WorkspacePlan{}
	again, err := StartOidcSignIn{Writer: f.writer}.Execute(t.Context(), StartOidcSignInCommand{ProviderID: ownRow})
	if err != nil {
		t.Fatalf("starting again: %v", err)
	}
	result, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{Code: "two", State: again.State})
	if err != nil || pairOf(result).Session.AccountID != ada.ID {
		t.Fatalf("included again, the callback answered %+v, %v", pairOf(result), err)
	}
}
