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
	"github.com/Jersyfi/hubtask/core/port/audit"
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

// ADR-0073 §1 through the authoriser: the workspace's administrator holds nothing on a private hub,
// and what they ask about it is answered as for anything they hold nothing on.
func TestAWorkspaceRoleDoesNotReachAPrivateHub(t *testing.T) {
	authorizer, store, _, _ := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleOwner},
	})
	store.hubs = map[shared.ID]identityrepository.Hub{
		hubID: {ID: hubID, Private: true}, collectionID: {ID: hubID, Private: true},
	}
	path := []identity.Scope{
		identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(collectionID),
	}

	allowed, err := authorizer.Permits(context.Background(), actorWithScopes(),
		Request{Permission: service.PermissionRead, Path: path})
	if err != nil || allowed {
		t.Fatalf("allowed %v, error %v; the workspace's owner read a private hub", allowed, err)
	}

	entry := shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	err = authorizer.Authorize(context.Background(), actorWithScopes(), Request{
		Permission: service.PermissionRead, Path: path, Action: "item.read",
		On: ItemSubject{Does: service.ItemRead, ID: entry},
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an entry in a private hub answered %v, want not found (T-04)", err)
	}

	if _, found, err := authorizer.RoleAlong(context.Background(), actorWithScopes(), path); err != nil || found {
		t.Errorf("a role along a private hub: found %v, error %v", found, err)
	}
	if visible, err := authorizer.CanSee(context.Background(), actorWithScopes(), accountID, path); err != nil || visible {
		t.Errorf("CanSee answered %v, error %v; want false", visible, err)
	}
}

// Outside a private hub, any question about it reads as a hub that is not there (UC-ID-16 check
// 2, T-04); inside it, a refusal is the ordinary one.
func TestARefusalOnAPrivateHubIsNotFoundForOutsidersOnly(t *testing.T) {
	path := []identity.Scope{identity.TenantScope(), identity.HubScope(hubID)}
	hubs := map[shared.ID]identityrepository.Hub{hubID: {ID: hubID, Private: true}}
	ask := func(held []identity.Membership, does service.ItemAction) ([]audit.Entry, error) {
		authorizer, store, trail, _ := serviceWith(held)
		store.hubs = hubs
		request := Request{Permission: service.PermissionStructure, Path: path, Action: "container.updated"}
		if does != "" {
			request.On = ItemSubject{Does: does}
		}
		err := authorizer.Authorize(context.Background(), actorWithScopes(), request)
		return trail.entries, err
	}

	owner := []identity.Membership{{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleOwner}}
	for name, does := range map[string]service.ItemAction{"a container": "", "a creation": service.ItemCreate} {
		entries, err := ask(owner, does)
		var refusal *shared.Error
		if !errors.As(err, &refusal) || !errors.Is(err, shared.ErrNotFound) || refusal.DetailCode != "containers.not_found" {
			t.Errorf("%s: the workspace's owner was answered %v, want containers.not_found", name, err)
		}
		if len(entries) != 1 {
			t.Errorf("%s: the refusal was not recorded: %+v", name, entries)
		}
	}

	viewer := []identity.Membership{{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleViewer}}
	if _, err := ask(viewer, ""); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("the hub's viewer was answered %v, want forbidden", err)
	}
}

// A membership on the hub, or below it, still grants what it grants (ADR-0073 §1).
func TestAMembershipOnOrBelowAPrivateHubStillGrants(t *testing.T) {
	for name, scope := range map[string]identity.Scope{
		"the hub":        identity.HubScope(hubID),
		"its collection": identity.CollectionScope(collectionID),
	} {
		t.Run(name, func(t *testing.T) {
			authorizer, store, _, _ := serviceWith([]identity.Membership{
				{AccountID: accountID, Scope: identity.TenantScope(), Role: identity.RoleOwner},
				{AccountID: accountID, Scope: scope, Role: identity.RoleMember},
			})
			store.hubs = map[shared.ID]identityrepository.Hub{
				hubID: {ID: hubID, Private: true}, collectionID: {ID: hubID, Private: true},
			}
			path := []identity.Scope{
				identity.TenantScope(), identity.HubScope(hubID), identity.CollectionScope(collectionID),
			}

			role, found, err := authorizer.RoleAlong(context.Background(), actorWithScopes(), path)
			if err != nil || !found || role != identity.RoleMember {
				t.Fatalf("role %q, found %v, error %v; want MEMBER from %s", role, found, err, name)
			}
		})
	}
}

// Nobody with a role on the workspace is asked about privacy: only such a role is ever discounted,
// so the ordinary question costs no second read.
func TestPrivacyIsReadOnlyForAWorkspaceRole(t *testing.T) {
	authorizer, store, _, _ := serviceWith([]identity.Membership{
		{AccountID: accountID, Scope: identity.HubScope(hubID), Role: identity.RoleMember},
	})

	if _, err := authorizer.Permits(context.Background(), actorWithScopes(), Request{
		Permission: service.PermissionRead,
		Path:       []identity.Scope{identity.TenantScope(), identity.HubScope(hubID)},
	}); err != nil {
		t.Fatal(err)
	}
	if len(store.asked) != 0 {
		t.Errorf("the hubs were read for somebody holding nothing on the workspace: %v", store.asked)
	}
}

// RoleOf answers about another account by the same rule: a rule's run_as holding a workspace role
// reaches nothing in a private hub.
func TestRoleOfAnotherAccountKnowsAPrivateHub(t *testing.T) {
	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000e2")
	authorizer, store, _, _ := serviceWith([]identity.Membership{
		{AccountID: other, Scope: identity.TenantScope(), Role: identity.RoleAdmin},
	})
	store.hubs = map[shared.ID]identityrepository.Hub{hubID: {ID: hubID, Private: true}}

	_, found, err := authorizer.RoleOf(context.Background(), actorWithScopes(), other,
		[]identity.Scope{identity.TenantScope(), identity.HubScope(hubID)})
	if err != nil || found {
		t.Errorf("found %v, error %v; want no role in a private hub", found, err)
	}
}
