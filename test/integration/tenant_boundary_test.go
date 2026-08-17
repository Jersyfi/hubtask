// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

var (
	tenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000000a")
	tenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000000b")
)

// The acceptance criteria of A-03 (security.md §6, multi-tenancy.md §2.1).

func TestTheApplicationRoleCannotBypassRowLevelSecurity(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)

	var bypassRLS, superuser, isReplication bool
	err := admin.QueryRow(ctx,
		`SELECT rolbypassrls, rolsuper, rolreplication FROM pg_roles WHERE rolname = 'hubtask_app'`).
		Scan(&bypassRLS, &superuser, &isReplication)
	if err != nil {
		t.Fatalf("the role hubtask_app does not exist: %v", err)
	}

	if bypassRLS {
		t.Error("hubtask_app holds BYPASSRLS - the tenant boundary is decoration")
	}
	if superuser {
		t.Error("hubtask_app is a superuser, which ignores every policy")
	}
	if isReplication {
		t.Error("hubtask_app may replicate, which reads the whole cluster")
	}

	// Owning a table would be the quieter way around a policy: FORCE ROW LEVEL SECURITY is
	// what stops it, and the role owning nothing is the belt to that pair of braces.
	var owned int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r','p')
		  AND pg_get_userbyid(c.relowner) = 'hubtask_app'`).Scan(&owned); err != nil {
		t.Fatalf("ownership query: %v", err)
	}
	if owned != 0 {
		t.Errorf("hubtask_app owns %d tables", owned)
	}
}

// Every tenant table carries RLS, and it is FORCE - without that the owner would be exempt.
// The list comes from the catalogue rather than from a constant in this file: a table added
// later without a policy has to turn this red on its own.
func TestRowLevelSecurityIsActiveOnEveryTenantTable(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)

	// The documented exceptions. Anything else missing RLS is a finding.
	exceptions := map[string]string{
		"job":              "system jobs are partly tenant-less; access is restricted by privileges (db/schema.sql)",
		"goose_db_version": "the migration ledger; the application role has no access at all",
	}

	rows, err := admin.Query(ctx, `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r','p')
		ORDER BY c.relname`)
	if err != nil {
		t.Fatalf("catalogue query: %v", err)
	}
	defer rows.Close()

	checked := 0
	for rows.Next() {
		var name string
		var enabled, forced bool
		if err := rows.Scan(&name, &enabled, &forced); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if reason, ok := exceptions[name]; ok {
			if enabled {
				t.Logf("note: %s now has RLS although it is listed as an exception (%s)", name, reason)
			}
			continue
		}
		checked++
		if !enabled {
			t.Errorf("%s has no row level security", name)
		}
		if !forced {
			t.Errorf("%s does not FORCE row level security, so its owner is exempt", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if checked < 40 {
		t.Errorf("only %d tables checked - the catalogue query is not seeing the schema", checked)
	}
}

// A partition is a table of its own. PostgreSQL applies the policies of the relation named in
// the query, so a policy on the parent does not protect a partition addressed directly - this
// is the hole the migration closes, and the test that keeps it closed.
func TestAPartitionCannotBeUsedToReadAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedAuditRows(ctx, t)

	pool := appPool(ctx, t)
	uow := postgres.NewUnitOfWork(pool)

	// The question is not how many rows a relation holds - the default partition legitimately
	// holds none - but whether any of them belong to the other tenant.
	for _, relation := range []string{"audit_log", "audit_log_2026_08", "audit_log_default"} {
		t.Run(relation, func(t *testing.T) {
			var foreign int
			err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
				tx, err := postgres.FromContext(ctx)
				if err != nil {
					return err
				}
				return tx.QueryRow(ctx,
					`SELECT count(*) FROM `+relation+` WHERE tenant_id = $1`, tenantB.String()).Scan(&foreign)
			})
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if foreign != 0 {
				t.Errorf("%s shows %d rows of the other tenant under tenant A's context", relation, foreign)
			}
		})
	}

	// And the policy is not simply denying everything: the tenant's own row is visible, through
	// the parent and through the partition that holds it.
	//
	// Counted by the fixture's own action rather than as a total. The package shares one database
	// and other tests append to this tenant's trail; an exact total would make this assertion
	// depend on how many of them ran first, which is not what it is about.
	for _, relation := range []string{"audit_log", "audit_log_2026_08"} {
		t.Run(relation+" own rows", func(t *testing.T) {
			var own int
			err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
				tx, err := postgres.FromContext(ctx)
				if err != nil {
					return err
				}
				return tx.QueryRow(ctx,
					`SELECT count(*) FROM `+relation+` WHERE action = 'seeded'`).Scan(&own)
			})
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if own != 1 {
				t.Errorf("%s returns %d of the tenant's own rows, want 1", relation, own)
			}
		})
	}
}

// Without a tenant context every query returns nothing. A programming error therefore reads as
// "not found", never as another tenant's data (multi-tenancy.md §2.1).
func TestWithoutATenantContextNothingIsVisible(t *testing.T) {
	ctx := context.Background()
	seedAuditRows(ctx, t)
	pool := appPool(ctx, t)

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 0 {
		t.Errorf("a query without a tenant context returned %d rows", count)
	}
}

// The wrapper refuses to open a transaction without a tenant rather than opening one that
// silently sees nothing.
func TestAUnitOfWorkWithoutATenantIsRefused(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	err := uow.Within(ctx, persistence.Scope{}, func(context.Context) error {
		t.Error("the transaction body ran without a tenant")
		return nil
	})

	if err == nil {
		t.Fatal("a unit of work without a tenant was accepted")
	}
	var domainErr *shared.Error
	if !errorsAs(err, &domainErr) || domainErr.DetailCode != "postgres.scope_missing" {
		t.Errorf("error = %v, want postgres.scope_missing", err)
	}
}

func TestAForeignTenantSeesNothingOfAnother(t *testing.T) {
	ctx := context.Background()
	seedAuditRows(ctx, t)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	for _, tc := range []struct {
		name   string
		tenant shared.ID
		want   int
	}{
		{"own rows", tenantA, 1},
		{"the other tenant's rows", tenantB, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var count int
			err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tc.tenant}, func(ctx context.Context) error {
				tx, err := postgres.FromContext(ctx)
				if err != nil {
					return err
				}
				// The fixture's own row, for the reason given above: the total belongs to
				// whichever tests ran first, the isolation does not.
				return tx.QueryRow(ctx,
					`SELECT count(*) FROM audit_log WHERE action = 'seeded'`).Scan(&count)
			})
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if count != tc.want {
				t.Errorf("count = %d, want %d", count, tc.want)
			}
		})
	}
}

// A write for another tenant is rejected by the WITH CHECK half of the policy, which is the
// half that stops a cross-tenant insert rather than merely hiding one.
func TestWritingForAnotherTenantIsRejected(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		tx, err := postgres.FromContext(ctx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO audit_log (id, tenant_id, seq, occurred_at, action, outcome, actor_type, hash)
			VALUES (gen_random_uuid(), $1, 99, now(), 'smuggled', 'SUCCESS', 'SYSTEM', '\x00')`, tenantB.String())
		return err
	})

	if err == nil {
		t.Fatal("a row was written for another tenant")
	}
}

