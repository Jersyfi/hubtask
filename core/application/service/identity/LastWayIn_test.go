// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-ID-12 check 6 and UC-ID-11 check 8 (SC-06): the last remaining way in cannot be switched off -
// at every door that can switch one off. The provider switch guarded it since SI-10; the password's
// switch (`methods`), the provider's own form and its removal did not, so a workspace could still be
// left with no way in by any of those three.

func refusedAsLastWayIn(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s left the workspace with no way in", what)
	}
	if detail := shared.AsError(err).DetailCode; detail != "identity_provider.last_way_in" {
		t.Errorf("%s was refused with %q, want identity_provider.last_way_in", what, detail)
	}
}

func TestThePasswordCannotBeSwitchedOffWithNoProviderOn(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	providers := newProviderStore(tenant)
	fixture.writer.Providers = providers
	onlyProviders := []string{domain.MethodOidc}

	_, err := UpdateWorkspace{Writer: fixture.writer}.Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{
			SignIn:      WorkspacePolicyChange{Policy: domain.PolicyPatch{Methods: &onlyProviders}},
			StepUpToken: "hbt_stp_x",
		})
	refusedAsLastWayIn(t, err, "switching the password off with no provider")
	var refusal *shared.Error
	if e := shared.AsError(err); e != nil {
		refusal = e
	}
	if refusal == nil || len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/sign_in_policy/methods" {
		t.Errorf("the refusal does not attach to the rule: %v", err)
	}
	if len(fixture.store.updates) != 0 {
		t.Error("the workspace was written despite the refusal")
	}

	// With a provider on here, the password may go.
	providers.rows = append(providers.rows, domain.IdentityProvider{
		ID: shared.MustParseID("11111111-1111-4111-8111-111111111111"), TenantID: tenant,
		Issuer: "https://login.example.org", ClientID: "hubtask", DisplayName: "Example",
		Kind: domain.KindGeneric, Provisioning: domain.ProvisionInvitedOnly, Enabled: true, Version: 1,
	})
	if _, err := (UpdateWorkspace{Writer: fixture.writer}).Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{
			SignIn:      WorkspacePolicyChange{Policy: domain.PolicyPatch{Methods: &onlyProviders}},
			StepUpToken: "hbt_stp_x",
		}); err != nil {
		t.Errorf("switching the password off beside a provider that is on was refused: %v", err)
	}
}

func TestTheOnlyProviderCannotBeSwitchedOffOnItsFormOrRemoved(t *testing.T) {
	f := newOfferFixture(time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC))
	id := f.own(t, true)
	onlyProvider := []string{domain.MethodOidc}
	f.workspaces.row.Settings.SignIn.Methods = &onlyProvider
	f.writer.Workspaces = f.workspaces

	// Its own form, saved with the provider off. Since SC-21 the form switches nothing at all
	// (ADR-0076 §5), so the refusal is that one rather than the last way in - and stronger for it.
	command := configureCommand()
	command.ID = id
	command.Enabled = boolOf(false)
	_, err := ConfigureIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor(), command)
	refusedAsSwitchInList(t, err, "switching the only provider off on its form")

	// Removing it.
	err = RemoveIdentityProvider{Writer: f.writer}.Execute(t.Context(), providerActor(), id, "")
	refusedAsLastWayIn(t, err, "removing the only provider")
	if len(f.store.rows) != 1 || !f.store.rows[0].Enabled {
		t.Error("the only way in was written despite the refusals")
	}

	// The form saved with the provider still on is an ordinary save.
	command.Enabled = boolOf(true)
	if _, err := (ConfigureIdentityProvider{Writer: f.writer}).Execute(t.Context(), providerActor(), command); err != nil {
		t.Errorf("saving the only provider's form, still on, was refused: %v", err)
	}
}
