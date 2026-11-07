// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// seedRoleGrants gives Leah (in her group) a custom role in her own right and through the group,
// and somebody else the same role; the role goes with the test.
func seedRoleGrants(ctx context.Context, t *testing.T) (leah, group, own, throughGroup, others shared.ID) {
	t.Helper()
	leah, group = seedLeah(ctx, t)
	role, someone := freshID(t), freshID(t)
	own, throughGroup, others = freshID(t), freshID(t), freshID(t)
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Someone')`,
			[]any{someone.String(), tenantA.String()}},
		{`INSERT INTO custom_role (id, tenant_id, key, name, permissions, created_at, updated_at)
		  VALUES ($1, $2, $3, 'Auditor', ARRAY['READ'], now(), now())`,
			[]any{role.String(), tenantA.String(), "auditor_" + role.String()[30:]}},
		{`INSERT INTO custom_role_grant (id, tenant_id, account_id, scope_type, role_id, created_at)
		  VALUES ($1, $2, $3, 'TENANT', $4, now())`,
			[]any{own.String(), tenantA.String(), leah.String(), role.String()}},
		{`INSERT INTO custom_role_grant (id, tenant_id, group_id, scope_type, role_id, created_at)
		  VALUES ($1, $2, $3, 'TENANT', $4, now())`,
			[]any{throughGroup.String(), tenantA.String(), group.String(), role.String()}},
		{`INSERT INTO custom_role_grant (id, tenant_id, account_id, scope_type, role_id, created_at)
		  VALUES ($1, $2, $3, 'TENANT', $4, now())`,
			[]any{others.String(), tenantA.String(), someone.String(), role.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the grants: %v", err)
		}
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.WithoutCancel(ctx), `DELETE FROM custom_role WHERE id = $1`, role.String()); err != nil {
			t.Errorf("removing the role: %v", err)
		}
	})
	return leah, group, own, throughGroup, others
}

func ownGrantIDs(ctx context.Context, t *testing.T, tenant, account shared.ID) []shared.ID {
	t.Helper()
	var ids []shared.ID
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		rows, err := snapshotRepo().OwnRoleGrants(ctx, account, "", 1000)
		for _, row := range rows {
			ids = append(ids, row.Grant.ID)
		}
		return err
	}); err != nil {
		t.Fatalf("paging the grants: %v", err)
	}
	return ids
}

// offline-sync.md §4.3: a reader is walked the grants named to them and to their group, never
// another person's - and from another tenant, none.
func TestTheSnapshotPagesOnlyTheReadersOwnRoleGrants(t *testing.T) {
	ctx := context.Background()
	leah, group, own, throughGroup, others := seedRoleGrants(ctx, t)

	got := ownGrantIDs(ctx, t, tenantA, leah)
	if !slices.Contains(got, own) || !slices.Contains(got, throughGroup) || slices.Contains(got, others) {
		t.Errorf("Leah was walked %v, want %s and %s, never %s", got, own, throughGroup, others)
	}
	var member bool
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		member, err = snapshotRepo().InGroup(ctx, leah, group)
		return err
	}); err != nil || !member {
		t.Errorf("Leah in her group: %v, %v", member, err)
	}
}

func TestTheReadersRoleGrantsAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	leah, group, _, _, _ := seedRoleGrants(ctx, t)

	if got := ownGrantIDs(ctx, t, tenantB, leah); len(got) != 0 {
		t.Errorf("tenant B was walked tenant A's grants %v", got)
	}
	var member bool
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		member, err = snapshotRepo().InGroup(ctx, leah, group)
		return err
	}); err != nil || member {
		t.Errorf("tenant B saw tenant A's group membership: %v, %v", member, err)
	}
}
