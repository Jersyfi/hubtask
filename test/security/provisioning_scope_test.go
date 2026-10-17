// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/catalogue"
)

// The purchase platform's credential as a gate (T-26): the narrow scope reaches the verbs
// UC-INS-06 check 6 lists and no other.

const (
	provisioningScope = "admin:provisioning"
	operatorScope     = "admin:tenants"
)

// platformVerbs is UC-INS-06 check 6, as use cases. Written down here on purpose, against the
// descriptors: the scope is derived from them, so the gate is what notices an eleventh use case
// quietly opened to a platform's token.
var platformVerbs = []string{
	"ListTenants", "MoveTenantPlan", "ProvisionTenant", "QueryTenants", "ReadTenant",
	"RequestTenantDeletion", "ResumeTenant", "SuspendTenant", "UpdateTenant", "UpdateTenantQuotas",
}

func TestSGTheProvisioningScopeAdmitsExactlyThePlatformsVerbs(t *testing.T) {
	var admitted []string
	for _, descriptor := range catalogue.Descriptors() {
		if descriptor.TokenScope == provisioningScope {
			t.Errorf("%s demands the narrow scope alone; it is a subset of %s, never its own door",
				descriptor.Name, operatorScope)
		}
		if descriptor.AlsoScope == "" {
			continue
		}
		if descriptor.AlsoScope != provisioningScope || descriptor.TokenScope != operatorScope {
			t.Errorf("%s is admitted by %q beside %q; only the control plane has a narrower scope",
				descriptor.Name, descriptor.AlsoScope, descriptor.TokenScope)
		}
		admitted = append(admitted, descriptor.Name)
	}
	slices.Sort(admitted)
	if !slices.Equal(admitted, platformVerbs) {
		t.Errorf("the narrow scope admits %v, want exactly %v (UC-INS-06 check 6)", admitted, platformVerbs)
	}
	if !slices.Contains(catalogue.Scopes(), provisioningScope) {
		t.Error("the narrow scope is not in the catalogue, so no token can be minted with it")
	}
	if slices.Contains(catalogue.SessionScopes(), provisioningScope) {
		t.Error("a session carries the narrow scope; the control plane is entered by a minted credential")
	}
}
