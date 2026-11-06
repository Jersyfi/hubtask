// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

func memberRequest() Request {
	return Request{AnyMember: true, Action: "group.read", TokenScope: "members:read", TargetType: "group"}
}

// A read of the workspace's own answers any member - a role on one hub is enough - and refuses a
// requester (REQUEST without READ, service.IsRequester) and an account holding nothing, as
// access.not_permitted for READ, recorded; Permits answers the same without recording.
func TestAnyMemberRefusesARequesterAndAnAccountHoldingNothing(t *testing.T) {
	cases := []struct {
		name         string
		held         []identity.Membership
		holdsNothing bool
		want         bool
	}{
		{name: "a viewer of one hub", held: []identity.Membership{
			{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleViewer},
		}, want: true},
		{name: "a requester", held: []identity.Membership{{
			AccountID: accountID, Scope: identity.CollectionScope(hubID), Custom: "requester",
			Permissions: []string{"REQUEST"},
		}}, holdsNothing: true, want: false},
		{name: "an account holding nothing", holdsNothing: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			authorize, store, trail, _ := serviceWith(tc.held)
			store.holdsNothing = tc.holdsNothing
			actor := actorWithScopes("members:read")

			permits, err := authorize.Permits(t.Context(), actor, memberRequest())
			if err != nil {
				t.Fatal(err)
			}
			if permits != tc.want {
				t.Errorf("Permits = %v, want %v", permits, tc.want)
			}
			if len(trail.entries) != 0 {
				t.Fatalf("Permits wrote %d audit entries, want none", len(trail.entries))
			}

			err = authorize.Authorize(t.Context(), actor, memberRequest())
			if tc.want {
				if err != nil {
					t.Fatalf("Authorize = %v, want nil", err)
				}
				return
			}
			refusal := shared.AsError(err)
			if !errors.Is(err, shared.ErrForbidden) ||
				refusal.DetailCode != "access.not_permitted" || refusal.Params["permission"] != "READ" {
				t.Fatalf("Authorize = %v, want access.not_permitted for READ", err)
			}
			if len(trail.entries) != 1 || trail.entries[0].Outcome != audit.OutcomeDenied ||
				trail.entries[0].Action != "group.read" {
				t.Fatalf("trail = %+v, want one denied group.read", trail.entries)
			}
		})
	}
}

// The token scope still bounds a member, and the person an agent acts for must be a member too
// (G2): an agent never reads for a requester what the requester may not read.
func TestAnyMemberAsksTheScopeAndTheDelegator(t *testing.T) {
	authorize, store, _, _ := serviceWith(nil)
	if err := authorize.Authorize(t.Context(), actorWithScopes("items:read"), memberRequest()); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("without members:read = %v, want forbidden", err)
	}

	person := shared.MustParseID("0192f000-0000-7000-8000-0000000000ee")
	store.holdsNothingFor = []shared.ID{person}
	agent := actorWithScopes("members:read")
	agent.Kind, agent.OnBehalfOf = appshared.ActorAIAgent, person
	if err := authorize.Authorize(t.Context(), agent, memberRequest()); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("for a requester = %v, want forbidden", err)
	}
	if ok, err := authorize.Permits(t.Context(), agent, memberRequest()); err != nil || ok {
		t.Fatalf("Permits for a requester = %v, %v, want false", ok, err)
	}
}
