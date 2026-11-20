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

	lifecycleRepository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
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
// not - only a write past the triggers, both ways, can make one.
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
	pastTheTriggers, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pastTheTriggers.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatal(err)
	}
	if _, err := pastTheTriggers.Exec(ctx, `
		UPDATE work_item_retention SET action = 'ARCHIVE', pending_until = now() - interval '2 hours'
		 WHERE item_id = $1`, item.String()); err != nil {
		_ = pastTheTriggers.Rollback(ctx)
		t.Fatalf("changing the side row: %v", err)
	}
	if err := pastTheTriggers.Commit(ctx); err != nil {
		t.Fatal(err)
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

// The older binary's statements, as v0.10.0 ran them on the columns: what the mirror stays for
// while that binary rolls out (versioning-release.md §4).
const (
	olderMark = `UPDATE work_item SET retention_pending_until = $1::timestamptz, retention_rule_id = $2,
		retention_action = $3, retention_blocked_by = NULL
		WHERE id = ANY($4::uuid[]) AND retention_pending_until IS NULL`
	olderBlock = `UPDATE work_item SET retention_rule_id = $1, retention_action = $2, retention_blocked_by = $3
		WHERE id = ANY($4::uuid[]) AND retention_pending_until IS NULL
		  AND (retention_rule_id IS DISTINCT FROM $1 OR retention_action IS DISTINCT FROM $2
		    OR retention_blocked_by IS DISTINCT FROM $3)`
	olderClear = `UPDATE work_item SET retention_pending_until = NULL, retention_action = NULL,
		retention_blocked_by = NULL,
		retention_rule_id = CASE WHEN $1::boolean THEN retention_rule_id END,
		updated_at = $2::timestamptz, version = version + 1
		WHERE id = ANY($3::uuid[]) AND retention_pending_until IS NOT NULL`
	olderClearRule = `UPDATE work_item SET retention_pending_until = NULL, retention_action = NULL,
		retention_blocked_by = NULL, retention_rule_id = NULL, updated_at = $1::timestamptz, version = version + 1
		WHERE retention_rule_id = $2`
	olderDueDate = `UPDATE work_item SET due_at = $1, due_soon_announced_at = NULL, overdue_announced_at = NULL,
		updated_at = $2, version = version + 1 WHERE id = $3::uuid`
	olderClaimDueSoon = `UPDATE work_item SET due_soon_announced_at = $1
		WHERE id IN (SELECT due.id FROM work_item AS due
		  WHERE due.due_at IS NOT NULL AND due.due_at <= $2 AND due.due_soon_announced_at IS NULL
		    AND due.deleted_at IS NULL AND due.archived_at IS NULL AND due.is_completed = false
		  ORDER BY due.due_at FOR UPDATE SKIP LOCKED LIMIT 100)
		RETURNING id`
	olderClaimOverdue = `UPDATE work_item SET overdue_announced_at = $1
		WHERE id IN (SELECT due.id FROM work_item AS due
		  WHERE due.due_at IS NOT NULL AND due.due_at <= $1 AND due.overdue_announced_at IS NULL
		    AND due.deleted_at IS NULL AND due.archived_at IS NULL AND due.is_completed = false
		  ORDER BY due.due_at FOR UPDATE SKIP LOCKED LIMIT 100)
		RETURNING id`
)

// The mirror and the columns agree after every statement the older binary writes the mirrored
// columns with: the retention marking, its block, its clearing with and without the rule, a
// withdrawn rule's clearing, a new due date, an announcement and a flow moment.
func TestTheMirrorAgreesAfterEveryWritePath(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	marked, blocked := seedWorkItemForRules(ctx, t, tenantA), seedWorkItemForRules(ctx, t, tenantA)
	rule := freshID(t).String()
	now := time.Now()
	ids := func(items ...shared.ID) []string {
		out := make([]string, len(items))
		for i, item := range items {
			out[i] = item.String()
		}
		return out
	}
	check := func(step string) {
		t.Helper()
		if n := disagreements(ctx, t, marked, blocked); n != 0 {
			t.Fatalf("after %s, %d entries disagree with their side rows", step, n)
		}
	}
	older := func(step, statement string, args ...any) {
		t.Helper()
		asTenant(ctx, t, tenantA, func(_ *sqlc.Queries, tx pgx.Tx) {
			if _, err := tx.Exec(ctx, statement, args...); err != nil {
				t.Fatalf("%s: %v", step, err)
			}
		})
		check(step)
	}
	older("marking", olderMark, now, rule, "ARCHIVE", ids(marked))
	older("blocking", olderBlock, rule, "ARCHIVE", "legal_hold", ids(blocked))
	older("clearing a stage that acted", olderClear, true, now, ids(marked))
	older("withdrawing the rule", olderClearRule, now, rule)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		UPDATE work_item SET due_at = now(), due_soon_announced_at = now(), overdue_announced_at = now(),
		       flow_started_at = now() - interval '1 day', flow_finished_at = now()
		 WHERE id = $1`, marked.String()); err != nil {
		t.Fatalf("announcing and timing the entry: %v", err)
	}
	check("an announcement and a flow moment")
	older("a new due date", olderDueDate, now.Add(48*time.Hour), now, marked.String())
	if a, _, _ := sideRows(ctx, t, marked); a != 0 {
		t.Errorf("a new due date left %d announcement rows", a)
	}
}

// What this release writes to a side row is not undone by a write of the columns, which this
// release leaves stale: the mirror carries only the columns a statement changed. A card moved
// into its first column records its flow moment without touching the announcement claimed on the
// side row, and the older binary's block keeps the marking's moment it did not write.
func TestAColumnWriteLeavesWhatItDidNotChange(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	item := seedWorkItemForRules(ctx, t, tenantA)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO work_item_announcement (tenant_id, item_id, due_soon_announced_at)
		VALUES ($1, $2, now() - interval '1 hour')`, tenantA.String(), item.String()); err != nil {
		t.Fatalf("claiming on the side row: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO work_item_retention (tenant_id, item_id, pending_until, rule_id, action)
		VALUES ($1, $2, now() + interval '1 day', gen_random_uuid(), 'ARCHIVE')`,
		tenantA.String(), item.String()); err != nil {
		t.Fatalf("marking on the side row: %v", err)
	}

	if _, err := admin.Exec(ctx, `
		UPDATE work_item SET flow_started_at = now(), retention_blocked_by = 'legal_hold' WHERE id = $1`,
		item.String()); err != nil {
		t.Fatalf("writing two columns: %v", err)
	}
	var soon, pending *time.Time
	var blockedBy *string
	if err := admin.QueryRow(ctx, `
		SELECT a.due_soon_announced_at, r.pending_until, r.blocked_by
		  FROM work_item_announcement a JOIN work_item_retention r USING (tenant_id, item_id)
		 WHERE a.item_id = $1`, item.String()).Scan(&soon, &pending, &blockedBy); err != nil {
		t.Fatalf("reading the side rows: %v", err)
	}
	if soon == nil {
		t.Error("a flow moment undid the announcement claimed on the side row")
	}
	if pending == nil || blockedBy == nil || *blockedBy != "legal_hold" {
		t.Errorf("the retention row is %v / %v, want the side row's moment and the column's block", pending, blockedBy)
	}
	if _, f, _ := sideRows(ctx, t, item); f != 1 {
		t.Error("the flow moment was not carried")
	}
}

// The roll-out window, closed: what this release writes to a side row, v0.10.0 reads from the
// entry's columns, which follow the side rows (work_item_columns_follow). Its clearings find a
// mark this release set, so phase two no longer sees it; its claims do not announce again what
// this release announced; its new due date removes the announcement, so the next claim announces
// the moved deadline.
func TestTheOlderBinarySeesWhatTheSideRowsSay(t *testing.T) {
	ctx := context.Background()
	tenant, author := seedOwnTenant(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenant, author)
	now := time.Now().UTC()
	older := func(step, statement string, args ...any) int64 {
		t.Helper()
		var n int64
		asTenant(ctx, t, tenant, func(_ *sqlc.Queries, tx pgx.Tx) {
			tag, err := tx.Exec(ctx, statement, args...)
			if err != nil {
				t.Fatalf("%s: %v", step, err)
			}
			n = tag.RowsAffected()
		})
		return n
	}
	due := func() int {
		t.Helper()
		var n int
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			candidates, err := markingRepo().MarkedDue(ctx, now.Add(time.Hour), lifecycleRepository.Position{}, 100)
			n = len(candidates)
			return err
		}); err != nil {
			t.Fatalf("reading what is due: %v", err)
		}
		return n
	}

	// A person's :retain and a withdrawn rule, served by v0.10.0, on marks this release set.
	retained, withdrawn := seedTask(ctx, t, tenant, author, collection), seedTask(ctx, t, tenant, author, collection)
	rule := freshID(t)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		if _, err := markingRepo().Mark(ctx, []shared.ID{retained}, rule, "ARCHIVE", now); err != nil {
			return err
		}
		_, err := markingRepo().Mark(ctx, []shared.ID{withdrawn}, rule, "ARCHIVE", now)
		return err
	}); err != nil {
		t.Fatalf("marking: %v", err)
	}
	if n := due(); n != 2 {
		t.Fatalf("%d entries are due after marking two", n)
	}
	if n := older(":retain", olderClear, false, now, []string{retained.String()}); n != 1 {
		t.Errorf("v0.10.0's :retain cleared %d entries, want the one this release marked", n)
	}
	if n := older("withdrawing the rule", olderClearRule, now, rule.String()); n != 1 {
		t.Errorf("v0.10.0's withdrawal cleared %d entries, want the one still marked", n)
	}
	if n := due(); n != 0 {
		t.Errorf("phase two still sees %d marks v0.10.0 cleared", n)
	}
	for _, id := range []shared.ID{retained, withdrawn} {
		if _, _, r := sideRows(ctx, t, id); r != 0 {
			t.Errorf("v0.10.0's clearing left the side mark of %s", id)
		}
	}

	// v0.10.0's mark and block land on the side row as before.
	if n := older("marking", olderMark, now, rule.String(), "ARCHIVE", []string{retained.String()}); n != 1 {
		t.Errorf("v0.10.0 marked %d entries", n)
	}
	if n := older("blocking a marked entry", olderBlock, rule.String(), "ARCHIVE", "legal_hold",
		[]string{retained.String()}); n != 0 {
		t.Errorf("v0.10.0 blocked %d marked entries", n)
	}
	if n := due(); n != 1 {
		t.Errorf("%d entries are due after v0.10.0 marked one", n)
	}

	// Announcements: what this release claimed, v0.10.0 does not claim again; a date it moves is
	// announced again by this release.
	deadline := dueTask(ctx, t, tenant, author, collection, now.Add(-time.Hour))
	if got := claimOverdue(ctx, t, tenant, now); len(got) != 1 {
		t.Fatalf("this release claimed %v", got)
	}
	if got := claimDueSoon(ctx, t, tenant, now); len(got) != 1 {
		t.Fatalf("this release claimed %v as due soon", got)
	}
	if n := older("claiming overdue", olderClaimOverdue, now); n != 0 {
		t.Errorf("v0.10.0 announced %d deadlines this release announced", n)
	}
	if n := older("claiming due soon", olderClaimDueSoon, now, now.Add(24*time.Hour)); n != 0 {
		t.Errorf("v0.10.0 announced %d approaching deadlines this release announced", n)
	}
	if a, _, _ := sideRows(ctx, t, deadline); a != 1 {
		t.Errorf("the entry has %d announcement rows", a)
	}
	if n := older("moving the date", olderDueDate, now.Add(-time.Minute), now, deadline.String()); n != 1 {
		t.Fatalf("v0.10.0 moved %d dates", n)
	}
	if a, _, _ := sideRows(ctx, t, deadline); a != 0 {
		t.Errorf("v0.10.0's new date left the announcement")
	}
	if got := claimOverdue(ctx, t, tenant, now); len(got) != 1 || got[0].ItemID != deadline {
		t.Errorf("the moved deadline was not announced again: %v", got)
	}
	// And a v0.10.0 claim of a stamp the side row lacks lands on the existing row.
	if n := older("claiming due soon after the move", olderClaimDueSoon, now, now.Add(24*time.Hour)); n != 1 {
		t.Errorf("v0.10.0 claimed %d", n)
	}
	if a, _, _ := sideRows(ctx, t, deadline); a != 1 || columnsLag(ctx, t, deadline) != 0 {
		t.Errorf("after v0.10.0's claim: %d announcement rows, columns lag %d", a, columnsLag(ctx, t, deadline))
	}
}
