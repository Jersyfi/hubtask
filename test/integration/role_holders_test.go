// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func roleHoldersAlong(
	ctx context.Context, t *testing.T, tenant shared.ID, path []identity.Scope, role string,
) []shared.ID {
	t.Helper()
	var holders []shared.ID
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		holders, err = postgres.NewMembershipRepository().RoleHolders(ctx, path, role)
		return err
	}); err != nil {
		t.Fatalf("reading the role's holders: %v", err)
	}
	return holders
}

// NOTIFY_ROLE's candidates against PostgreSQL (UC-AUT-09): a built-in role directly or through a
// group, a custom role by its key through a group, each holder once.
func TestTheHoldersOfARoleAlongAPathAreFound(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	path := []identity.Scope{identity.TenantScope(), identity.HubScope(membershipHub)}
	if holders := roleHoldersAlong(ctx, t, tenantA, path, "OWNER"); !slices.Contains(holders, authorA) {
		t.Errorf("the hub's OWNER through a group is missing: %v", holders)
	}
	if holders := roleHoldersAlong(ctx, t, tenantA, path, "ADMIN"); len(holders) != 1 || holders[0] != authorA {
		t.Errorf("the workspace's ADMIN: %v, want Anna once", holders)
	}
	if holders := roleHoldersAlong(ctx, t, tenantA, path, "VIEWER"); slices.Contains(holders, authorA) {
		t.Errorf("Anna holds no VIEWER: %v", holders)
	}

	role, grant := freshID(t), freshID(t)
	key := "dispatch_" + role.String()[30:]
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO custom_role (id, tenant_id, key, name, permissions, created_at, updated_at)
		  VALUES ($1, $2, $3, 'Dispatch', ARRAY['READ'], now(), now())`,
			[]any{role.String(), tenantA.String(), key}},
		{`INSERT INTO custom_role_grant (id, tenant_id, group_id, scope_type, scope_id, role_id, created_at)
		  VALUES ($1, $2, $3, 'HUB', $4, $5, now())`,
			[]any{grant.String(), tenantA.String(), membershipGroup.String(), membershipHub.String(), role.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the custom role: %v", err)
		}
	}
	if holders := roleHoldersAlong(ctx, t, tenantA, path, key); len(holders) != 1 || holders[0] != authorA {
		t.Errorf("the custom role through a group: %v, want Anna once", holders)
	}
	if holders := roleHoldersAlong(ctx, t, tenantA, []identity.Scope{identity.TenantScope()}, key); len(holders) != 0 {
		t.Errorf("a hub's role was found off its path: %v", holders)
	}
}

// The cross-tenant negative test for RoleHolders (gate SG-3): another tenant's holders are not
// found, even on a path that names that tenant's hub.
func TestTheHoldersOfARoleInAnotherTenantAreInvisible(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	path := []identity.Scope{identity.TenantScope(), identity.HubScope(membershipHub)}
	if holders := roleHoldersAlong(ctx, t, tenantB, path, "ADMIN"); slices.Contains(holders, authorA) {
		t.Errorf("tenant B read tenant A's ADMIN: %v", holders)
	}
	if holders := roleHoldersAlong(ctx, t, tenantB, path, "OWNER"); len(holders) != 1 || holders[0] != authorB {
		t.Errorf("tenant B's owners are %v, want its own", holders)
	}
}
