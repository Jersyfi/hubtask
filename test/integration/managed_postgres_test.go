// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// The third way an installation gets a database: one the operator brings and hands over as a
// connection string (ADR-0052). `database.enabled` is off in the chart by default, and every
// deployment reads its DSN from a Secret, so the chart has always supported it. The *migrations*
// did not.
//
// The reason this suite never noticed is the reason it exists now: Compose, CI and the integration
// environment all migrate as the PostgreSQL **superuser**, and a superuser ignores
// `FORCE ROW LEVEL SECURITY`. A managed service hands out an owner with `CREATEROLE` and no
// superuser, and under that role migration 0002 could not seed the system defaults its own policy
// forbids anybody to write.
//
// So this test migrates the way a managed service would, in a database of its own on the shared
// container. What it guards is not a feature but an absence of privilege: the day somebody writes a
// migration that only a superuser can apply, this turns red instead of a customer's installation.

// managedOwnerPassword is a fixture, not a secret: it lives for the length of the container.
const managedOwnerPassword = "test-only-managed-owner"

// managedDatabase creates a database owned by hubtask_migrator, the role the schema's own grants
// name, and hands back the DSNs to reach it as the owner and as the application.
//
// The two roles already exist, created by the shared database's migration - which is exactly the
// arrangement `0001_init.sql` describes for a managed instance: "the operator creates the two roles
// beforehand". All this adds is a login for the owner, because a managed service would have handed
// one over.
func managedDatabase(ctx context.Context, t *testing.T, name string) (ownerDSN, appDSN string) {
	t.Helper()
	shared := dbtest.Start(t)

	admin, err := pgxpool.New(ctx, shared.AdminDSN)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer admin.Close()

	// ALTER ROLE and CREATE DATABASE take no parameters, so the two values that reach them are a
	// constant and a name this file chose. Quoted anyway, which is the habit rule 9 rests on.
	if _, err := admin.Exec(ctx,
		"ALTER ROLE hubtask_migrator WITH LOGIN PASSWORD '"+managedOwnerPassword+"'"); err != nil {
		t.Fatalf("granting the owner a login: %v", err)
	}
	if _, err := admin.Exec(ctx, `DROP DATABASE IF EXISTS "`+name+`"`); err != nil {
		t.Fatalf("dropping a previous %s: %v", name, err)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE "`+name+`" OWNER hubtask_migrator`); err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		pool, err := pgxpool.New(cleanup, shared.AdminDSN)
		if err != nil {
			return
		}
		defer pool.Close()
		_, _ = pool.Exec(cleanup, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`)
	})

	return rewrite(t, shared.AdminDSN, "hubtask_migrator", managedOwnerPassword, name),
		rewrite(t, shared.AdminDSN, "hubtask_app", dbtest.AppPassword, name)
}

// inRawTenant runs fn inside a transaction with the tenant set the way the transaction wrapper sets
// it (ADR-0010).
//
// The rollback is deferred rather than trailing, and that is not tidiness: a `t.Fatalf` inside a
// transaction ends the test's goroutine, the connection is never returned, and the pool's Close
// waits for it until the package's timeout. The failure then reads as "the suite hung" rather than
// as the assertion that actually failed.
func inRawTenant(ctx context.Context, t *testing.T, pool *pgxpool.Pool, tenantID string, commit bool, fn func(pgx.Tx)) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenantID); err != nil {
		t.Fatalf("setting the tenant: %v", err)
	}
	fn(tx)
	if commit {
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
}

// rewrite points a DSN at another role and another database on the same server.
func rewrite(t *testing.T, dsn, user, password, database string) string {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("the shared DSN is not a URL: %v", err)
	}
	parsed.User = url.UserPassword(user, password)
	parsed.Path = "/" + database
	return parsed.String()
}

// The regression guard, and the whole point of the file: every migration applies under an owner
// that is not a superuser and cannot become one.
func TestTheMigrationsApplyWithoutASuperuser(t *testing.T) {
	ctx := context.Background()
	ownerDSN, _ := managedDatabase(ctx, t, "hubtask_managed")

	owner, err := pgxpool.New(ctx, ownerDSN)
	if err != nil {
		t.Fatalf("owner pool: %v", err)
	}
	defer owner.Close()

	// What a managed service gives you, stated rather than assumed - if this ever became a
	// superuser the test below would prove nothing at all.
	var super, bypass, createRole bool
	if err := owner.QueryRow(ctx, `
		SELECT rolsuper, rolbypassrls, rolcreaterole FROM pg_roles WHERE rolname = 'hubtask_migrator'`).
		Scan(&super, &bypass, &createRole); err != nil {
		t.Fatalf("reading the owner: %v", err)
	}
	if super || bypass {
		t.Fatalf("the owner is superuser=%t bypassrls=%t, so this test is not testing anything", super, bypass)
	}

	if err := dbtest.Migrate(ctx, ownerDSN); err != nil {
		t.Fatalf("the migrations do not apply on a database an operator brings: %v", err)
	}

	// The seed migration 0002 could not write before ADR-0052.
	var defaults int
	if err := owner.QueryRow(ctx,
		`SELECT count(*) FROM item_capability_profile WHERE tenant_id IS NULL`).Scan(&defaults); err != nil {
		t.Fatalf("reading the system defaults: %v", err)
	}
	if defaults != 3 {
		t.Errorf("the system defaults are %d rows, want 3", defaults)
	}
}

