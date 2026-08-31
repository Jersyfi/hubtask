// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

var (
	account       = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	hub           = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	otherHub      = shared.MustParseID("0192f000-0000-7000-8000-00000000000e")
	collectionID  = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	hubPath       = []identity.Scope{identity.TenantScope(), identity.HubScope(hub)}
	collectionPth = []identity.Scope{identity.TenantScope(), identity.HubScope(hub), identity.CollectionScope(collectionID)}
)

func grant(scope identity.Scope, role identity.Role) identity.Membership {
	return identity.Membership{AccountID: account, Scope: scope, Role: role}
}

// The matrix of domain-model.md §3.2, read as a table. The row that matters most is the
// administrator's: it ranks above a member and still may not delete a container.
func TestTheRoleMatrix(t *testing.T) {
	cases := []struct {
		role       identity.Role
		permission service.Permission
		want       bool
	}{
		{identity.RoleOwner, service.PermissionDeleteContainer, true},
		{identity.RoleAdmin, service.PermissionDeleteContainer, false},
		{identity.RoleAdmin, service.PermissionStructure, true},
		{identity.RoleAdmin, service.PermissionManageMembers, true},
		{identity.RoleMember, service.PermissionStructure, false},
		{identity.RoleMember, service.PermissionWriteItems, true},
		{identity.RoleMember, service.PermissionAutomation, true},
		{identity.RoleContributor, service.PermissionAutomation, false},
		{identity.RoleContributor, service.PermissionWriteItems, true},
		{identity.RoleViewer, service.PermissionWriteItems, false},
		{identity.RoleViewer, service.PermissionRead, true},
		{identity.RoleGuest, service.PermissionWriteItems, false},
		{identity.RoleGuest, service.PermissionRead, true},
		// The auditor's row is the one that is not a rung on the ladder: the whole trail, how the
		// workspace is configured, and nothing that anybody wrote (audit.md §5, §9).
		{identity.RoleAuditor, service.PermissionAuditRead, true},
		{identity.RoleAuditor, service.PermissionReadConfiguration, true},
		{identity.RoleAuditor, service.PermissionRead, false},
		{identity.RoleAuditor, service.PermissionWriteItems, false},
		{identity.RoleAuditor, service.PermissionStructure, false},
		{identity.RoleAuditor, service.PermissionAutomation, false},
		{identity.RoleAuditor, service.PermissionManageMembers, false},
		{identity.RoleAuditor, service.PermissionDeleteContainer, false},
		{identity.RoleOwner, service.PermissionAuditRead, true},
		{identity.RoleAdmin, service.PermissionAuditRead, true},
		// A member reads their own events instead, which is the absence of this permission
		// rather than a weaker version of it.
		{identity.RoleMember, service.PermissionAuditRead, false},
		{identity.RoleViewer, service.PermissionAuditRead, false},
	}

	for _, c := range cases {
		if got := service.RoleAllows(c.role, c.permission); got != c.want {
			t.Errorf("%s may %s: %v, want %v", c.role, c.permission, got, c.want)
		}
	}

	// Every role carries at least the right to see something, and an invented one carries
	// nothing. The auditor is the exception and the reason the loop names one: it sees the trail
	// and none of the work, which is what the role is for.
	for _, role := range identity.Roles() {
		if role == identity.RoleAuditor {
			continue
		}
		if !service.RoleAllows(role, service.PermissionRead) {
			t.Errorf("%s cannot read anything", role)
		}
	}
	if len(service.PermissionsOf(identity.RoleAuditor)) != 2 {
		t.Errorf("the auditor carries %v, want the trail and the configuration",
			service.PermissionsOf(identity.RoleAuditor))
	}
	if service.RoleAllows("SUPERUSER", service.PermissionRead) {
		t.Error("a role that does not exist was granted a permission")
	}
}

