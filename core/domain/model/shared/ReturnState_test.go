// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import "testing"

func TestTheReturnStateNamesItsWorkspace(t *testing.T) {
	tenant := MustParseID("01936f2a-7c1e-7000-8000-000000000001")
	state := ReturnState(tenant, "aGFuZGxlLV8")
	if state != "01936f2a-7c1e-7000-8000-000000000001.aGFuZGxlLV8" {
		t.Fatalf("state %q", state)
	}
	for _, c := range []struct {
		name  string
		state string
		want  ID
		ok    bool
	}{
		{"minted", state, tenant, true},
		{"no handle", tenant.String() + ".", "", false},
		{"no workspace", "aGFuZGxlLV8", "", false},
		{"upper case", "01936F2A-7C1E-7000-8000-000000000001.x", "", false},
		{"not an id", "anna.x", "", false},
		{"empty", "", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, ok := TenantOfReturnState(c.state)
			if got != c.want || ok != c.ok {
				t.Errorf("(%q, %v), want (%q, %v)", got, ok, c.want, c.ok)
			}
		})
	}
}
