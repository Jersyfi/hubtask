// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// check is one question asked of the restored instance, with its answer. The detail names
// catalogue objects and counts, never content - it travels into evidence and, on failure, into a
// log line.
type check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// checkTimeout bounds one check's queries; a check that hangs is a finding of its own.
const checkTimeout = 2 * time.Minute

// runChecks is T-20's drill half against the instance that came back (security.md §4), plus the
// consistency questions a point-in-time recovery has to answer. Every check runs whatever the
// earlier ones found: the evidence is a list, not a first failure.
func runChecks(ctx context.Context, owner, app *pgx.Conn, runID string, liveSchemaVersion int64) []check {
	return []check{
		markersRecovered(ctx, owner, runID),
		notInRecovery(ctx, owner),
		schemaVersionMatches(ctx, owner, liveSchemaVersion),
		indexesValid(ctx, owner),
		constraintsValidated(ctx, owner),
		rowLevelSecurityForced(ctx, owner),
		applicationRoleBounded(ctx, owner),
		tenantContextRequired(ctx, app),
		tenantSeesOnlyItself(ctx, owner, app),
	}
}

func failed(name string, err error) check {
	return check{Name: name, OK: false, Detail: "the check could not run: " + err.Error()}
}

// markersRecovered is the point of the whole exercise: the first marker survived the recovery,
// the second did not, so the target fell between two commits exactly as asked.
func markersRecovered(ctx context.Context, owner *pgx.Conn, runID string) check {
	const name = "recovery_target_between_the_markers"
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	var first, second int
	if err := owner.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE seq = 1), count(*) FILTER (WHERE seq = 2)
		FROM restore_drill_marker WHERE run_id = $1`, runID).Scan(&first, &second); err != nil {
		return failed(name, err)
	}
	return check{
		Name:   name,
		OK:     first == 1 && second == 0,
		Detail: fmt.Sprintf("first marker present: %d, second marker present: %d", first, second),
	}
}

// notInRecovery: the temporary cluster was promoted after reaching the target, so it is a
// primary that could take traffic - which is what an RTO is measured to.
func notInRecovery(ctx context.Context, owner *pgx.Conn) check {
	const name = "restored_instance_promoted"
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	var inRecovery bool
	if err := owner.QueryRow(ctx, `SELECT pg_is_in_recovery()`).Scan(&inRecovery); err != nil {
		return failed(name, err)
	}
	return check{Name: name, OK: !inRecovery, Detail: fmt.Sprintf("pg_is_in_recovery: %t", inRecovery)}
}

// schemaVersion is the migration ledger's head.
func schemaVersion(ctx context.Context, conn *pgx.Conn) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	var version int64
	err := conn.QueryRow(ctx, `SELECT coalesce(max(version_id), 0) FROM goose_db_version WHERE is_applied`).Scan(&version)
	return version, err
}

func schemaVersionMatches(ctx context.Context, owner *pgx.Conn, live int64) check {
	const name = "schema_version_matches_live"
	restored, err := schemaVersion(ctx, owner)
	if err != nil {
		return failed(name, err)
	}
	return check{
		Name:   name,
		OK:     restored == live,
		Detail: fmt.Sprintf("restored %d, live %d", restored, live),
	}
}

// indexesValid: an index left invalid by an interrupted build is one the planner ignores, and a
// restore is exactly where such a thing surfaces.
func indexesValid(ctx context.Context, owner *pgx.Conn) check {
	const name = "indexes_valid"
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	var invalid int
	if err := owner.QueryRow(ctx, `SELECT count(*) FROM pg_index WHERE NOT indisvalid`).Scan(&invalid); err != nil {
		return failed(name, err)
	}
	return check{Name: name, OK: invalid == 0, Detail: fmt.Sprintf("invalid indexes: %d", invalid)}
}

// constraintsValidated: a foreign key or check added NOT VALID and never validated is a promise
// the data may already break.
func constraintsValidated(ctx context.Context, owner *pgx.Conn) check {
	const name = "constraints_validated"
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	var pending int
	if err := owner.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint WHERE contype IN ('f', 'c') AND NOT convalidated`).Scan(&pending); err != nil {
		return failed(name, err)
	}
	return check{Name: name, OK: pending == 0, Detail: fmt.Sprintf("unvalidated constraints: %d", pending)}
}

