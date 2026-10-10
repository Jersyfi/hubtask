// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"
	"errors"
	"testing"

	identityrepository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// A rule scoped to a collection names the collection and not its hub (automation.Scope.Path); the
// hub's administrator must still be judged on the role they hold on the hub.
func TestAHubsAdministratorIsJudgedOnACollectionPathWithoutTheHub(t *testing.T) {
	authorizer, store, _, _ := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleAdmin},
	})
	store.hubs = map[shared.ID]identityrepository.Hub{collectionID: {ID: hubID}}

	err := authorizer.Authorize(context.Background(), actorWithScopes("automation:write"), Request{
		Permission: service.PermissionAutomation,
		Path:       []identity.Scope{identity.TenantScope(), identity.CollectionScope(collectionID)},
		Action:     "automation.rule_created",
		TokenScope: "automation:write",
	})
	if err != nil {
		t.Fatalf("the hub's administrator was refused on its collection: %v", err)
	}
	want := []identity.Scope{
		identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(collectionID),
	}
	if len(store.path) != len(want) {
		t.Fatalf("the memberships were read along %v, want %v", store.path, want)
	}
	for i := range want {
		if store.path[i] != want[i] {
			t.Errorf("step %d is %v, want %v", i, store.path[i], want[i])
		}
	}
}

// The ordinary path names its hub, and asks storage nothing more than the memberships.
func TestAPathWithItsHubAsksForNoHub(t *testing.T) {
	authorizer, store, _, _ := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleAdmin},
	})

	allowed, err := authorizer.Permits(context.Background(), actorWithScopes(), Request{
		Permission: service.PermissionRead,
		Path: []identity.Scope{
			identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(collectionID),
		},
	})
	if err != nil || !allowed {
		t.Fatalf("allowed %v, error %v; want allowed", allowed, err)
	}
	if len(store.asked) != 0 {
		t.Errorf("a path naming its hub asked for hubs: %v", store.asked)
	}
}

// A collection storage does not know - gone, or another workspace's - adds nothing, and the path
// is judged as it was given.
func TestAnUnknownCollectionIsJudgedAsGiven(t *testing.T) {
	authorizer, _, _, _ := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleAdmin},
	})

	allowed, err := authorizer.Permits(context.Background(), actorWithScopes(), Request{
		Permission: service.PermissionRead,
		Path:       []identity.Scope{identity.TenantScope(), identity.CollectionScope(collectionID)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Error("a role on a hub reached a collection storage does not place under it")
	}
}

// Many paths at once complete each path on its own.
func TestPermittedCompletesEachPath(t *testing.T) {
	otherCollection := shared.MustParseID("0192f000-0000-7000-8000-0000000000cc")
	authorizer, store, _, _ := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleViewer},
	})
	store.hubs = map[shared.ID]identityrepository.Hub{collectionID: {ID: hubID}}

	allowed, err := authorizer.Permitted(context.Background(), actorWithScopes(),
		Request{Permission: service.PermissionRead}, [][]identity.Scope{
			{identity.TenantScope(), identity.CollectionScope(collectionID)},
			{identity.TenantScope(), identity.CollectionScope(otherCollection)},
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(allowed) != 2 || !allowed[0] || allowed[1] {
		t.Errorf("answers %v, want [true false]", allowed)
	}
	if len(store.asked) != 1 || len(store.asked[0]) != 2 {
		t.Errorf("the hubs were asked %v, want one question about both collections", store.asked)
	}
}

// A failed read of the hubs is not a refusal: the question could not be answered.
func TestAFailedHubReadIsNotARefusal(t *testing.T) {
	authorizer, store, trail, _ := serviceWith(nil)
	store.err = shared.ErrUnavailable

	err := authorizer.Authorize(context.Background(), actorWithScopes(), Request{
		Permission: service.PermissionRead,
		Path:       []identity.Scope{identity.TenantScope(), identity.CollectionScope(collectionID)},
	})
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("error %v, want unavailable", err)
	}
	if len(trail.entries) != 0 {
		t.Errorf("a failed read was recorded as a refusal: %+v", trail.entries)
	}
}
