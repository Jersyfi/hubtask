// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INS-11 check 4, the installation's screen: each offered row says what the installation's lock on
// the ways in does to it - on everywhere, or each workspace's switch - read through the registry.
func TestTheOperatorsListSaysWhatTheLockDoesToEachRow(t *testing.T) {
	writer, store, _, _ := newInstanceProviderWriter(newRegister(operatorID))
	settings := writer.Instance.Settings.(*instanceStore)
	invited := shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")
	byDomain := shared.ID("01936f2a-7c1e-7000-8000-0000000000a2")
	store.rows = []domain.IdentityProvider{
		{ID: invited, Kind: domain.KindGeneric, Enabled: true, Provisioning: domain.ProvisionInvitedOnly},
		{ID: byDomain, Kind: domain.KindMicrosoft, Enabled: true, Provisioning: domain.ProvisionDomains},
	}
	registry, err := usecase.NewRegistry(nil, ListInstanceIdentityProviders{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	locks := func() map[string]any {
		t.Helper()
		listed, err := registry.Invoke(t.Context(), ListInstanceIdentityProvidersName, operator(), usecase.Input{})
		if err != nil {
			t.Fatalf("listing: %v", err)
		}
		rows, _ := listed["data"].([]usecase.Output)
		said := map[string]any{}
		for _, row := range rows {
			said[row["id"].(string)] = row["installation_lock"]
		}
		return said
	}

	unlocked := locks()
	for id, said := range unlocked {
		if said != string(domain.InstallationLockNone) {
			t.Errorf("without a lock row %s says %v", id, said)
		}
	}

	for _, methods := range [][]string{{domain.MethodOidc}, {domain.MethodDirect, domain.MethodOidc}} {
		settings.level.Policy.Patch.Methods = &methods
		settings.level.Policy.Locks[domain.SwitchMethods] = true
		said := locks()
		if said[invited.String()] != string(domain.InstallationLockTurnsOn) {
			t.Errorf("under a lock on %v the invite-only row says %v", methods, said[invited.String()])
		}
		if said[byDomain.String()] != string(domain.InstallationLockLeavesToWorkspace) {
			t.Errorf("under a lock on %v the row admitted by domain says %v", methods, said[byDomain.String()])
		}
	}

	settings.level.Policy.Patch.Methods = &[]string{domain.MethodDirect}
	if said := locks(); said[invited.String()] != string(domain.InstallationLockNone) {
		t.Errorf("under a lock on the password alone the invite-only row says %v", said[invited.String()])
	}
}
