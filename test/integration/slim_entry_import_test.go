// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// A restore writes the side rows the archive's record carries, the export carries them in the
// record, and a replacing restore whose record carries none removes them - the flow moments aside,
// which the trigger records as the entry is written.
func TestABackupImportLandsTheSideRows(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	id := seedTask(ctx, t, tenantA, authorA, collection)
	rule := freshID(t).String()
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO work_item_announcement (tenant_id, item_id, due_soon_announced_at, overdue_announced_at)
		VALUES ($1, $2, '2026-10-20T08:00:00Z', '2026-10-21T08:00:00Z')`, tenantA.String(), id.String()); err != nil {
		t.Fatalf("announcing: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO work_item_flow (tenant_id, item_id, flow_started_at, flow_finished_at)
		VALUES ($1, $2, '2026-10-19T08:00:00Z', '2026-10-20T08:00:00Z')`, tenantA.String(), id.String()); err != nil {
		t.Fatalf("timing: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO work_item_retention (tenant_id, item_id, pending_until, rule_id, action)
		VALUES ($1, $2, '2026-11-20T08:00:00Z', $3, 'ARCHIVE')`, tenantA.String(), id.String(), rule); err != nil {
		t.Fatalf("marking: %v", err)
	}

	row := exportedShape(ctx, t, tenantA, id)
	if row["retention_action"] != "ARCHIVE" || row["retention_rule_id"] != rule ||
		row["due_soon_announced_at"] == nil || row["overdue_announced_at"] == nil ||
		row["flow_started_at"] == nil || row["flow_finished_at"] == nil || row["retention_pending_until"] == nil {
		t.Fatalf("the export's record does not carry the side rows: %v", row)
	}

	if _, err := admin.Exec(ctx, `DELETE FROM work_item WHERE tenant_id = $1 AND id = $2`,
		tenantA.String(), id.String()); err != nil {
		t.Fatal(err)
	}
	importRow := func(data map[string]any, overwrite bool) {
		t.Helper()
		if err := write(ctx, t, tenantA, func(ctx context.Context) error {
			written, err := importRepo().Write(ctx, "work_item", data, overwrite)
			if err == nil && !written {
				t.Error("the entry was not written")
			}
			return err
		}); err != nil {
			t.Fatalf("importing the entry: %v", err)
		}
	}
	importRow(row, false)
	if a, f, r := sideRows(ctx, t, id); a != 1 || f != 1 || r != 1 {
		t.Fatalf("a restored entry has side rows %d, %d, %d, want one each", a, f, r)
	}
	keys := []string{"due_soon_announced_at", "overdue_announced_at", "flow_started_at", "flow_finished_at",
		"retention_pending_until", "retention_rule_id", "retention_action", "retention_blocked_by"}
	again := exportedShape(ctx, t, tenantA, id)
	for _, key := range keys {
		if !sameJSON(again[key], row[key]) {
			t.Errorf("the restore did not round-trip %s: %v, want %v", key, again[key], row[key])
		}
	}

	for _, key := range keys {
		row[key] = nil
	}
	importRow(row, true)
	if a, f, r := sideRows(ctx, t, id); a != 0 || r != 0 || f != 1 {
		t.Errorf("a replacing restore without them left side rows %d, %d, %d, want the flow's alone", a, f, r)
	}
}

// Gate SG-3 for the restore's side rows: written in tenant B for an identifier of tenant A's
// entry - which the import reaches only after writing the entry in its own tenant - they are
// refused or change nothing of tenant A's.
func TestTheImportsSideRowsStopAtTheTenantBoundary(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	marked := seedTask(ctx, t, tenantA, authorA, collection)
	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO work_item_announcement (tenant_id, item_id, overdue_announced_at) VALUES ($1, $2, now())`,
		tenantA.String(), marked.String()); err != nil {
		t.Fatalf("announcing A's entry: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO work_item_retention (tenant_id, item_id, pending_until, rule_id, action)
		VALUES ($1, $2, now() + interval '1 day', gen_random_uuid(), 'ARCHIVE')`,
		tenantA.String(), marked.String()); err != nil {
		t.Fatalf("marking A's entry: %v", err)
	}
	beforeMarked, beforeAnnounced := retentionOf(ctx, t, marked), announcementOf(ctx, t, marked)

	for _, carried := range []bool{false, true} {
		record := map[string]any{"id": marked.String(), "retention_action": nil, "overdue_announced_at": nil}
		if carried {
			record["retention_action"], record["overdue_announced_at"] = "TRASH", "2026-10-20T08:00:00Z"
		}
		payload, _ := json.Marshal(record)
		tx, err := appPool(ctx, t).Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenantB.String()); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatal(err)
		}
		err = sqlc.New(tx).ImportWorkItemSideRows(ctx, payload)
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if carried && err == nil {
			t.Error("side rows for another tenant's entry were accepted")
		}
	}

	if after := retentionOf(ctx, t, marked); !after.exists || after.action == nil ||
		*after.action != *beforeMarked.action || !after.pending.Equal(*beforeMarked.pending) {
		t.Errorf("tenant A's marking changed from %+v to %+v", beforeMarked, after)
	}
	if after := announcementOf(ctx, t, marked); !after.exists || !after.over.Equal(*beforeAnnounced.over) {
		t.Errorf("tenant A's announcement changed from %+v to %+v", beforeAnnounced, after)
	}
}
