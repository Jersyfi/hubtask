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

// A grant on a single entry is decided along the entry's whole path - workspace, hub, collection,
// entry - as every other question about an entry is (domain-model.md §3.2, UC-WRK-21). The grant
// names only the entry; storage adds its collection and hub, so a role held there counts, and the
// private hub it may sit in discounts the workspace's role as it does for the hub itself.
func TestAGrantOnAnEntryCountsTheRolesAboveIt(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	authoriser := realAuthoriser(ctx, t)

	sharedItem := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().Insert(ctx, taskIn(tenantA, authorA, w.sharedColl, sharedItem, freshName(t), "a0"))
	}); err != nil {
		t.Fatalf("seeding the shared entry: %v", err)
	}
	hubAdmin, collectionAdmin := freshID(t), freshID(t)
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Hanna Hub')`,
			[]any{hubAdmin.String(), tenantA.String()}},
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Carl Collection')`,
			[]any{collectionAdmin.String(), tenantA.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		  VALUES ($1, $2, $3, 'HUB', $4, 'ADMIN')`,
			[]any{freshID(t).String(), tenantA.String(), hubAdmin.String(), w.sharedHub.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		  VALUES ($1, $2, $3, 'COLLECTION', $4, 'ADMIN')`,
			[]any{freshID(t).String(), tenantA.String(), collectionAdmin.String(), w.collection.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the people: %v", err)
		}
	}

	granter := func(account shared.ID) appshared.ActorContext {
		return appshared.ActorContext{
			Kind: appshared.ActorUser, TenantID: tenantA, AccountID: account, AccountName: "Someone",
			Scopes: []string{"members:write"},
		}
	}
	onEntry := func(item shared.ID) access.Request {
		return access.Request{
			Permission: service.PermissionManageMembers,
			Path:       []identity.Scope{identity.TenantScope(), identity.ItemScope(item)},
			Action:     "membership.granted", TokenScope: "members:write", TargetType: "membership",
			Grants: identity.RoleViewer,
		}
	}

	for name, tc := range map[string]struct {
		account shared.ID
		item    shared.ID
		allowed bool
	}{
		"the shared hub's admin, an entry in it":         {hubAdmin, sharedItem, true},
		"the shared hub's admin, an entry elsewhere":     {hubAdmin, w.item, false},
		"the private collection's admin, an entry in it": {collectionAdmin, w.item, true},
		"the private hub's owner, an entry in it":        {w.member, w.item, true},
		"the workspace's owner, the private entry":       {w.administrator, w.item, false},
		"the workspace's owner, the shared entry":        {w.administrator, sharedItem, true},
	} {
		err := authoriser.Authorize(ctx, granter(tc.account), onEntry(tc.item))
		if (err == nil) != tc.allowed {
			t.Errorf("%s: %v, want allowed %v", name, err, tc.allowed)
		}
	}
}
