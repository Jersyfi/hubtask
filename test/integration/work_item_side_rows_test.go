// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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
