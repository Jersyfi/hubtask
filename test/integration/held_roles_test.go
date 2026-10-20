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

func rolesHeldAnywhere(ctx context.Context, t *testing.T, tenant, account shared.ID) []identity.Role {
	t.Helper()
	var roles []identity.Role
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		roles, err = postgres.NewMembershipRepository().HeldAnywhere(ctx, account)
		return err
	}); err != nil {
		t.Fatalf("reading the roles held: %v", err)
	}
	slices.Sort(roles)
	return roles
}

// seedLeah is an account of tenant A holding MEMBER on a hub in her own right and OWNER on the
// workspace through a group, removed with the group after the test.
func seedLeah(ctx context.Context, t *testing.T) (leah, group shared.ID) {
	t.Helper()
	seedContainerTenants(ctx, t)
	leah, group = freshID(t), freshID(t)
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Leah')`,
			[]any{leah.String(), tenantA.String()}},
		{`INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, $3)`,
			[]any{group.String(), tenantA.String(), "Owners " + group.String()}},
		{`INSERT INTO account_group_member (tenant_id, group_id, account_id) VALUES ($1, $2, $3)`,
			[]any{tenantA.String(), group.String(), leah.String()}},
		{`INSERT INTO membership (id, tenant_id, group_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
			[]any{freshID(t).String(), tenantA.String(), group.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		  VALUES ($1, $2, $3, 'HUB', $4, 'MEMBER')`,
			[]any{freshID(t).String(), tenantA.String(), leah.String(), membershipHub.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding Leah: %v", err)
		}
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.WithoutCancel(ctx), `DELETE FROM account_group WHERE id = $1`, group.String()); err != nil {
			t.Errorf("removing the group: %v", err)
		}
	})
	return leah, group
}

// Every role an account holds anywhere, in its own right and through a group, each once - and
// nothing through a group the account has left, nor for an account that holds nothing
// (identity.md §13).
func TestEveryRoleAnAccountHoldsIsRead(t *testing.T) {
	ctx := context.Background()
	leah, group := seedLeah(ctx, t)

	if got, want := rolesHeldAnywhere(ctx, t, tenantA, leah), []identity.Role{identity.RoleMember, identity.RoleOwner}; !slices.Equal(got, want) {
		t.Errorf("Leah in the group holds %v, want %v", got, want)
	}

	if _, err := adminPool(ctx, t).Exec(ctx, `DELETE FROM account_group_member WHERE group_id = $1 AND account_id = $2`,
		group.String(), leah.String()); err != nil {
		t.Fatalf("taking Leah out of the group: %v", err)
	}
	if got, want := rolesHeldAnywhere(ctx, t, tenantA, leah), []identity.Role{identity.RoleMember}; !slices.Equal(got, want) {
		t.Errorf("Leah after leaving the group holds %v, want %v", got, want)
	}

	if got := rolesHeldAnywhere(ctx, t, tenantA, freshID(t)); len(got) != 0 {
		t.Errorf("an account that holds nothing holds %v", got)
	}
}

// The cross-tenant negative test for HeldAnywhere (gate SG-3): tenant B asking about an account of
// tenant A reads none of its roles, though the identifier is known.
func TestRolesHeldInAnotherTenantAreInvisible(t *testing.T) {
	ctx := context.Background()
	leah, _ := seedLeah(ctx, t)

	if got := rolesHeldAnywhere(ctx, t, tenantB, leah); len(got) != 0 {
		t.Errorf("tenant B read tenant A's roles %v", got)
	}
}