// And the claim ADR-0052 turns on: freeing the owner freed nobody else. This is the test that has
// to fail if the correction is ever widened into a hole.
func TestTheBoundaryHoldsOnADatabaseWithoutASuperuser(t *testing.T) {
	ctx := context.Background()
	ownerDSN, appDSN := managedDatabase(ctx, t, "hubtask_managed_boundary")
	if err := dbtest.Migrate(ctx, ownerDSN); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	app, err := pgxpool.New(ctx, appDSN)
	if err != nil {
		t.Fatalf("application pool: %v", err)
	}
	defer app.Close()

	t.Run("the application role is still bounded", func(t *testing.T) {
		var super, bypass bool
		if err := app.QueryRow(ctx,
			`SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'hubtask_app'`).
			Scan(&super, &bypass); err != nil {
			t.Fatalf("reading the role: %v", err)
		}
		if super || bypass {
			t.Errorf("hubtask_app is superuser=%t bypassrls=%t", super, bypass)
		}
	})

	t.Run("it may read the system defaults", func(t *testing.T) {
		var visible int
		if err := app.QueryRow(ctx,
			`SELECT count(*) FROM item_capability_profile WHERE tenant_id IS NULL`).Scan(&visible); err != nil {
			t.Fatalf("query: %v", err)
		}
		if visible != 3 {
			t.Errorf("the application sees %d system defaults, want 3", visible)
		}
	})

	t.Run("and may not write them", func(t *testing.T) {
		_, err := app.Exec(ctx, `
			INSERT INTO item_capability_profile (tenant_id, type, capabilities, allowed_child_types, max_depth)
			VALUES (NULL, 'TASK', ARRAY['COMPLETION'], ARRAY[]::item_type[], 3)`)
		if err == nil {
			t.Fatal("the application wrote a system default")
		}
		if !strings.Contains(err.Error(), "row-level security") {
			t.Errorf("refused for the wrong reason: %v", err)
		}
	})

	t.Run("without a tenant it sees nothing", func(t *testing.T) {
		var tenants, items int
		if err := app.QueryRow(ctx,
			`SELECT (SELECT count(*) FROM tenant), (SELECT count(*) FROM work_item)`).
			Scan(&tenants, &items); err != nil {
			t.Fatalf("query: %v", err)
		}
		if tenants != 0 || items != 0 {
			t.Errorf("visible without a tenant: %d tenants, %d items", tenants, items)
		}
	})

	// The behavioural half: two workspaces written through the application role the way the
	// transaction wrapper writes them, and neither sees the other.
	t.Run("one tenant sees nothing of another", func(t *testing.T) {
		const (
			first  = "11111111-1111-4111-8111-111111111111"
			second = "22222222-2222-4222-8222-222222222222"
		)
		for i, id := range []string{first, second} {
			inRawTenant(ctx, t, app, id, true, func(tx pgx.Tx) {
				if _, err := tx.Exec(ctx,
					`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, $3)`,
					id, fmt.Sprintf("managed-%d", i), fmt.Sprintf("Managed %d", i)); err != nil {
					t.Fatalf("writing a workspace: %v", err)
				}
			})
		}

		for _, id := range []string{first, second} {
			inRawTenant(ctx, t, app, id, false, func(tx pgx.Tx) {
				var visible int
				var seen string
				if err := tx.QueryRow(ctx,
					`SELECT count(*), coalesce(max(id::text), '') FROM tenant`).Scan(&visible, &seen); err != nil {
					t.Fatalf("query: %v", err)
				}
				if visible != 1 || seen != id {
					t.Errorf("under %s the application sees %d workspaces (%s)", id, visible, seen)
				}
			})
		}
	})

	t.Run("exactly one table gives up FORCE", func(t *testing.T) {
		owner, err := pgxpool.New(ctx, ownerDSN)
		if err != nil {
			t.Fatalf("owner pool: %v", err)
		}
		defer owner.Close()

		rows, err := owner.Query(ctx, `
			SELECT c.relname
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant_id' AND NOT a.attisdropped
			WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND NOT c.relispartition
			  AND NOT (c.relrowsecurity AND c.relforcerowsecurity)
			ORDER BY c.relname`)
		if err != nil {
			t.Fatalf("catalogue query: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var table string
			if err := rows.Scan(&table); err != nil {
				t.Fatalf("scan: %v", err)
			}
			if _, documented := rlsExceptions[table]; !documented {
				t.Errorf("%s has no forced row level security and no entry saying why", table)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
	})
}
