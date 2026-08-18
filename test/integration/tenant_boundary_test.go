// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// The gate of ADR-0024: a reference between two tenant tables carries the tenant.
//
// Row level security does not reach a foreign key - PostgreSQL checks referential integrity in
// triggers that run as the table owner - so a single-column key lets a row in one tenant reference,
// and a cascade delete, a row in another. The list comes from the catalogue rather than from a
// constant here: a table added later with a single-column reference has to turn this red on its own.
func TestEveryReferenceBetweenTenantTablesCarriesTheTenant(t *testing.T) {
	ctx := context.Background()

	findings, checked := unscopedReferences(ctx, t)
	for _, finding := range findings {
		t.Error(finding)
	}
	if checked < 25 {
		t.Errorf("only %d references checked - the catalogue query is not seeing the schema", checked)
	}
}

// And the other half of a gate: proof that it fails when the rule is broken. A gate nobody has seen
// go red is a gate nobody knows is connected (the reasoning behind `make gate-selftest`, which
// covers the Go rules the same way).
func TestTheReferenceGateCatchesASingleColumnKey(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)

	// A table shaped like a real one: a NOT NULL tenant, and a reference that forgets it.
	for _, statement := range []string{
		`CREATE TABLE selftest_reference (
		   id uuid PRIMARY KEY,
		   tenant_id uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
		   collection_id uuid NOT NULL REFERENCES container(id) ON DELETE CASCADE)`,
	} {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("building the deliberate violation: %v", err)
		}
	}
	defer func() {
		if _, err := admin.Exec(ctx, `DROP TABLE IF EXISTS selftest_reference`); err != nil {
			t.Fatalf("removing the deliberate violation: %v", err)
		}
	}()

	findings, _ := unscopedReferences(ctx, t)

	var caught bool
	for _, finding := range findings {
		if strings.Contains(finding, "selftest_reference") {
			caught = true
		}
	}
	if !caught {
		t.Errorf("the gate did not report a single-column reference it was shown: %v", findings)
	}
	if len(findings) != 1 {
		t.Errorf("the gate reported %d findings, want only the deliberate one: %v",
			len(findings), findings)
	}
}

