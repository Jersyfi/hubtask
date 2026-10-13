// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// UC-ID-14 check 1 against PostgreSQL (domain-model.md §3.2): Anna administers the workspace and
// owns one hub through a group. She does not hand out OWNER on the workspace, and she does on the
// hub - the role resolved from the stored memberships, the group's included, decides.
func TestNobodyGrantsAboveTheirOwnRoleAsTheDatabaseResolvesIt(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	authoriser := realAuthoriser(ctx, t)
	anna := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantA, AccountID: authorA, AccountName: "Anna",
		Scopes: []string{"members:write"},
	}
	workspace := []identity.Scope{identity.TenantScope()}
	hub := []identity.Scope{identity.TenantScope(), identity.HubScope(membershipHub)}
	grant := func(path []identity.Scope, role identity.Role) access.Request {
		return access.Request{
			Permission: service.PermissionManageMembers, Path: path, Action: "membership.granted",
			TokenScope: "members:write", TargetType: "membership", Grants: role,
		}
	}

	err := authoriser.Authorize(ctx, anna, grant(workspace, identity.RoleOwner))
	if shared.AsError(err).DetailCode != "access.role_above_own" {
		t.Errorf("OWNER on the workspace answered %v, want access.role_above_own", err)
	}
	if err := authoriser.Authorize(ctx, anna, grant(workspace, identity.RoleAdmin)); err != nil {
		t.Errorf("ADMIN on the workspace was refused: %v", err)
	}
	if err := authoriser.Authorize(ctx, anna, grant(hub, identity.RoleOwner)); err != nil {
		t.Errorf("OWNER on the hub she owns through a group was refused: %v", err)
	}

	atWorkspace, err := authoriser.GrantableRoles(ctx, anna, grant(workspace, ""))
	if err != nil || slices.Contains(atWorkspace, identity.RoleOwner) || !slices.Contains(atWorkspace, identity.RoleAdmin) {
		t.Errorf("grantable on the workspace %v (%v), want every role but OWNER", atWorkspace, err)
	}
	atHub, err := authoriser.GrantableRoles(ctx, anna, grant(hub, ""))
	if err != nil || !slices.Equal(atHub, identity.Roles()) {
		t.Errorf("grantable on the hub %v (%v), want every role", atHub, err)
	}
}
