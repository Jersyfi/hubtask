// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"
	"testing"

	identityrepository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// ADR-0073 §4 through the authoriser: a running emergency access opens a private hub to its
// opener, as their workspace role would open a shared one - for the person, present, while they
// are still an owner, and for nothing that outlives the hour.
func TestARunningEmergencyAccessOpensThePrivateHubToItsOpenerAlone(t *testing.T) {
	path := []identity.Scope{
		identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(collectionID),
	}
	present := func() appshared.ActorContext {
		actor := actorWithScopes()
		actor.Present = true
		return actor
	}
	owner := identity.Membership{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleOwner}
	admin := identity.Membership{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleAdmin}

	for _, tc := range []struct {
		name   string
		held   identity.Membership
		opened bool
		actor  func() appshared.ActorContext
		want   bool
	}{
		{name: "the owner, present, with a running access", held: owner, opened: true, actor: present, want: true},
		{name: "no access, or one that ended", held: owner, actor: present},
		{name: "an administrator holding an access (an owner demoted)", held: admin, opened: true, actor: present},
		{name: "the owner's copy - a device, a feed, a job", held: owner, opened: true, actor: func() appshared.ActorContext { return actorWithScopes() }},
		{name: "an AI agent with the owner's credential", held: owner, opened: true, actor: func() appshared.ActorContext {
			actor := present()
			actor.Kind = appshared.ActorAIAgent
			return actor
		}},
		{name: "a copy taken from a present actor", held: owner, opened: true, actor: func() appshared.ActorContext {
			return present().ForACopy()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			authorizer, store, _, _ := serviceWith([]identity.Membership{tc.held})
			store.hubs = map[shared.ID]identityrepository.Hub{
				hubID: {ID: hubID, Private: true}, collectionID: {ID: hubID, Private: true},
			}
			if tc.opened {
				store.opened = []shared.ID{hubID}
			}
			allowed, err := authorizer.Permits(context.Background(), tc.actor(),
				Request{Permission: service.PermissionRead, Path: path})
			if err != nil {
				t.Fatal(err)
			}
			if allowed != tc.want {
				t.Errorf("allowed %v, want %v", allowed, tc.want)
			}
			if role, found, _ := authorizer.RoleAlong(context.Background(), tc.actor(), path); found != tc.want ||
				(tc.want && role != identity.RoleOwner) {
				t.Errorf("role along: %q %v, want found %v", role, found, tc.want)
			}
		})
	}
}

// Whoever the question is about other than the present actor, and whatever is about to run for
// somebody (Hidden: a rule, a subscription, a notification), the access counts for nothing.
func TestAnEmergencyAccessCountsForNobodyElse(t *testing.T) {
	path := []identity.Scope{identity.TenantScope(), identity.HubScope(hubID)}
	authorizer, store, _, _ := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleOwner},
	})
	store.hubs = map[shared.ID]identityrepository.Hub{hubID: {ID: hubID, Private: true}}
	store.opened = []shared.ID{hubID}
	actor := actorWithScopes()
	actor.Present = true

	hidden, err := authorizer.Hidden(context.Background(), actor, path)
	if err != nil || !hidden {
		t.Errorf("Hidden answered %v, %v; the hub is hidden from what runs for the opener", hidden, err)
	}
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000f9")
	asking := actor
	asking.AccountID = other
	if visible, err := authorizer.CanSee(context.Background(), asking, accountID, path); err != nil || visible {
		t.Errorf("CanSee about the opener, asked by somebody else: %v, %v", visible, err)
	}
}