// unscopedReferences returns one finding per foreign key between two tables whose `tenant_id` is
// NOT NULL that does not carry the tenant, and how many references it judged.
func unscopedReferences(ctx context.Context, t *testing.T) (findings []string, checked int) {
	t.Helper()

	rows, err := adminPool(ctx, t).Query(ctx, `
		WITH strict_tenant AS (
		  SELECT c.oid, c.relname
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		  JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant_id'
		       AND a.attnum > 0 AND a.attnotnull
		  WHERE n.nspname = 'public' AND c.relkind IN ('r','p')
		)
		SELECT src.relname, tgt.relname, con.conname, array_length(con.conkey, 1),
		       (SELECT a.attname FROM pg_attribute a
		        WHERE a.attrelid = con.conrelid AND a.attnum = con.conkey[1])
		FROM pg_constraint con
		JOIN strict_tenant src ON src.oid = con.conrelid
		JOIN strict_tenant tgt ON tgt.oid = con.confrelid
		WHERE con.contype = 'f'
		ORDER BY src.relname, con.conname`)
	if err != nil {
		t.Fatalf("catalogue query: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var src, tgt, name, firstColumn string
		var columns int
		if err := rows.Scan(&src, &tgt, &name, &columns, &firstColumn); err != nil {
			t.Fatalf("scan: %v", err)
		}
		checked++

		switch {
		case columns < 2:
			findings = append(findings, fmt.Sprintf(
				"%s references %s on one column (%s): a row in one tenant could reference, and a "+
					"cascade could delete, a row in another (ADR-0024)", name, tgt, firstColumn))
		case firstColumn != "tenant_id":
			// The tenant has to be the first column, or the key scopes something else.
			findings = append(findings, fmt.Sprintf(
				"%s starts at %s rather than tenant_id", name, firstColumn))
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return findings, checked
}

// The trap that makes the gate above insufficient on its own: a composite key whose delete rule
// nulls every column of itself would null `tenant_id`, which is NOT NULL. PostgreSQL accepts that
// form when it is declared and fails only when it fires, so nothing but a check like this one finds
// it before a delete does (ADR-0024).
func TestNoDeleteRuleWouldNullTheTenant(t *testing.T) {
	ctx := context.Background()

	findings, checked := tenantNullingDeleteRules(ctx, t)
	for _, finding := range findings {
		t.Error(finding)
	}
	if checked == 0 {
		t.Error("no composite SET NULL reference found at all - the query is not seeing the schema")
	}
}

// The same second half this gate's neighbour has: proof that it fails when the rule is broken.
// Without it the check above is a query nobody has watched answer "yes" - and this is the one trap
// PostgreSQL accepts at declaration time, so a gate that is quietly disconnected would be found by
// a delete in production rather than here (ADR-0024).
func TestTheDeleteRuleGateCatchesAMissingColumnList(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)

	// Composite and tenant-first, so the neighbouring gate has nothing to say about it - what is
	// wrong is only the delete rule, which nulls the whole key including the tenant.
	if _, err := admin.Exec(ctx, `
		CREATE TABLE selftest_delete_rule (
		  id uuid PRIMARY KEY,
		  tenant_id uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
		  collection_id uuid,
		  FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id)
		    ON DELETE SET NULL)`); err != nil {
		t.Fatalf("building the deliberate violation: %v", err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, `DROP TABLE IF EXISTS selftest_delete_rule`); err != nil {
			t.Fatalf("removing the deliberate violation: %v", err)
		}
	}()

	findings, _ := tenantNullingDeleteRules(ctx, t)

	var caught bool
	for _, finding := range findings {
		if strings.Contains(finding, "selftest_delete_rule") {
			caught = true
		}
	}
	if !caught {
		t.Errorf("the gate did not report a delete rule it was shown: %v", findings)
	}
	if len(findings) != 1 {
		t.Errorf("the gate reported %d findings, want only the deliberate one: %v",
			len(findings), findings)
	}
}

// tenantNullingDeleteRules returns one finding per composite reference whose delete rule would null
// the tenant, and how many delete rules it judged.
func tenantNullingDeleteRules(ctx context.Context, t *testing.T) (findings []string, checked int) {
	t.Helper()

	rows, err := adminPool(ctx, t).Query(ctx, `
		SELECT con.conname, src.relname,
		       (SELECT count(*) FROM unnest(con.confdelsetcols) AS c) AS set_columns
		FROM pg_constraint con
		JOIN pg_class src ON src.oid = con.conrelid
		JOIN pg_namespace n ON n.oid = src.relnamespace
		JOIN pg_attribute a ON a.attrelid = src.oid AND a.attname = 'tenant_id'
		     AND a.attnum > 0 AND a.attnotnull
		WHERE n.nspname = 'public' AND con.contype = 'f'
		  AND con.confdeltype = 'n' AND array_length(con.conkey, 1) > 1
		ORDER BY con.conname`)
	if err != nil {
		t.Fatalf("catalogue query: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name, table string
		var setColumns int
		if err := rows.Scan(&name, &table, &setColumns); err != nil {
			t.Fatalf("scan: %v", err)
		}
		checked++
		// No column list means "null the whole key", and the whole key includes the tenant.
		if setColumns == 0 {
			findings = append(findings, fmt.Sprintf(
				"%s on %s is ON DELETE SET NULL without a column list: the delete would null "+
					"tenant_id and fail at run time (ADR-0024)", name, table))
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return findings, checked
}

// The tables whose tenant is nullable are outside the rule, and that has to stay a decision rather
// than an oversight. `NULL` means installation-wide there, so a composite key would both forbid a
// tenant using an installation-wide row and - under MATCH SIMPLE, which skips the check when any
// key column is NULL - switch the check off for the rows that keep a NULL tenant (ADR-0024).
//
// A new table with a nullable tenant turns this red, so somebody has to say which of the two it is.
func TestTheTablesOutsideTheRuleAreTheDocumentedOnes(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)

	expected := map[string]string{
		"job":                     "system jobs are partly tenant-less; restricted by privileges",
		"item_capability_profile": "NULL is a system default every tenant may read and none may write",
		"privacy_incident":        "an incident may span the installation rather than one tenant",
		"backup_target":           "NULL is an installation-wide target (ADR-0019)",
		"backup_schedule":         "schedules an installation-wide target",
		"backup_run":              "records a run of an installation-wide target",
		"restore_run":             "restores from an installation-wide target",
	}

	rows, err := admin.Query(ctx, `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant_id' AND a.attnum > 0
		WHERE n.nspname = 'public' AND c.relkind IN ('r','p') AND NOT a.attnotnull
		ORDER BY c.relname`)
	if err != nil {
		t.Fatalf("catalogue query: %v", err)
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen[name] = true
		if _, documented := expected[name]; !documented {
			t.Errorf("%s has a nullable tenant_id and is not in the documented list: decide whether "+
				"the column should be NOT NULL and the references composite, or add it here with a "+
				"reason (ADR-0024)", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	for name, reason := range expected {
		if !seen[name] {
			t.Errorf("%s is listed as having a nullable tenant_id (%s) and no longer has one - "+
				"its references belong under the rule now", name, reason)
		}
	}
}
