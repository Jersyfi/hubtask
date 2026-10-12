// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// What changed between the configuration and the arrival is asked again: a group that is gone, or
// that holds OWNER by now, gives a newcomer nothing - and the person still gets in, and is told. A
// build wired without the stores refuses rather than creating an account without what was named.
// The modes and what they give are proven against PostgreSQL (test/integration).
func TestANewcomerGetsNothingFromAGroupThatChangedSinceItWasNamed(t *testing.T) {
	at := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	owners := shared.ID("01936f2a-7c1e-7000-8000-0000000002c1")
	gone := shared.ID("01936f2a-7c1e-7000-8000-0000000002c2")
	staff := shared.ID("01936f2a-7c1e-7000-8000-0000000002c3")

	cases := []struct {
		name   string
		group  shared.ID
		wired  bool
		joined bool
		failed bool
	}{
		{name: "a group that is still fine", group: staff, wired: true, joined: true},
		{name: "a group that holds OWNER by now", group: owners, wired: true},
		{name: "a group that is gone", group: gone, wired: true},
		{name: "a build wired without the stores", group: staff, failed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newOidcFixture(t, at)
			groups := newGroups(
				domain.Group{ID: owners, TenantID: tenant, Name: "Owners", Version: 1},
				domain.Group{ID: staff, TenantID: tenant, Name: "Staff", Version: 1},
			)
			grants := newGrants(domain.Grant{
				ID: "01936f2a-7c1e-7000-8000-0000000002c4", TenantID: tenant, GroupID: owners,
				Scope: domain.TenantScope(), Role: domain.RoleOwner,
			})
			if tc.wired {
				f.writer.Groups, f.writer.Grants = groups, grants
			}
			f.store.rows[0].Provisioning = domain.ProvisionAny
			f.store.rows[0].NewcomerGroupID = tc.group

			result, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(),
				CompleteOidcSignInCommand{Code: "the-code", State: start(t, f)})
			if tc.failed {
				if err == nil || shared.AsError(err).DetailCode != "identity_provider.newcomers_unwired" {
					t.Fatalf("answered %v, want the wiring refused", err)
				}
				return
			}
			if err != nil || result.Pair == nil {
				t.Fatalf("the arrival answered (%+v, %v), want a session", result, err)
			}
			joined := len(groups.members[tc.group]) == 1
			if joined != tc.joined {
				t.Errorf("joined the group: %v, want %v", joined, tc.joined)
			}
			if len(grants.granted) != 0 {
				t.Errorf("a group newcomer was granted %v directly", grants.granted)
			}
		})
	}
}
