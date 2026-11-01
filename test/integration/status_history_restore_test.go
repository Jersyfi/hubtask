// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"
)

// A replace-restore deletes the living entry, and its status history with it; the history is not
// archived. The restored entry's status is dated from the restore, not from its creation: an entry
// created a year ago that entered In Progress yesterday does not read as In Progress for a year
// (migration 0350, UC-GOL-04 check 11).
func TestARestoredEntrysStatusIsDatedFromTheRestore(t *testing.T) {
	ctx := context.Background()
	pool := adminPool(ctx, t)
	collection := collectionFor(ctx, t, tenantA, authorA)
	task := taskIn(tenantA, authorA, collection, freshID(t), "printer", "a0")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := itemRepo().Insert(ctx, task)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	yearAgo := time.Now().UTC().AddDate(-1, 0, 0)
	if _, err := pool.Exec(ctx, `UPDATE work_item SET created_at = $1, updated_at = $2, status = 'in_progress'
		WHERE tenant_id = $3 AND id = $4`, yearAgo, time.Now().UTC().AddDate(0, 0, -1),
		tenantA.String(), task.ID.String()); err != nil {
		t.Fatal(err)
	}
	row := exportedShape(ctx, t, tenantA, task.ID)
	if _, err := pool.Exec(ctx, `DELETE FROM work_item WHERE tenant_id = $1 AND id = $2`,
		tenantA.String(), task.ID.String()); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC().Add(-time.Minute)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := importRepo().Write(ctx, "work_item", row, false)
		return err
	}); err != nil {
		t.Fatalf("restoring: %v", err)
	}
	var status string
	var entered time.Time
	if err := pool.QueryRow(ctx, `SELECT status, entered_at FROM entry_status_change
		WHERE tenant_id = $1 AND item_id = $2 ORDER BY seq DESC LIMIT 1`,
		tenantA.String(), task.ID.String()).Scan(&status, &entered); err != nil {
		t.Fatalf("reading the history: %v", err)
	}
	if status != "in_progress" || entered.Before(before) {
		t.Errorf("the restored entry entered %q at %s, want in_progress from the restore (after %s)",
			status, entered, before)
	}

}
