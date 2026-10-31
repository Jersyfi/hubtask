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

// sideRows counts the entry's rows in the three side tables of migration 0227, past every policy.
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

// The slim entry's expand half (F-10): the trigger mirrors what background loops keep on an
// entry into its side tables in the writing transaction, keeps a row only while one of its values
// does, and the side rows follow the entry when it is deleted.
func TestTheSideTablesMirrorTheEntrysColumns(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	item := seedWorkItemForRules(ctx, t, tenantA)
	admin := adminPool(ctx, t)

	if a, f, r := sideRows(ctx, t, item); a+f+r != 0 {
		t.Fatalf("an entry carrying none of the values has side rows (%d, %d, %d)", a, f, r)
	}

	if _, err := admin.Exec(ctx, `
		UPDATE work_item SET due_soon_announced_at = now(), flow_started_at = now(),
		       retention_action = 'DELETE', retention_rule_id = gen_random_uuid()
		 WHERE id = $1`, item.String()); err != nil {
		t.Fatalf("marking the entry: %v", err)
	}
	if a, f, r := sideRows(ctx, t, item); a != 1 || f != 1 || r != 1 {
		t.Fatalf("side rows %d, %d, %d after the marks, want one each", a, f, r)
	}
	var action string
	if err := admin.QueryRow(ctx, `SELECT action FROM work_item_retention WHERE item_id = $1`,
		item.String()).Scan(&action); err != nil || action != "DELETE" {
		t.Errorf("the mirrored action is %q (%v), want DELETE", action, err)
	}

	// Cleared again: the rows go with their last value.
	if _, err := admin.Exec(ctx, `
		UPDATE work_item SET due_soon_announced_at = NULL, retention_action = NULL, retention_rule_id = NULL
		 WHERE id = $1`, item.String()); err != nil {
		t.Fatalf("clearing the marks: %v", err)
	}
	if a, f, r := sideRows(ctx, t, item); a != 0 || f != 1 || r != 0 {
		t.Errorf("side rows %d, %d, %d after clearing, want only the flow's", a, f, r)
	}

	if _, err := admin.Exec(ctx, `DELETE FROM work_item WHERE id = $1`, item.String()); err != nil {
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
	if _, err := adminPool(ctx, t).Exec(ctx, `
		UPDATE work_item SET overdue_announced_at = now(), flow_finished_at = now(),
		       retention_blocked_by = 'legal_hold'
		 WHERE id = $1`, item.String()); err != nil {
		t.Fatalf("marking the entry: %v", err)
	}

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

// disagreements counts the entries among ids whose side rows say something else than their
// columns, which is what the contract half's DROP may only follow once it is always zero.
func disagreements(ctx context.Context, t *testing.T, ids ...shared.ID) int {
	t.Helper()
	text := make([]string, len(ids))
	for i, id := range ids {
		text[i] = id.String()
	}
	var n int
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT count(*) FROM work_item w
		  LEFT JOIN work_item_announcement a ON a.tenant_id = w.tenant_id AND a.item_id = w.id
		  LEFT JOIN work_item_flow f ON f.tenant_id = w.tenant_id AND f.item_id = w.id
		  LEFT JOIN work_item_retention r ON r.tenant_id = w.tenant_id AND r.item_id = w.id
		 WHERE w.id = ANY($1::uuid[])
		   AND ((w.due_soon_announced_at, w.overdue_announced_at)
		          IS DISTINCT FROM (a.due_soon_announced_at, a.overdue_announced_at)
		     OR (w.flow_started_at, w.flow_finished_at) IS DISTINCT FROM (f.flow_started_at, f.flow_finished_at)
		     OR (w.retention_pending_until, w.retention_rule_id, w.retention_action, w.retention_blocked_by)
		          IS DISTINCT FROM (r.pending_until, r.rule_id, r.action, r.blocked_by))`, text).Scan(&n); err != nil {
		t.Fatalf("comparing the side rows: %v", err)
	}
	return n
}

// F-10's readers move before the columns go: the retention walk, the entry's own marking and the
// entry read answer from the side table. Proven by a side row that says something the columns do
// not - only a write past the trigger can make one.
func TestTheReadersAnswerFromTheSideTables(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	item := seedWorkItemForRules(ctx, t, tenantA)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		UPDATE work_item SET retention_pending_until = now() - interval '1 hour',
		       retention_rule_id = gen_random_uuid(), retention_action = 'DELETE'
		 WHERE id = $1`, item.String()); err != nil {
		t.Fatalf("marking the entry: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		UPDATE work_item_retention SET action = 'ARCHIVE', pending_until = now() - interval '2 hours'
		 WHERE item_id = $1`, item.String()); err != nil {
		t.Fatalf("changing the side row: %v", err)
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

// The mirror and the columns agree after every statement that writes the mirrored columns: the
// retention marking, its block, its clearing with and without the rule, a withdrawn rule's
// clearing, a new due date, an announcement and a flow moment.
func TestTheMirrorAgreesAfterEveryWritePath(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	marked, blocked := seedWorkItemForRules(ctx, t, tenantA), seedWorkItemForRules(ctx, t, tenantA)
	rule := freshID(t)
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	action, hold := "DELETE", "legal_hold"
	check := func(step string) {
		t.Helper()
		if n := disagreements(ctx, t, marked, blocked); n != 0 {
			t.Fatalf("after %s, %d entries disagree with their side rows", step, n)
		}
	}
	asTenant(ctx, t, tenantA, func(q *sqlc.Queries, _ pgx.Tx) {
		if _, err := q.MarkItemsForRetention(ctx, sqlc.MarkItemsForRetentionParams{
			EffectiveAt: now, RuleID: uuidOf(rule), Action: &action, Ids: []pgtype.UUID{uuidOf(marked)},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.BlockItemsForRetention(ctx, sqlc.BlockItemsForRetentionParams{
			RuleID: uuidOf(rule), Action: &action, BlockedBy: &hold, Ids: []pgtype.UUID{uuidOf(blocked)},
		}); err != nil {
			t.Fatal(err)
		}
	})
	check("marking and blocking")
	asTenant(ctx, t, tenantA, func(q *sqlc.Queries, _ pgx.Tx) {
		if _, err := q.ClearItemRetention(ctx, sqlc.ClearItemRetentionParams{KeepRule: true, Now: now, Ids: []pgtype.UUID{uuidOf(marked)}}); err != nil {
			t.Fatal(err)
		}
	})
	check("clearing a stage that acted")
	asTenant(ctx, t, tenantA, func(q *sqlc.Queries, _ pgx.Tx) {
		if _, err := q.ClearRetentionMarksOfRule(ctx, sqlc.ClearRetentionMarksOfRuleParams{Now: now, RuleID: uuidOf(rule)}); err != nil {
			t.Fatal(err)
		}
	})
	check("withdrawing the rule")
	if _, err := adminPool(ctx, t).Exec(ctx, `
		UPDATE work_item SET due_at = now(), due_soon_announced_at = now(), overdue_announced_at = now(),
		       flow_started_at = now() - interval '1 day', flow_finished_at = now()
		 WHERE id = $1`, marked.String()); err != nil {
		t.Fatalf("announcing and timing the entry: %v", err)
	}
	check("an announcement and a flow moment")
	asTenant(ctx, t, tenantA, func(q *sqlc.Queries, tx pgx.Tx) {
		var version int32
		if err := tx.QueryRow(ctx, `SELECT version FROM work_item WHERE id = $1`, marked.String()).Scan(&version); err != nil {
			t.Fatal(err)
		}
		if n, err := q.SetWorkItemDueDate(ctx, sqlc.SetWorkItemDueDateParams{
			DueAt: pgtype.Timestamptz{Time: time.Now().Add(48 * time.Hour), Valid: true}, UpdatedAt: now,
			ID: uuidOf(marked), ExpectedVersion: version,
		}); err != nil || n != 1 {
			t.Fatalf("setting the due date: %d, %v", n, err)
		}
	})
	check("a new due date")
	if a, _, _ := sideRows(ctx, t, marked); a != 0 {
		t.Errorf("a new due date left %d announcement rows", a)
	}
}
