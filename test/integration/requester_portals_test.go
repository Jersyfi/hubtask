// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

func portalsOf(ctx context.Context, t *testing.T, tenant, account shared.ID) []repository.RequesterPortal {
	t.Helper()
	var portals []repository.RequesterPortal
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		portals, err = postgres.NewMembershipRepository().PortalsOf(ctx, account)
		return err
	}); err != nil {
		t.Fatalf("reading the portals: %v", err)
	}
	return portals
}

// A requester's portals against PostgreSQL (UC-SVC-05): a role of the workspace's own carrying
// REQUEST at a collection, held through a group, names that collection; one without REQUEST names
// none; and another tenant's reader finds nothing of it (gate SG-3).
func TestARequesterFindsThePortalsItsRolesOpen(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	hub, desk, quiet, requester, reader, grant, other := freshID(t), freshID(t), freshID(t), freshID(t), freshID(t), freshID(t), freshID(t)
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		  VALUES ($1, $2, 'HUB', 'Support', 'a0', $3)`, []any{hub.String(), tenantA.String(), authorA.String()}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'COLLECTION', $3, 'Service desk', 'a0', $4)`,
			[]any{desk.String(), tenantA.String(), hub.String(), authorA.String()}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'COLLECTION', $3, 'Backlog', 'a1', $4)`,
			[]any{quiet.String(), tenantA.String(), hub.String(), authorA.String()}},
		{`INSERT INTO custom_role (id, tenant_id, key, name, permissions, created_at, updated_at)
		  VALUES ($1, $2, $3, 'Customer', ARRAY['REQUEST'], now(), now())`,
			[]any{requester.String(), tenantA.String(), "customer_" + requester.String()[30:]}},
		{`INSERT INTO custom_role (id, tenant_id, key, name, permissions, created_at, updated_at)
		  VALUES ($1, $2, $3, 'Reader', ARRAY['READ'], now(), now())`,
			[]any{reader.String(), tenantA.String(), "reader_" + reader.String()[30:]}},
		{`INSERT INTO custom_role_grant (id, tenant_id, group_id, scope_type, scope_id, role_id, created_at)
		  VALUES ($1, $2, $3, 'COLLECTION', $4, $5, now())`,
			[]any{grant.String(), tenantA.String(), membershipGroup.String(), desk.String(), requester.String()}},
		{`INSERT INTO custom_role_grant (id, tenant_id, group_id, scope_type, scope_id, role_id, created_at)
		  VALUES ($1, $2, $3, 'COLLECTION', $4, $5, now())`,
			[]any{other.String(), tenantA.String(), membershipGroup.String(), quiet.String(), reader.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v\n%s", err, statement.sql)
		}
	}

	portals := portalsOf(ctx, t, tenantA, authorA)
	found := false
	for _, portal := range portals {
		if portal.CollectionID == quiet {
			t.Errorf("a role without REQUEST opened a portal: %v", portals)
		}
		found = found || (portal.CollectionID == desk && portal.Name == "Service desk")
	}
	if !found {
		t.Errorf("the requester's portal is missing: %v", portals)
	}
	if portals := portalsOf(ctx, t, tenantB, authorA); len(portals) != 0 {
		t.Errorf("another tenant's reader found portals: %v", portals)
	}
}
