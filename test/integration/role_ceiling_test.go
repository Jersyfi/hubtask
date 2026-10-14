// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
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

var (
	ownersGroup      = shared.MustParseID("01936f2a-7c1e-7000-8000-00000c371001")
	ownersGroupGrant = shared.MustParseID("01936f2a-7c1e-7000-8000-00000c371002")
)

// seedOwnersGroup is a group of nobody that holds OWNER on Anna's workspace, removed after the test.
func seedOwnersGroup(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, 'Owners')
		ON CONFLICT (id) DO NOTHING`, ownersGroup.String(), tenantA.String()); err != nil {
		t.Fatalf("seeding the group: %v", err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO membership (id, tenant_id, group_id, scope_type, role)
		VALUES ($1, $2, $3, 'TENANT', 'OWNER') ON CONFLICT (id) DO NOTHING`,
		ownersGroupGrant.String(), tenantA.String(), ownersGroup.String()); err != nil {
		t.Fatalf("seeding the group's role: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.WithoutCancel(ctx), `DELETE FROM account_group WHERE id = $1`, ownersGroup.String()); err != nil {
			t.Errorf("removing the group: %v", err)
		}
	})
}

// #1371 and #1372 against PostgreSQL (domain-model.md §3.2): Anna administers the workspace. She
// does not join a group that holds OWNER there, does not delete it, and does not revoke its OWNER
// membership - each refused by the ceiling the stored memberships resolve, and nothing changes.
func TestNobodyTakesOrJoinsARoleAboveTheirOwnAsTheDatabaseResolvesIt(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	seedOwnersGroup(ctx, t)
	authoriser := realAuthoriser(ctx, t)
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	groups := postgres.NewGroupRepository(pageCursors())
	grants := postgres.NewMembershipGrantRepository(pageCursors())
	anna := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenantA, AccountID: authorA, AccountName: "Anna",
		Scopes: []string{"members:write"},
	}

	_, err := identityservice.UpdateGroup{
		Groups: groups, Accounts: postgres.NewAccountRepository(), Grants: grants, Authorizer: authoriser,
		UnitOfWork: unitOfWork, Clock: portclock.Fixed(created),
	}.Execute(ctx, anna, identityservice.UpdateGroupCommand{
		GroupID: ownersGroup, Members: []shared.ID{authorA}, ReplaceMembers: true,
	})
	if shared.AsError(err).DetailCode != "access.role_above_own" {
		t.Errorf("joining the owners' group answered %v, want access.role_above_own", err)
	}

	err = identityservice.DeleteGroup{
		Groups: groups, Grants: grants, Authorizer: authoriser,
		UnitOfWork: unitOfWork, Clock: portclock.Fixed(created),
	}.Execute(ctx, anna, identityservice.DeleteGroupCommand{GroupID: ownersGroup})
	if shared.AsError(err).DetailCode != "access.role_above_own" {
		t.Errorf("deleting the owners' group answered %v, want access.role_above_own", err)
	}

	err = identityservice.RevokeMembership{
		Grants: grants, Authorizer: authoriser, UnitOfWork: unitOfWork, Clock: portclock.Fixed(created),
	}.Execute(ctx, anna, identityservice.RevokeMembershipCommand{MembershipID: ownersGroupGrant})
	if shared.AsError(err).DetailCode != "access.role_above_own" {
		t.Errorf("revoking the owners' membership answered %v, want access.role_above_own", err)
	}

	var members []shared.ID
	var held int
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if members, err = groups.Members(ctx, ownersGroup); err != nil {
			return err
		}
		of, err := grants.OfGroup(ctx, ownersGroup)
		held = len(of)
		return err
	}); err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if len(members) != 0 || held != 1 {
		t.Errorf("after the refusals the group has members %v and %d roles, want none and one", members, held)
	}
}
