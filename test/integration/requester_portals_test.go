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

func holdsAny(ctx context.Context, t *testing.T, tenant, account shared.ID) bool {
	t.Helper()
	var held bool
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		held, err = postgres.NewMembershipRepository().HoldsAny(ctx, account)
		return err
	}); err != nil {
		t.Fatalf("reading whether the account holds a role: %v", err)
	}
	return held
}

// Roles of the workspace's own count as access, and a requester granted at the workspace finds
// the portals outside a private hub: a person with a READ role there is not told to ask an
// administrator, one with REQUEST alone is sent to the portal (UC-SVC-05), and another tenant's
// reader finds neither (gate SG-3).
func TestAWorkspaceRoleCountsAsAccessAndAWorkspaceRequesterFindsItsPortals(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	ids := make([]shared.ID, 10)
	for i := range ids {
		ids[i] = freshID(t)
	}
	reader, requester, readRole, requestRole, hub, hidden, desk, secret := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5], ids[6], ids[7]
	readGrant, requestGrant := ids[8], ids[9]
	admin := adminPool(ctx, t)
	// A workspace of its own: a workspace grant reaches every collection, and the shared tenant
	// holds the package's other fixtures - more than the fifty a portal list answers.
	workspace, author := seedOwnTenant(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Rita')`, []any{reader.String(), workspace.String()}},
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Quinn')`, []any{requester.String(), workspace.String()}},
		{`INSERT INTO custom_role (id, tenant_id, key, name, permissions, created_at, updated_at)
		  VALUES ($1, $2, $3, 'Contractor', ARRAY['READ'], now(), now())`,
			[]any{readRole.String(), workspace.String(), "contractor_" + readRole.String()[30:]}},
		{`INSERT INTO custom_role (id, tenant_id, key, name, permissions, created_at, updated_at)
		  VALUES ($1, $2, $3, 'Client', ARRAY['REQUEST'], now(), now())`,
			[]any{requestRole.String(), workspace.String(), "client_" + requestRole.String()[30:]}},
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		  VALUES ($1, $2, 'HUB', 'Help', 'a0', $3)`, []any{hub.String(), workspace.String(), author.String()}},
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by, private)
		  VALUES ($1, $2, 'HUB', 'Board room', 'a1', $3, true)`, []any{hidden.String(), workspace.String(), author.String()}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'COLLECTION', $3, 'Help desk', 'a0', $4)`,
			[]any{desk.String(), workspace.String(), hub.String(), author.String()}},
		{`INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  VALUES ($1, $2, 'COLLECTION', $3, 'Minutes', 'a0', $4)`,
			[]any{secret.String(), workspace.String(), hidden.String(), author.String()}},
		{`INSERT INTO custom_role_grant (id, tenant_id, account_id, scope_type, scope_id, role_id, created_at)
		  VALUES ($1, $2, $3, 'HUB', $4, $5, now())`,
			[]any{readGrant.String(), workspace.String(), reader.String(), hub.String(), readRole.String()}},
		{`INSERT INTO custom_role_grant (id, tenant_id, account_id, scope_type, role_id, created_at)
		  VALUES ($1, $2, $3, 'TENANT', $4, now())`,
			[]any{requestGrant.String(), workspace.String(), requester.String(), requestRole.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v\n%s", err, statement.sql)
		}
	}

	if !holdsAny(ctx, t, workspace, reader) {
		t.Error("a person holding a READ role of the workspace's own was answered as holding nothing")
	}
	if holdsAny(ctx, t, workspace, requester) {
		t.Error("a requester's role was counted as access; their place is the portal")
	}
	portals := portalsOf(ctx, t, workspace, requester)
	found := false
	for _, portal := range portals {
		if portal.CollectionID == secret {
			t.Errorf("a workspace grant opened a portal in a private hub: %v", portals)
		}
		found = found || portal.CollectionID == desk
	}
	if !found {
		t.Errorf("a requester granted at the workspace found no portal: %v", portals)
	}
	if holdsAny(ctx, t, tenantB, reader) {
		t.Error("another tenant's reader found the role")
	}
	if portals := portalsOf(ctx, t, tenantB, requester); len(portals) != 0 {
		t.Errorf("another tenant's reader found portals: %v", portals)
	}
}
