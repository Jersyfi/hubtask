// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// Custom roles through the real decision point (D-05, UC-ID-26): a custom role reaches what it
// names and nothing else, keeps the shipped narrowings, and nobody hands out what they lack.

func customAt(scope identity.Scope, permissions ...service.Permission) []identity.Membership {
	names := make([]string, len(permissions))
	for i, p := range permissions {
		names[i] = string(p)
	}
	return []identity.Membership{{AccountID: actorID, Scope: scope, Custom: "custom", Permissions: names}}
}

func TestACustomRoleKeepsTheShippedNarrowings(t *testing.T) {
	hub := identity.HubScope(hubID)
	assertAnswer(t, "READ reads", customAt(hub, service.PermissionRead),
		request(service.ItemRead, otherItemID, strangerID), permitted)
	assertAnswer(t, "READ does not write", customAt(hub, service.PermissionRead),
		request(service.ItemChange, otherItemID, strangerID), forbidden)
	assertAnswer(t, "WRITE_ASSIGNED writes its own", customAt(hub, service.PermissionRead, service.PermissionWriteAssigned),
		request(service.ItemChange, ownItemID, actorID), permitted)
	assertAnswer(t, "WRITE_ASSIGNED does not write somebody else's", customAt(hub, service.PermissionRead, service.PermissionWriteAssigned),
		request(service.ItemChange, otherItemID, strangerID), forbidden)
	assertAnswer(t, "STRUCTURE alone reads nothing", customAt(hub, service.PermissionStructure),
		request(service.ItemRead, otherItemID, strangerID), forbidden)
	assertAnswer(t, "a custom role on a neighbouring hub reaches nothing here",
		customAt(identity.HubScope(strangerID), service.PermissionRead, service.PermissionWriteItems),
		request(service.ItemRead, otherItemID, strangerID), invisible)
}

func TestNobodyHandsOutAPermissionTheyLack(t *testing.T) {
	cases := []struct {
		what     string
		held     []identity.Membership
		handsOut []string
		want     bool
	}{
		{"an administrator hands out STRUCTURE", grantedAt(identity.TenantScope(), identity.RoleAdmin), []string{"STRUCTURE"}, true},
		{"an administrator does not hand out DELETE_CONTAINER", grantedAt(identity.TenantScope(), identity.RoleAdmin), []string{"DELETE_CONTAINER"}, false},
		{"an owner hands out DELETE_CONTAINER", grantedAt(identity.TenantScope(), identity.RoleOwner), []string{"DELETE_CONTAINER"}, true},
		{"a member does not hand out READ", grantedAt(identity.TenantScope(), identity.RoleMember), []string{"READ"}, false},
		{
			"a custom people manager hands out what it holds",
			customAt(identity.TenantScope(), service.PermissionManageMembers, service.PermissionRead),
			[]string{"READ"}, true,
		},
		{
			"a custom people manager does not hand out STRUCTURE",
			customAt(identity.TenantScope(), service.PermissionManageMembers, service.PermissionRead),
			[]string{"READ", "STRUCTURE"}, false,
		},
	}
	for _, c := range cases {
		authorizer, trail := serviceWith(c.held)
		err := authorizer.Authorize(context.Background(), actor("members:write"), access.Request{
			Permission: service.PermissionManageMembers,
			Path:       []identity.Scope{identity.TenantScope()},
			Action:     "role.created",
			TokenScope: "members:write",
			TargetType: "custom_role",
			HandsOut:   c.handsOut,
		})
		if (err == nil) != c.want {
			t.Errorf("%s: %v", c.what, err)
		}
		if err != nil {
			if !errors.Is(err, shared.ErrForbidden) || len(trail.entries) != 1 {
				t.Errorf("%s: refused as %v, recorded %d", c.what, err, len(trail.entries))
			}
		}
	}
}
