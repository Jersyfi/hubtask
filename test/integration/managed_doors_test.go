// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// The doors through the boundary answer on a database the operator brings (multi-tenancy.md §2.1).
// Every SECURITY DEFINER function that reads across workspaces runs as the owner, and FORCE binds
// an owner that is neither superuser nor BYPASSRLS: without the owner's own policies, sign-in found
// no workspace, the census counted nothing and the register named nobody. Each door is called the
// way the product calls it - as the application role, with no workspace set. The rows are seeded by
// the server's superuser, so that seeding asks nothing of the policies under test.
func TestTheDoorsAnswerOnADatabaseWithoutASuperuser(t *testing.T) {
	ctx := context.Background()
	const name = "hubtask_managed_doors"
	ownerDSN, appDSN := managedDatabase(ctx, t, name)
	if err := dbtest.Migrate(ctx, ownerDSN); err != nil {
		t.Fatalf("migrating: %v", err)
	}
	seeder, err := url.Parse(dbtest.Start(t).AdminDSN)
	if err != nil {
		t.Fatalf("the shared DSN is not a URL: %v", err)
	}
	seeder.Path = "/" + name
	super, err := pgxpool.New(ctx, seeder.String())
	if err != nil {
		t.Fatalf("superuser pool: %v", err)
	}
	defer super.Close()

	const (
		tenantA  = "01936f2a-7c1e-7000-8000-000000145601"
		tenantB  = "01936f2a-7c1e-7000-8000-000000145602"
		ownerA   = "01936f2a-7c1e-7000-8000-000000145611"
		personB  = "01936f2a-7c1e-7000-8000-000000145612"
		provider = "01936f2a-7c1e-7000-8000-000000145621"
	)
	for _, statement := range []string{
		`INSERT INTO tenant (id, slug, display_name, settings) VALUES
		 ('` + tenantA + `', 'door-a', 'Door A', '{"offered_providers": ["` + provider + `"]}'),
		 ('` + tenantB + `', 'door-b', 'Door B', '{}')`,
		`INSERT INTO account (id, tenant_id, kind, email, display_name, status) VALUES
		 ('` + ownerA + `', '` + tenantA + `', 'USER', 'same@door.example', 'Ada', 'ACTIVE'),
		 ('` + personB + `', '` + tenantB + `', 'USER', 'same@door.example', 'Ben', 'ACTIVE')`,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES
		 ('01936f2a-7c1e-7000-8000-000000145631', '` + tenantA + `', '` + ownerA + `', 'TENANT', 'OWNER')`,
		`INSERT INTO identity_provider (id, tenant_id, issuer, client_id, client_secret_enc, client_secret_key_id, created_at)
		 VALUES ('` + provider + `', NULL, 'https://door.example/idp', 'hubtask', '\x00', 'k-door', now())`,
	} {
		if _, err := super.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	app, err := pgxpool.New(ctx, appDSN)
	if err != nil {
		t.Fatalf("application pool: %v", err)
	}
	defer app.Close()
	one := func(t *testing.T, query string, into any, args ...any) {
		t.Helper()
		if err := app.QueryRow(ctx, query, args...).Scan(into); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}

	t.Run("sign-in finds the workspace", func(t *testing.T) {
		var a, b *string
		one(t, `SELECT resolve_tenant('door-a')::text`, &a)
		one(t, `SELECT resolve_tenant('door-b')::text`, &b)
		if a == nil || *a != tenantA || b == nil || *b != tenantB {
			t.Errorf("resolve_tenant answered %v and %v", a, b)
		}
	})

	t.Run("the operator's list and the census count both workspaces", func(t *testing.T) {
		var listed, active, accounts, workspaces int
		var unset bool
		one(t, `SELECT count(*) FROM admin_tenants()`, &listed)
		one(t, `SELECT workspaces_active FROM instance_census()`, &active)
		one(t, `SELECT accounts_total FROM instance_census()`, &accounts)
		one(t, `SELECT workspaces FROM operator_register_state()`, &workspaces)
		one(t, `SELECT installation_unset()`, &unset)
		if listed != 2 || active != 2 || accounts != 2 || workspaces != 2 || unset {
			t.Errorf("admin_tenants %d, census active %d and accounts %d, register state %d, unset %t",
				listed, active, accounts, workspaces, unset)
		}
	})

	t.Run("a data subject is found in every workspace", func(t *testing.T) {
		var found int
		one(t, `SELECT count(*) FROM subject_tenants('SAME@door.example')`, &found)
		if found != 2 {
			t.Errorf("subject_tenants found %d workspaces, want 2", found)
		}
	})

	t.Run("the operator register reads accounts and memberships", func(t *testing.T) {
		var resolved *string
		one(t, `SELECT resolve_operator_account('door-b', 'same@door.example')::text`, &resolved)
		if resolved == nil || *resolved != personB {
			t.Errorf("resolve_operator_account answered %v", resolved)
		}
		var outcome string
		one(t, `SELECT enlist_operator($1, $1)`, &outcome, ownerA)
		if outcome != "ADDED" {
			t.Errorf("enlist_operator answered %q, want ADDED", outcome)
		}
		var isOperator, isNot bool
		var roster int
		one(t, `SELECT is_operator($1)`, &isOperator, ownerA)
		one(t, `SELECT is_operator($1)`, &isNot, personB)
		one(t, `SELECT count(*) FROM operator_roster($1) WHERE workspace_slug = 'door-a'`, &roster, ownerA)
		if !isOperator || isNot || roster != 1 {
			t.Errorf("is_operator %t for the operator and %t for another, roster %d", isOperator, isNot, roster)
		}
	})

	t.Run("an offered provider is counted and moved", func(t *testing.T) {
		var offers int
		one(t, `SELECT count_provider_offers($1)`, &offers, provider)
		if offers != 1 {
			t.Errorf("count_provider_offers answered %d, want 1", offers)
		}
		if _, err := app.Exec(ctx, `SELECT move_provider_offer($1, 1)`, provider); err != nil {
			t.Fatalf("move_provider_offer: %v", err)
		}
		var moved int
		if err := super.QueryRow(ctx,
			`SELECT offered_workspaces FROM identity_provider WHERE id = $1`, provider).Scan(&moved); err != nil {
			t.Fatalf("reading the provider: %v", err)
		}
		if moved != 1 {
			t.Errorf("move_provider_offer left %d offered workspaces, want 1", moved)
		}
	})

	// The owner's policies name the owner and nobody else: the application, reading the same tables
	// directly with no workspace set, still sees nothing.
	t.Run("the application role is still bounded", func(t *testing.T) {
		var tenants, accounts, memberships, providers int
		one(t, `SELECT count(*) FROM tenant`, &tenants)
		one(t, `SELECT count(*) FROM account`, &accounts)
		one(t, `SELECT count(*) FROM membership`, &memberships)
		one(t, `SELECT count(*) FROM identity_provider WHERE tenant_id IS NOT NULL`, &providers)
		if tenants+accounts+memberships+providers != 0 {
			t.Errorf("without a workspace the application read %d tenants, %d accounts, %d memberships, %d providers",
				tenants, accounts, memberships, providers)
		}
	})
}
