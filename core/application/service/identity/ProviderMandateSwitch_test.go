// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INS-11 check 4, the workspace's screen, through the registry the way REST, MCP and automation
// reach it: under a lock on the ways in with providers among them, the installation's invite-only
// row reads on and set by the installation, and the switch refuses to turn it off; one admitted by
// domain stays this workspace's switch.

// lockedOfferFixture is the offer fixture under an installation lock to providers alone, with the
// installation's invite-only row and one admitted by domain, neither switched on here.
func lockedOfferFixture(t *testing.T) (*offerFixture, *usecase.Registry, shared.ID, shared.ID) {
	t.Helper()
	f := newOfferFixture(time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC))
	instance := &instanceSettings{level: repository.InstanceLevel{Policy: domain.PolicyLayer{
		Patch: domain.PolicyPatch{Methods: providersAlone()},
		Locks: map[domain.PolicySwitch]bool{domain.SwitchMethods: true},
	}}}
	f.writer.Workspaces, f.writer.Instance = f.workspaces, instance
	f.offer.Writer = f.writer
	invited := f.installation(t)
	byDomain := shared.MustParseID("33333333-3333-4333-8333-333333333333")
	f.store.rows = append(f.store.rows, domain.IdentityProvider{
		ID: byDomain, Issuer: "https://login.microsoftonline.com/x/v2.0", ClientID: "hubtask",
		DisplayName: "Entra", Kind: domain.KindMicrosoft,
		Provisioning: domain.ProvisionDomains, Enabled: true, Version: 1,
	})
	registry, err := usecase.NewRegistry(nil,
		ListIdentityProviders{Writer: f.writer}.Descriptor(), f.offer.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	return f, registry, invited, byDomain
}

func listedRows(t *testing.T, registry *usecase.Registry) map[string]usecase.Output {
	t.Helper()
	listed, err := registry.Invoke(t.Context(), ListIdentityProvidersName, providerActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows := map[string]usecase.Output{}
	for _, row := range listed["data"].([]usecase.Output) {
		rows[row["id"].(string)] = row
	}
	return rows
}

func TestTheWorkspaceListShowsTheMandatedProviderOnAndSetByTheInstallation(t *testing.T) {
	_, registry, invited, byDomain := lockedOfferFixture(t)
	rows := listedRows(t, registry)

	if row := rows[invited.String()]; row["offered_here"] != true ||
		row["installation_lock"] != string(domain.InstallationLockTurnsOn) {
		t.Errorf("the invite-only row reads offered %v, lock %v", row["offered_here"], row["installation_lock"])
	}
	if row := rows[byDomain.String()]; row["offered_here"] != false ||
		row["installation_lock"] != string(domain.InstallationLockLeavesToWorkspace) {
		t.Errorf("the row admitted by domain reads offered %v, lock %v", row["offered_here"], row["installation_lock"])
	}
}

func TestTheSwitchRefusesToTurnOffAMandatedProvider(t *testing.T) {
	f, registry, invited, _ := lockedOfferFixture(t)

	_, err := registry.Invoke(t.Context(), OfferIdentityProviderName, providerActor(),
		usecase.Input{"id": invited.String(), "offered": false})
	if detailOf(err) != "identity_provider.mandated" || !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("switching off the mandated provider answered %v, want identity_provider.mandated", err)
	}
	if origin := shared.AsError(err).Params["origin"]; origin != string(domain.LockInstance) {
		t.Errorf("the refusal names origin %q, want INSTANCE", origin)
	}
	if len(f.workspaces.updates) != 0 {
		t.Error("the refused switch wrote the workspace")
	}
	if len(f.session.audit.entries) != 0 {
		t.Errorf("the refused switch left a trail entry: %+v", f.session.audit.entries)
	}

	// Switching it on is the workspace's own choice, kept for when the lock lifts.
	answered, err := registry.Invoke(t.Context(), OfferIdentityProviderName, providerActor(),
		usecase.Input{"id": invited.String(), "offered": true})
	if err != nil {
		t.Fatalf("switching on the mandated provider: %v", err)
	}
	if !f.workspaces.row.Settings.Offers(invited) || answered["installation_lock"] != string(domain.InstallationLockTurnsOn) {
		t.Errorf("switching on stored %v and answered %v", f.workspaces.row.Settings.Offers(invited), answered)
	}
}

// The mandated provider is a way in, so another one may go: the last-way guard counts it.
func TestAProviderBesideAMandatedOneMaySwitchOff(t *testing.T) {
	f, registry, _, byDomain := lockedOfferFixture(t)
	f.workspaces.row.Settings = f.workspaces.row.Settings.WithOffer(byDomain, true)

	answered, err := registry.Invoke(t.Context(), OfferIdentityProviderName, providerActor(),
		usecase.Input{"id": byDomain.String(), "offered": false})
	if err != nil {
		t.Fatalf("switching off the provider admitted by domain: %v", err)
	}
	if answered["offered_here"] != false || f.workspaces.row.Settings.Offers(byDomain) {
		t.Errorf("switching off answered %v and left the switch %v", answered["offered_here"],
			f.workspaces.row.Settings.Offers(byDomain))
	}
}
