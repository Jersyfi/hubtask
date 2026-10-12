// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

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
