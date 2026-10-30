// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

var personID = shared.MustParseID("0192f000-0000-7000-8000-0000000000ee")

// UC-AGT-02 check 2: an agent never does what the person it acts for may not, whatever its own
// account holds - and the trail says the delegator was the reason.
func TestAnAgentDoesNoMoreThanThePersonItActsFor(t *testing.T) {
	cases := []struct {
		name   string
		person identity.Role
		allow  bool
	}{
		{"the person may", identity.RoleAdmin, true},
		{"the person may not", identity.RoleMember, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			authorize, store, trail, _ := serviceWith(nil)
			store.byAccount = map[shared.ID][]identity.Membership{
				accountID: {{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleAdmin}},
				personID:  {{AccountID: personID, Scope: identity.TenantScope(), Role: c.person}},
			}
			agent := actorWithScopes("containers:write")
			agent.Kind = shared.ActorAIAgent
			agent.OnBehalfOf = personID

			err := authorize.Authorize(context.Background(), agent, request())
			if c.allow {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if !errors.Is(err, shared.ErrForbidden) {
				t.Fatalf("error %v, want forbidden", err)
			}
			if len(trail.entries) != 1 || trail.entries[0].Outcome != audit.OutcomeDenied ||
				trail.entries[0].ActorID != accountID || deniedBy(trail.entries[0]) != "delegator" {
				t.Fatalf("entries = %+v", trail.entries)
			}
			allowed, err := authorize.Permits(context.Background(), agent, request())
			if err != nil || allowed {
				t.Fatalf("a filter let the agent past its person: %v %v", allowed, err)
			}
		})
	}
}