// The tenant setting is bound to the transaction, so a connection handed back to the pool
// carries nothing with it. Without that, the next caller on the same connection would inherit
// a tenant - the failure pgbouncer would make routine.
func TestTheTenantContextDoesNotOutliveTheTransaction(t *testing.T) {
	ctx := context.Background()
	seedAuditRows(ctx, t)
	pool := appPool(ctx, t)
	uow := postgres.NewUnitOfWork(pool)

	if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		return nil
	}); err != nil {
		t.Fatalf("first transaction: %v", err)
	}

	// Same pool, no scope: whatever the previous transaction set must be gone.
	var setting string
	if err := pool.QueryRow(ctx, `SELECT coalesce(current_setting('app.tenant_id', true), '')`).Scan(&setting); err != nil {
		t.Fatalf("query: %v", err)
	}
	if setting != "" {
		t.Errorf("app.tenant_id survived the transaction: %q", setting)
	}
}

// Switching tenant inside a running transaction would run the inner work under the outer
// SET LOCAL - the wrong tenant, silently.
func TestSwitchingTenantInsideATransactionIsRefused(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		return uow.Within(ctx, persistence.Scope{TenantID: tenantB}, func(context.Context) error {
			t.Error("the inner body ran under the outer tenant's context")
			return nil
		})
	})

	if err == nil {
		t.Fatal("a tenant switch inside a transaction was accepted")
	}
	var domainErr *shared.Error
	if !errorsAs(err, &domainErr) || domainErr.DetailCode != "postgres.tenant_switch_in_transaction" {
		t.Errorf("error = %v, want postgres.tenant_switch_in_transaction", err)
	}
}

// The audit trail is append-only for the application role: no UPDATE, no DELETE, on the parent
// and on every partition (audit.md, gate SG-4).
func TestTheApplicationRoleCannotRewriteTheAuditTrail(t *testing.T) {
	ctx := context.Background()
	seedAuditRows(ctx, t)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	for _, statement := range []string{
		`UPDATE audit_log SET action = 'rewritten'`,
		`DELETE FROM audit_log`,
		`UPDATE audit_log_2026_08 SET action = 'rewritten'`,
		`DELETE FROM audit_log_2026_08`,
	} {
		t.Run(statement, func(t *testing.T) {
			err := uow.Within(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
				tx, err := postgres.FromContext(ctx)
				if err != nil {
					return err
				}
				_, err = tx.Exec(ctx, statement)
				return err
			})
			if err == nil {
				t.Errorf("%q succeeded - the audit trail is not append-only", statement)
			}
		})
	}
}

// The application role must not be able to rewrite the record of which migrations ran.
func TestTheApplicationRoleCannotTouchTheMigrationLedger(t *testing.T) {
	ctx := context.Background()
	pool := appPool(ctx, t)

	var count int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM goose_db_version`).Scan(&count)

	if err == nil {
		t.Error("the application role can read the migration ledger")
	}
}

// seedAuditRows puts one recognisable entry into each tenant's trail. Recognisable rather than
// only counted: other tests in this package write real entries for the same tenants, so the
// fixture asks for its own row by action instead of assuming it is the only one.
func seedAuditRows(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)

	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'tenant-a', 'A'), ($2, 'tenant-b', 'B')
		ON CONFLICT (id) DO NOTHING`, tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding tenants: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO audit_log (id, tenant_id, seq, occurred_at, action, outcome, actor_type, hash)
		SELECT gen_random_uuid(), t.id, 1, now(), 'seeded', 'SUCCESS', 'SYSTEM', '\x00'
		FROM (VALUES ($1::uuid), ($2::uuid)) AS t(id)
		WHERE NOT EXISTS (SELECT 1 FROM audit_log WHERE tenant_id = t.id AND action = 'seeded')`,
		tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding audit rows: %v", err)
	}
}

// errorsAs keeps the errors import out of every assertion above.
func errorsAs(err error, target **shared.Error) bool { return errors.As(err, target) }
