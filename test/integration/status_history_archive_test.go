// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The status history travels in the archive (migration 0374, tenant-export.md §6, UC-GOL-04
// check 11): a restored entry keeps when it entered each status, so its cycle time is the one it
// had before the restore - not one counted from the restore's moment.

var (
	historyStarted   = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	historyCompleted = time.Date(2026, 9, 4, 17, 30, 0, 0, time.UTC)
)

// cycleMinutes is the cycle time as the summaries read it (query/Aggregate.go flowQuantity): the
// first entry into the started status to completion, in whole minutes.
func cycleMinutes(ctx context.Context, t *testing.T, tenant shared.ID) int64 {
	t.Helper()
	var minutes int64
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT div(extract(epoch FROM wi.completed_at - (
		         SELECT min(sc.entered_at) FROM entry_status_change sc
		         WHERE sc.tenant_id = wi.tenant_id AND sc.item_id = wi.id AND sc.status = 'in_progress')), 60)::bigint
		FROM work_item wi WHERE wi.tenant_id = $1`, tenant.String()).Scan(&minutes); err != nil {
		t.Fatalf("reading the cycle time: %v", err)
	}
	return minutes
}

func TestARestoredEntryKeepsItsStatusHistoryAndItsCycleTime(t *testing.T) {
	ctx := context.Background()
	f := newForeignRestore(ctx, t)
	completed := func(tenant, item shared.ID) (string, []any) {
		return `UPDATE work_item SET status = 'done', is_completed = true, completed_at = $3
			WHERE tenant_id = $1 AND id = $2`, []any{tenant.String(), item.String(), historyCompleted}
	}
	// The trigger wrote the moment of the seeding; the history the entry really had replaces it.
	rewritten := func(tenant, item shared.ID) (string, []any) {
		return `DELETE FROM entry_status_change WHERE tenant_id = $1 AND item_id = $2`,
			[]any{tenant.String(), item.String()}
	}
	history := func(tenant, item shared.ID) (string, []any) {
		return `INSERT INTO entry_status_change (tenant_id, item_id, status, entered_at)
			VALUES ($1, $2, 'todo', $3), ($1, $2, 'in_progress', $4), ($1, $2, 'done', $5)`,
			[]any{tenant.String(), item.String(), historyStarted.Add(-24 * time.Hour), historyStarted, historyCompleted}
	}
	source, prefix := foreignWorkspace(ctx, t, f.store, "Ship the order", completed, rewritten, history)
	before := cycleMinutes(ctx, t, source)
	if want := int64(historyCompleted.Sub(historyStarted) / time.Minute); before != want {
		t.Fatalf("the seeded cycle time is %d minutes, want %d", before, want)
	}
	deleteForGood(ctx, t, source)

	restoreID, minted, err := f.restoreAsNewWorkspace(ctx, t, prefix)
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}
	var restore domain.Restore
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		restore, err = restoreRepo().Find(ctx, restoreID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if restore.Status != domain.RestoreSucceeded {
		t.Fatalf("the restore ended %s (%s)", restore.Status, restore.ErrorCode)
	}

	if after := cycleMinutes(ctx, t, minted); after != before {
		t.Errorf("the restored entry's cycle time is %d minutes, want the %d it had", after, before)
	}
	rows, err := adminPool(ctx, t).Query(ctx, `SELECT status, entered_at, from_restore
		FROM entry_status_change WHERE tenant_id = $1 ORDER BY seq`, minted.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var status string
		var entered time.Time
		var standIn bool
		if err := rows.Scan(&status, &entered, &standIn); err != nil {
			t.Fatal(err)
		}
		if standIn {
			t.Errorf("the restore's stand-in for %s stayed beside the archived history", status)
		}
		got = append(got, status+"@"+entered.UTC().Format(time.RFC3339))
	}
	want := []string{
		"todo@" + historyStarted.Add(-24*time.Hour).Format(time.RFC3339),
		"in_progress@" + historyStarted.Format(time.RFC3339),
		"done@" + historyCompleted.Format(time.RFC3339),
	}
	if len(got) != len(want) {
		t.Fatalf("the restored history is %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("the restored history is %v, want %v", got, want)
			break
		}
	}
}

// A history row written twice - a resumed batch, a second restore with SKIP or OVERWRITE - lands
// once, and the stand-in the entry's write left goes with the first archived row.
func TestAnArchivedStatusChangeLandsOnceAndReplacesTheStandIn(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	task := taskIn(tenantA, authorA, collection, freshID(t), "label printer", "a0")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := itemRepo().Insert(ctx, task)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	pool := adminPool(ctx, t)
	if _, err := pool.Exec(ctx, `UPDATE work_item SET status = 'in_progress' WHERE tenant_id = $1 AND id = $2`,
		tenantA.String(), task.ID.String()); err != nil {
		t.Fatal(err)
	}
	row := exportedShape(ctx, t, tenantA, task.ID)
	if _, err := pool.Exec(ctx, `DELETE FROM work_item WHERE tenant_id = $1 AND id = $2`,
		tenantA.String(), task.ID.String()); err != nil {
		t.Fatal(err)
	}
	change := map[string]any{
		"item_id": task.ID.String(), "seq": 912345, "status": "in_progress",
		"entered_at": historyStarted.Format(time.RFC3339Nano),
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := importRepo().Write(ctx, "work_item", row, false); err != nil {
			return err
		}
		for range 2 {
			if _, err := importRepo().Write(ctx, "entry_status_change", change, true); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("restoring: %v", err)
	}
	var rows, standIns int
	var entered time.Time
	if err := pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE from_restore), min(entered_at)
		FROM entry_status_change WHERE tenant_id = $1 AND item_id = $2`,
		tenantA.String(), task.ID.String()).Scan(&rows, &standIns, &entered); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || standIns != 0 || !entered.Equal(historyStarted) {
		t.Errorf("the history holds %d rows, %d stand-ins, the first at %s; want the one archived at %s",
			rows, standIns, entered, historyStarted)
	}
	var held bool
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		held, err = importRepo().Holds(ctx, "entry_status_change", change)
		return err
	}); err != nil || !held {
		t.Errorf("the workspace does not hold the change it was given (%v)", err)
	}
}