func TestEffectiveRoleTakesTheHighestAlongThePath(t *testing.T) {
	cases := []struct {
		name        string
		memberships []identity.Membership
		path        []identity.Scope
		want        identity.Role
		wantFound   bool
	}{
		{
			name:      "nothing at all",
			path:      hubPath,
			wantFound: false,
		},
		{
			name:        "a role at the tenant applies downwards",
			memberships: []identity.Membership{grant(identity.TenantScope(), identity.RoleAdmin)},
			path:        collectionPth,
			want:        identity.RoleAdmin, wantFound: true,
		},
		{
			name: "the higher of two wins, whichever level it sits at",
			memberships: []identity.Membership{
				grant(identity.TenantScope(), identity.RoleViewer),
				grant(identity.HubScope(hub), identity.RoleOwner),
			},
			path: hubPath,
			want: identity.RoleOwner, wantFound: true,
		},
		{
			name: "a lower role at a deeper scope does not take anything away",
			memberships: []identity.Membership{
				grant(identity.TenantScope(), identity.RoleAdmin),
				grant(identity.CollectionScope(collectionID), identity.RoleViewer),
			},
			path: collectionPth,
			want: identity.RoleAdmin, wantFound: true,
		},
		{
			name:        "a role on a neighbour says nothing about this one",
			memberships: []identity.Membership{grant(identity.HubScope(otherHub), identity.RoleOwner)},
			path:        hubPath,
			wantFound:   false,
		},
		{
			name:        "a role at a deeper scope does not reach upwards",
			memberships: []identity.Membership{grant(identity.CollectionScope(collectionID), identity.RoleOwner)},
			path:        hubPath,
			wantFound:   false,
		},
		{
			name:        "an invalid role is not a role",
			memberships: []identity.Membership{grant(identity.TenantScope(), "SUPERUSER")},
			path:        hubPath,
			wantFound:   false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			role, found := service.EffectiveRole(c.memberships, c.path)
			if found != c.wantFound {
				t.Fatalf("found = %v, want %v (role %q)", found, c.wantFound, role)
			}
			if found && role != c.want {
				t.Errorf("role %s, want %s", role, c.want)
			}
		})
	}
}

// The whole decision in one call, which is what a use case asks.
func TestAllows(t *testing.T) {
	admin := []identity.Membership{grant(identity.TenantScope(), identity.RoleAdmin)}
	member := []identity.Membership{grant(identity.HubScope(hub), identity.RoleMember)}

	if !service.Allows(admin, hubPath, service.PermissionStructure) {
		t.Error("an administrator may not create a container")
	}
	if service.Allows(member, hubPath, service.PermissionStructure) {
		t.Error("a member may create a container")
	}
	if service.Allows(nil, hubPath, service.PermissionRead) {
		t.Error("an account with no membership at all may read")
	}
}

// Ranking is what "the highest role" means, so it is checked directly rather than only through
// the resolution.
func TestRolesRankFromOwnerDownwards(t *testing.T) {
	ordered := identity.Roles()

	for i, role := range ordered {
		if !role.AtLeast(role) {
			t.Errorf("%s does not rank as high as itself", role)
		}
		for _, weaker := range ordered[i+1:] {
			if !role.AtLeast(weaker) || weaker.AtLeast(role) {
				t.Errorf("%s and %s are ranked the wrong way round", role, weaker)
			}
		}
	}
	if identity.Role("SUPERUSER").AtLeast(identity.RoleGuest) {
		t.Error("a role that does not exist outranks one that does")
	}
}

// A-4's other half, and the half that could go wrong silently: splitting a read-only configuration
// permission out of STRUCTURE must leave every pre-existing role with exactly the rights it had.
//
// Written as the matrix before the split, so that the test fails whichever way somebody moves a
// cell - a role that gained one and a role that lost one are the same defect here, and neither is
// visible in a diff that only adds a column.
func TestTheSplitWidenedAndNarrowedNoExistingRole(t *testing.T) {
	before := map[identity.Role][]service.Permission{
		identity.RoleOwner: {
			service.PermissionRead, service.PermissionWriteItems, service.PermissionStructure,
			service.PermissionManageMembers, service.PermissionAutomation,
			service.PermissionDeleteContainer, service.PermissionAuditRead,
		},
		identity.RoleAdmin: {
			service.PermissionRead, service.PermissionWriteItems, service.PermissionStructure,
			service.PermissionManageMembers, service.PermissionAutomation,
			service.PermissionAuditRead,
		},
		identity.RoleMember: {
			service.PermissionRead, service.PermissionWriteItems, service.PermissionAutomation,
		},
		identity.RoleContributor: {service.PermissionRead, service.PermissionWriteItems},
		identity.RoleViewer:      {service.PermissionRead},
		identity.RoleGuest:       {service.PermissionRead},
		identity.RoleAuditor:     {service.PermissionAuditRead},
	}

	for role, had := range before {
		for _, permission := range had {
			if !service.RoleAllows(role, permission) {
				t.Errorf("%s lost %s", role, permission)
			}
		}
		now := service.PermissionsOf(role)
		for _, permission := range now {
			// The new one is the only permission a role may have gained, and only where the role
			// already held the writing permission it was split out of - or where it is the auditor,
			// which is what A-4 is for.
			if permission == service.PermissionReadConfiguration {
				if role != identity.RoleAuditor && !service.RoleAllows(role, service.PermissionStructure) {
					t.Errorf("%s gained %s without holding what it was split out of", role, permission)
				}
				continue
			}
			if !contains(had, permission) {
				t.Errorf("%s gained %s", role, permission)
			}
		}
	}
}

func contains(permissions []service.Permission, wanted service.Permission) bool {
	for _, permission := range permissions {
		if permission == wanted {
			return true
		}
	}
	return false
}
