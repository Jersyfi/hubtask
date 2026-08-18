// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A grant names an account or a group, never both and never neither. Neither applies to nobody;
// both has two subjects and no answer to whose right it is when one of them is removed.
func TestAGrantHasExactlyOneSubject(t *testing.T) {
	account := shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")
	group := shared.ID("01936f2a-7c1e-7000-8000-0000000000g1")
	tenant := shared.ID("01936f2a-7c1e-7000-8000-00000000000a")
	id := shared.ID("01936f2a-7c1e-7000-8000-0000000000m1")

	cases := []struct {
		name    string
		account shared.ID
		group   shared.ID
		code    string
	}{
		{"an account", account, "", ""},
		{"a group", "", group, ""},
		{"neither", "", "", "memberships.subject_ambiguous"},
		{"both", account, group, "memberships.subject_ambiguous"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewGrant(id, tenant, c.account, c.group, TenantScope(), RoleMember)
			if c.code == "" {
				if err != nil {
					t.Fatalf("error %v, want the grant accepted", err)
				}
				return
			}
			if err == nil || shared.AsError(err).DetailCode != c.code {
				t.Fatalf("error %v, want %s", err, c.code)
			}
		})
	}
}

// A scope carries the identifier its level needs: the tenant scope none, every other level the
// container or item it applies to. A hub scope without a hub would grant a role over nothing.
func TestAScopeCarriesWhatItsLevelNeeds(t *testing.T) {
	hub := shared.ID("01936f2a-7c1e-7000-8000-0000000000h1")

	cases := []struct {
		name  string
		scope Scope
		want  bool
	}{
		{"the tenant, with no identifier", TenantScope(), true},
		{"the tenant, with one anyway", Scope{Type: ScopeTenant, ID: hub}, false},
		{"a hub", HubScope(hub), true},
		{"a hub without a hub", Scope{Type: ScopeHub}, false},
		{"a collection", CollectionScope(hub), true},
		{"an item", Scope{Type: ScopeItem, ID: hub}, true},
		{"a level nobody defined", Scope{Type: "WORKSPACE", ID: hub}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.scope.Valid(); got != c.want {
				t.Errorf("Valid() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestAGrantRefusesARoleNobodyDefined(t *testing.T) {
	_, err := NewGrant(
		shared.ID("01936f2a-7c1e-7000-8000-0000000000m1"),
		shared.ID("01936f2a-7c1e-7000-8000-00000000000a"),
		shared.ID("01936f2a-7c1e-7000-8000-0000000000a1"), "",
		TenantScope(), Role("SUPERUSER"))

	if err == nil || shared.AsError(err).DetailCode != "memberships.role_unknown" {
		t.Fatalf("error %v, want the role refused", err)
	}
}
