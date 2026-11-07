// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// seedGroupWith writes a group holding the accounts, directly: the groups' own use cases are
// proven elsewhere, and this file is about what the query reads.
func seedGroupWith(ctx context.Context, t *testing.T, tenant shared.ID, members ...shared.ID) shared.ID {
	t.Helper()

	id := freshID(t)
	pool := adminPool(ctx, t)
	if _, err := pool.Exec(ctx, "INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, $3)",
		id.String(), tenant.String(), freshName(t)); err != nil {
		t.Fatalf("seeding the group: %v", err)
	}
	for _, member := range members {
		if _, err := pool.Exec(ctx,
			"INSERT INTO account_group_member (tenant_id, group_id, account_id) VALUES ($1, $2, $3)",
			tenant.String(), id.String(), member.String()); err != nil {
			t.Fatalf("seeding the membership: %v", err)
		}
	}
	return id
}

// seedPolicy gives a collection an auto-assign policy over the candidates, written as the
// repository writes the column.
func seedPolicy(ctx context.Context, t *testing.T, tenant, collection shared.ID, strategy, candidates string) {
	t.Helper()

	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO auto_assign_policy (id, tenant_id, scope_type, scope_id, strategy, candidates)
		 VALUES ($1, $2, 'COLLECTION', $3, $4, $5::jsonb)`,
		freshID(t).String(), tenant.String(), collection.String(), strategy, candidates); err != nil {
		t.Fatalf("seeding the policy: %v", err)
	}
}

// A group's queue (user-experience.md §3.3): the entries of a collection whose auto-assign policy
// names a group the reader is in; a policy naming another group, or the reader as an account, is
// not their group's queue, and another workspace's policy naming the same group answers nothing.
func TestAGroupsQueueIsTheCollectionsWhosePolicyNamesTheReadersGroup(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	reader := seedAccount(ctx, t, tenantA)
	theirs := seedGroupWith(ctx, t, tenantA, reader)
	other := seedGroupWith(ctx, t, tenantA, seedAccount(ctx, t, tenantA))

	_, queued := hubWithCollection(ctx, t, tenantA, authorA)
	seedPolicy(ctx, t, tenantA, queued, "RANDOM_GROUP_MEMBER", `[{"kind":"GROUP","id":"`+theirs.String()+`"}]`)
	_, elsewhere := hubWithCollection(ctx, t, tenantA, authorA)
	seedPolicy(ctx, t, tenantA, elsewhere, "RANDOM_GROUP_MEMBER", `[{"kind":"GROUP","id":"`+other.String()+`"}]`)
	_, personal := hubWithCollection(ctx, t, tenantA, authorA)
	seedPolicy(ctx, t, tenantA, personal, "FIXED", `[{"kind":"ACCOUNT","id":"`+reader.String()+`"}]`)

	inQueue := seedTask(ctx, t, tenantA, authorA, queued)
	seedTask(ctx, t, tenantA, authorA, elsewhere)
	seedTask(ctx, t, tenantA, authorA, personal)

	ask := map[string]any{"field": view.FieldGroupQueueOf, "op": "EQ", "value": reader.String()}
	if got := queried(ctx, t, tenantA, searchIn(t, queued, ask, view.Spec{})); len(got.Items) != 1 || got.Items[0].ID != inQueue {
		t.Errorf("the group's queue answered %v", titlesOf(got.Items))
	}
	for name, collection := range map[string]shared.ID{"another group's": elsewhere, "the reader's own account": personal} {
		if got := queried(ctx, t, tenantA, searchIn(t, collection, ask, view.Spec{})); len(got.Items) != 0 {
			t.Errorf("a policy naming %s answered %v", name, titlesOf(got.Items))
		}
	}

	// Cross-tenant: another workspace's policy naming the group, read there, finds no membership.
	_, foreign := hubWithCollection(ctx, t, tenantB, authorB)
	seedPolicy(ctx, t, tenantB, foreign, "RANDOM_GROUP_MEMBER", `[{"kind":"GROUP","id":"`+theirs.String()+`"}]`)
	seedTask(ctx, t, tenantB, authorB, foreign)
	if got := queried(ctx, t, tenantB, searchIn(t, foreign, ask, view.Spec{})); len(got.Items) != 0 {
		t.Errorf("another workspace's queue answered %v", titlesOf(got.Items))
	}
}
