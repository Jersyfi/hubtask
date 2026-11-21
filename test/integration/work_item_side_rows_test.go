// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// sideRows counts the entry's rows in the three side tables of migration 0234, past every policy.
func sideRows(ctx context.Context, t *testing.T, item shared.ID) (announcement, flow, retention int) {
	t.Helper()
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT (SELECT count(*) FROM work_item_announcement WHERE item_id = $1),
		       (SELECT count(*) FROM work_item_flow WHERE item_id = $1),
		       (SELECT count(*) FROM work_item_retention WHERE item_id = $1)`, item.String()).
		Scan(&announcement, &flow, &retention); err != nil {
		t.Fatalf("reading the side rows: %v", err)
	}
	return announcement, flow, retention
}

// markSideRows gives the entry a row in each of the three side tables, past every policy.
func markSideRows(ctx context.Context, t *testing.T, tenant, item shared.ID) {
	t.Helper()
	admin := adminPool(ctx, t)
	for _, statement := range []string{
		`INSERT INTO work_item_announcement (tenant_id, item_id, overdue_announced_at) VALUES ($1, $2, now())`,
		`INSERT INTO work_item_flow (tenant_id, item_id, flow_finished_at) VALUES ($1, $2, now())
		 ON CONFLICT (tenant_id, item_id) DO UPDATE SET flow_finished_at = EXCLUDED.flow_finished_at`,
		`INSERT INTO work_item_retention (tenant_id, item_id, rule_id, action, blocked_by)
		 VALUES ($1, $2, gen_random_uuid(), 'DELETE', 'legal_hold')`,
	} {
		if _, err := admin.Exec(ctx, statement, tenant.String(), item.String()); err != nil {
			t.Fatalf("writing a side row: %v", err)
		}
	}
}

// The slim entry (F-10): the side rows go with their entry.
func TestTheSideRowsGoWithTheirEntry(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	item := seedWorkItemForRules(ctx, t, tenantA)
	markSideRows(ctx, t, tenantA, item)
	if a, f, r := sideRows(ctx, t, item); a != 1 || f != 1 || r != 1 {
		t.Fatalf("side rows %d, %d, %d, want one each", a, f, r)
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `DELETE FROM work_item WHERE id = $1`, item.String()); err != nil {
		t.Fatalf("deleting the entry: %v", err)
	}
	if a, f, r := sideRows(ctx, t, item); a+f+r != 0 {
		t.Errorf("side rows outlived their entry (%d, %d, %d)", a, f, r)
	}
}

// The cross-tenant negative test for the three side tables (gate SG-3): under another tenant's
// context the application role reads none of them.
func TestTheSideTablesAreNotReadableFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	item := seedWorkItemForRules(ctx, t, tenantA)
	markSideRows(ctx, t, tenantA, item)

	tx, err := appPool(ctx, t).Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenantB.String()); err != nil {
		t.Fatalf("setting the tenant context: %v", err)
	}
	var visible int
	if err := tx.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM work_item_announcement WHERE item_id = $1)
		     + (SELECT count(*) FROM work_item_flow WHERE item_id = $1)
		     + (SELECT count(*) FROM work_item_retention WHERE item_id = $1)`, item.String()).
		Scan(&visible); err != nil {
		t.Fatalf("reading as the other tenant: %v", err)
	}
	if visible != 0 {
		t.Errorf("%d side rows of tenant A are visible under tenant B", visible)
	}
}

// asTenant runs fn in a transaction of the application role under the tenant's context, as the
// transaction wrapper would (rule 3).
func asTenant(ctx context.Context, t *testing.T, tenant shared.ID, fn func(q *sqlc.Queries, tx pgx.Tx)) {
	t.Helper()
	tx, err := appPool(ctx, t).Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenant.String()); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("setting the tenant context: %v", err)
	}
	fn(sqlc.New(tx), tx)
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func uuidOf(id shared.ID) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(id.String())
	return u
}

// The retention walk, the entry's own marking and the entry read answer from the side table.
func TestTheReadersAnswerFromTheSideTables(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	item := seedWorkItemForRules(ctx, t, tenantA)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO work_item_retention (tenant_id, item_id, pending_until, rule_id, action)
		VALUES ($1, $2, now() - interval '2 hours', gen_random_uuid(), 'ARCHIVE')`,
		tenantA.String(), item.String()); err != nil {
		t.Fatalf("marking the entry: %v", err)
	}
	asTenant(ctx, t, tenantA, func(q *sqlc.Queries, _ pgx.Tx) {
		marking, err := q.FindItemRetention(ctx, uuidOf(item))
		if err != nil || marking.RetentionAction == nil || *marking.RetentionAction != "ARCHIVE" {
			t.Errorf("FindItemRetention answered %v (%v), want the side row's ARCHIVE", marking.RetentionAction, err)
		}
		entry, err := q.FindWorkItem(ctx, uuidOf(item))
		if err != nil || entry.RetentionAction == nil || *entry.RetentionAction != "ARCHIVE" {
			t.Errorf("FindWorkItem answered %v (%v), want the side row's ARCHIVE", entry.RetentionAction, err)
		}
		due, err := q.RetentionMarkedItemsDue(ctx, sqlc.RetentionMarkedItemsDueParams{
			Now: pgtype.Timestamptz{Time: time.Now(), Valid: true}, Batch: 1000,
		})
		if err != nil {
			t.Fatalf("RetentionMarkedItemsDue: %v", err)
		}
		found := false
		for _, row := range due {
			if row.ID == uuidOf(item) {
				found = row.RetentionAction != nil && *row.RetentionAction == "ARCHIVE"
			}
		}
		if !found {
			t.Error("the walk did not answer the entry with the side row's ARCHIVE")
		}
	})
}

// The slim entry's drop (0445): the eight columns, the generated search vector, the mirror and the
// triggers that kept the columns in step, and the indexes over the columns are gone, and the floor
// holds back every binary that still names them.
func TestTheSlimEntrysDropLeavesNothingThatNamesTheColumns(t *testing.T) {
	ctx := context.Background()
	var columns, triggers, functions, indexes int
	var floor int64
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT (SELECT count(*) FROM information_schema.columns
		         WHERE table_schema = 'public' AND table_name = 'work_item'
		           AND column_name IN ('due_soon_announced_at', 'overdue_announced_at', 'flow_started_at',
		             'flow_finished_at', 'retention_pending_until', 'retention_rule_id', 'retention_action',
		             'retention_blocked_by', 'search_vector')),
		       (SELECT count(*) FROM pg_trigger WHERE tgname IN ('work_item_side_rows', 'work_item_columns_follow')),
		       (SELECT count(*) FROM pg_proc
		         WHERE proname IN ('work_item_side_rows', 'work_item_mirror_side_rows',
		           'work_item_announcement_follow', 'work_item_retention_follow')),
		       (SELECT count(*) FROM pg_indexes
		         WHERE indexname IN ('wi_due_announce_idx', 'work_item_retention_idx', 'wi_search_idx')),
		       schema_compatibility_floor()`).Scan(&columns, &triggers, &functions, &indexes, &floor); err != nil {
		t.Fatalf("reading the catalogue: %v", err)
	}
	if columns+triggers+functions+indexes != 0 {
		t.Errorf("left behind: %d columns, %d triggers, %d functions, %d indexes", columns, triggers, functions, indexes)
	}
	if floor != 442 {
		t.Errorf("the compatibility floor is %d, want 442, the first migration of v0.10.1", floor)
	}
}