// rowLevelSecurityForced is T-20's structural half: every table carrying a tenant column has row
// level security enabled and forced, so that not even its owner reads across tenants. Judged by
// the column rather than by a list of names, so that a table added since needs no entry here.
func rowLevelSecurityForced(ctx context.Context, owner *pgx.Conn) check {
	const name = "row_level_security_forced_on_tenant_tables"
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	rows, err := owner.Query(ctx, `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant_id' AND NOT a.attisdropped
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND NOT c.relispartition
		  AND NOT (c.relrowsecurity AND c.relforcerowsecurity)
		ORDER BY c.relname`)
	if err != nil {
		return failed(name, err)
	}
	defer rows.Close()
	var unprotected []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return failed(name, err)
		}
		unprotected = append(unprotected, table)
	}
	if err := rows.Err(); err != nil {
		return failed(name, err)
	}
	var tables int
	if err := owner.QueryRow(ctx, `
		SELECT count(*) FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant_id' AND NOT a.attisdropped
		WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND NOT c.relispartition`).Scan(&tables); err != nil {
		return failed(name, err)
	}
	if len(unprotected) > 0 {
		return check{Name: name, OK: false, Detail: "without forced row level security: " + strings.Join(unprotected, ", ")}
	}
	return check{Name: name, OK: tables > 0, Detail: fmt.Sprintf("tenant tables checked: %d", tables)}
}

// applicationRoleBounded: the role the application connects as came back as narrow as
// multi-tenancy.md §2.1 demands - restored with the data, so a restore cannot widen it.
func applicationRoleBounded(ctx context.Context, owner *pgx.Conn) check {
	const name = "application_role_bounded"
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	var super, bypass bool
	if err := owner.QueryRow(ctx, `
		SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = 'hubtask_app'`).Scan(&super, &bypass); err != nil {
		return failed(name, err)
	}
	return check{
		Name:   name,
		OK:     !super && !bypass,
		Detail: fmt.Sprintf("superuser: %t, bypassrls: %t", super, bypass),
	}
}

// tenantContextRequired: without a tenant set, the application role sees nothing at all. The
// policies compare against current_tenant_id(), which is NULL when nothing set it.
func tenantContextRequired(ctx context.Context, app *pgx.Conn) check {
	const name = "no_tenant_context_sees_nothing"
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	var tenants, items int
	if err := app.QueryRow(ctx, `SELECT (SELECT count(*) FROM tenant), (SELECT count(*) FROM work_item)`).Scan(&tenants, &items); err != nil {
		return failed(name, err)
	}
	return check{
		Name:   name,
		OK:     tenants == 0 && items == 0,
		Detail: fmt.Sprintf("visible without a tenant: tenants %d, items %d", tenants, items),
	}
}

// tenantSeesOnlyItself is T-20's behavioural half: under a tenant, the application role sees
// that tenant's rows - all of them and nobody else's. The owner names the tenants and counts
// what each one holds; the application role is then asked the same question under each.
func tenantSeesOnlyItself(ctx context.Context, owner, app *pgx.Conn) check {
	const name = "tenant_sees_only_itself"
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	rows, err := owner.Query(ctx, `
		SELECT t.id::text, (SELECT count(*) FROM work_item w WHERE w.tenant_id = t.id)
		FROM tenant t ORDER BY t.created_at LIMIT 2`)
	if err != nil {
		return failed(name, err)
	}
	type tenant struct {
		id    string
		items int64
	}
	var tenants []tenant
	for rows.Next() {
		var t tenant
		if err := rows.Scan(&t.id, &t.items); err != nil {
			rows.Close()
			return failed(name, err)
		}
		tenants = append(tenants, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return failed(name, err)
	}
	if len(tenants) == 0 {
		return check{Name: name, OK: true, Detail: "no tenant rows to judge by"}
	}

	for _, t := range tenants {
		visible, foreign, self, err := underTenant(ctx, app, t.id)
		if err != nil {
			return failed(name, err)
		}
		if foreign != 0 || self != 1 || visible != t.items {
			return check{
				Name: name,
				OK:   false,
				Detail: fmt.Sprintf("under one tenant: own items visible %d of %d, foreign rows %d, tenant rows %d",
					visible, t.items, foreign, self),
			}
		}
	}
	return check{Name: name, OK: true, Detail: fmt.Sprintf("tenants judged: %d", len(tenants))}
}

// underTenant asks the application role three counts inside one transaction with the tenant set
// the way the transaction wrapper sets it (ADR-0010): rows it can see, rows of other tenants
// among them, and tenant rows.
func underTenant(ctx context.Context, app *pgx.Conn, tenantID string) (visible, foreign, self int64, err error) {
	tx, err := app.Begin(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenantID); err != nil {
		return 0, 0, 0, err
	}
	err = tx.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM work_item),
		       (SELECT count(*) FROM work_item WHERE tenant_id <> $1::uuid),
		       (SELECT count(*) FROM tenant)`, tenantID).Scan(&visible, &foreign, &self)
	return visible, foreign, self, err
}
