// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Custom roles against the real boundary (UC-ID-26, gate SG-3): a workspace defines a role, grants
// it at a hub, and the grant reaches the decision through Memberships.Along - and the workspace
// next door neither reads, nor changes, nor revokes, nor holds any of it.
func TestAWorkspacesCustomRolesAreItsOwn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, collection := hubWithCollection(ctx, t, tenantA, authorA)
	for _, statement := range []string{
		`DELETE FROM custom_role WHERE id = '01936f2a-7c1e-7000-8000-00000000fe01'`,
	} {
		if _, err := adminPool(ctx, t).Exec(ctx, statement); err != nil {
			t.Fatalf("clearing an earlier run: %v", err)
		}
	}

	roles := postgres.NewCustomRoleRepository()
	memberships := postgres.NewMembershipRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC().Truncate(time.Microsecond)
	role, err := service.DefineRole(shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe01"), tenantA,
		"service_lead", "Service lead", []string{"READ", "STRUCTURE", "FIELD_WRITE:cost"})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := identity.NewCustomGrant(shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fe02"), tenantA,
		authorA, "", identity.HubScope(hub), role.ID)
	if err != nil {
		t.Fatal(err)
	}
	path := []identity.Scope{identity.TenantScope(), identity.HubScope(hub), identity.CollectionScope(collection)}

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		if err := roles.Insert(ctx, role, now); err != nil {
			t.Fatalf("defining A's role: %v", err)
		}
		return nil
	})
	// The refused insert aborts its transaction, so it is the transaction's answer.
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		return roles.Insert(ctx, role, now)
	}); err == nil {
		t.Fatal("a key defined twice")
	}
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		created, err := roles.Grant(ctx, grant, now)
		if err != nil || !created {
			t.Fatalf("granting: %v %v", created, err)
		}
		again, err := roles.Grant(ctx, grant, now)
		if err != nil || again {
			t.Fatalf("a grant written twice: %v %v", again, err)
		}
		held, err := memberships.Along(ctx, authorA, path)
		if err != nil {
			t.Fatalf("reading what A's author holds: %v", err)
		}
		if !service.Allows(held, path, service.SharedPath, service.PermissionStructure) ||
			!service.Allows(held, path, service.SharedPath, service.FieldWrite("cost")) {
			t.Fatalf("the custom role does not reach the decision: %+v", held)
		}
		return nil
	})

	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		listed, err := roles.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range listed {
			if row.Role.ID == role.ID {
				t.Fatal("B lists A's role")
			}
		}
		if _, err := roles.Find(ctx, "service_lead"); err == nil {
			t.Fatal("B finds A's role")
		}
		if written, err := roles.Update(ctx, role, 1, now); err != nil || written {
			t.Fatalf("B changes A's role: %v %v", written, err)
		}
		if _, err := roles.FindGrant(ctx, grant.ID); err == nil {
			t.Fatal("B finds A's grant")
		}
		grants, err := roles.Grants(ctx, 100)
		if err != nil || len(grants) != 0 {
			t.Fatalf("B lists grants: %v %v", grants, err)
		}
		if revoked, err := roles.Revoke(ctx, grant.ID); err != nil || revoked {
			t.Fatalf("B revokes A's grant: %v %v", revoked, err)
		}
		if deleted, err := roles.Delete(ctx, role.ID); err != nil || deleted {
			t.Fatalf("B deletes A's role: %v %v", deleted, err)
		}
		held, err := memberships.Along(ctx, authorA, path)
		if err != nil {
			t.Fatal(err)
		}
		for _, membership := range held {
			if membership.IsCustom() {
				t.Fatal("A's custom role is held in B")
			}
		}
		return nil
	})

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		found, err := roles.FindGrant(ctx, grant.ID)
		if err != nil || found.RoleKey != "service_lead" {
			t.Fatalf("the grant: %+v %v", found, err)
		}
		if deleted, err := roles.Delete(ctx, role.ID); err != nil || !deleted {
			t.Fatalf("deleting: %v %v", deleted, err)
		}
		if _, err := roles.FindGrant(ctx, grant.ID); err == nil {
			t.Fatal("the grant outlived its role")
		}
		return nil
	})
}
