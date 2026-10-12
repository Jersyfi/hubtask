// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Which installation locks put the invite-only offers on everywhere (identity.md §18): any lock on
// the ways in whose value holds a provider. The same stored shapes are asked of
// `count_provider_offers` in test/integration, with the same answers written down there.
func TestALockOnTheWaysInWithAProviderAmongThemMandatesTheOffers(t *testing.T) {
	cases := []struct {
		name    string
		methods *[]string
		locked  bool
		want    bool
	}{
		{"no lock and no value", nil, false, false},
		{"providers alone, locked", methodsOf(MethodOidc), true, true},
		{"password and providers, locked", methodsOf(MethodDirect, MethodOidc), true, true},
		{"the password alone, locked", methodsOf(MethodDirect), true, false},
		{"providers alone, a default only", methodsOf(MethodOidc), false, false},
		{"an empty list, locked: the product's default", methodsOf(), true, true},
		{"an unknown way in, locked: the product's default", methodsOf("SEMAPHORE"), true, true},
		{"a lock without a value pins the default", nil, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			instance := PolicyLayer{
				Patch: PolicyPatch{Methods: c.methods},
				Locks: map[PolicySwitch]bool{SwitchMethods: c.locked},
			}
			resolved := Effective(instance, PolicyLayer{}, PolicyLayer{})
			if got := resolved.ProvidersLockedIn(); got != c.want {
				t.Errorf("ProvidersLockedIn() = %v, want %v (methods %v)", got, c.want, resolved.Policy.Methods)
			}
		})
	}
}

// A workspace's own value cannot lift the mandate: it is ignored under the lock.
func TestAWorkspaceValueDoesNotLiftTheMandate(t *testing.T) {
	instance := PolicyLayer{
		Patch: PolicyPatch{Methods: methodsOf(MethodOidc)},
		Locks: map[PolicySwitch]bool{SwitchMethods: true},
	}
	workspace := PolicyLayer{Patch: PolicyPatch{Methods: methodsOf(MethodDirect)}}
	if !Effective(instance, PolicyLayer{}, workspace).ProvidersLockedIn() {
		t.Error("a workspace's own methods lifted the installation's lock")
	}
}

// Only the installation's invite-only rows are mandated, and only under the lock.
func TestOnlyAnInviteOnlyInstallationRowIsMandated(t *testing.T) {
	const workspace = shared.ID("01936f2a-7c1e-7000-8000-0000000000f1")
	cases := []struct {
		name     string
		tenant   shared.ID
		mode     Provisioning
		lockedIn bool
		want     bool
	}{
		{"installation, invited only, locked", "", ProvisionInvitedOnly, true, true},
		{"installation, invited only, no lock", "", ProvisionInvitedOnly, false, false},
		{"installation, by domain, locked", "", ProvisionDomains, true, false},
		{"installation, anyone, locked", "", ProvisionAny, true, false},
		{"the workspace's own, invited only, locked", workspace, ProvisionInvitedOnly, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := IdentityProvider{TenantID: c.tenant, Provisioning: c.mode}
			if got := row.MandatedHere(c.lockedIn); got != c.want {
				t.Errorf("MandatedHere(%v) = %v, want %v", c.lockedIn, got, c.want)
			}
		})
	}
}

// What a row says about the lock: nothing for a workspace's own, NONE without the lock or once the
// offer ended, and which of the two otherwise.
func TestEachRowSaysWhatTheInstallationsLockDoesToIt(t *testing.T) {
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	const workspace = shared.ID("01936f2a-7c1e-7000-8000-0000000000f1")
	cases := []struct {
		name     string
		row      IdentityProvider
		lockedIn bool
		want     InstallationLock
	}{
		{"a workspace's own row", IdentityProvider{TenantID: workspace, Enabled: true, Provisioning: ProvisionInvitedOnly}, true, ""},
		{"no lock", IdentityProvider{Enabled: true, Provisioning: ProvisionInvitedOnly}, false, InstallationLockNone},
		{"invited only, locked", IdentityProvider{Enabled: true, Provisioning: ProvisionInvitedOnly}, true, InstallationLockTurnsOn},
		{"by domain, locked", IdentityProvider{Enabled: true, Provisioning: ProvisionDomains}, true, InstallationLockLeavesToWorkspace},
		{"anyone, locked", IdentityProvider{Enabled: true, Provisioning: ProvisionAny}, true, InstallationLockLeavesToWorkspace},
		{"invited only, locked, withdrawn", IdentityProvider{Enabled: true, Provisioning: ProvisionInvitedOnly, WithdrawAt: at}, true, InstallationLockNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.row.InstallationLockAt(c.lockedIn, at); got != c.want {
				t.Errorf("InstallationLockAt = %q, want %q", got, c.want)
			}
		})
	}
}
