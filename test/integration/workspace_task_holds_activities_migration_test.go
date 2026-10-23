// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"regexp"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// beforeWorkspaceTaskHoldsActivities is the last version without migration 0191.
const beforeWorkspaceTaskHoldsActivities = 190

// Migration 0191 gives ACTIVITY to a workspace's own TASK row that offered every child there was
// before 0185 - WORK_PACKAGE - as 0185 gave it to the system row, so that the Tasks preset's
// activity under a task can be created there. A row narrowed to no children, a workspace's other
// rows and the system rows are left exactly as they were. Run as the owner, neither superuser nor
// BYPASSRLS (ADR-0052). Applied twice, the statement changes nothing the second time.
func TestTheMigrationLetsAWorkspaceTaskThatOfferedEverythingHoldActivities(t *testing.T) {
	ctx := context.Background()
	const name = "workspace_task_holds_activities_migration"
	ownerDSN, _ := managedDatabase(ctx, t, name)
	if err := dbtest.MigrateTo(ctx, ownerDSN, beforeWorkspaceTaskHoldsActivities); err != nil {
		t.Fatalf("migrating to %d: %v", beforeWorkspaceTaskHoldsActivities, err)
	}
	adminDSN := regexp.MustCompile(`/[^/?]+(\?|$)`).ReplaceAllString(dbtest.Start(t).AdminDSN, "/"+name+"$1")
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer admin.Close()

	// Three workspaces with rows of their own: one whose task offers work packages - everything
	// before 0185 - one whose task offers no children, and one that already has both.
	packages, noChildren, both := freshID(t).String(), freshID(t).String(), freshID(t).String()
	for i, step := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'wtha-a', 'A'), ($2, 'wtha-b', 'B'), ($3, 'wtha-c', 'C')`,
			[]any{packages, noChildren, both}},
		{`INSERT INTO item_capability_profile (tenant_id, type, capabilities, allowed_child_types, max_depth)
		    VALUES ($1, 'TASK', ARRAY['COMPLETION','NOTES'], ARRAY['WORK_PACKAGE']::item_type[], 3),
		           ($1, 'WORK_PACKAGE', ARRAY['COMPLETION'], ARRAY['ACTIVITY']::item_type[], 2),
		           ($2, 'TASK', ARRAY['COMPLETION'], ARRAY[]::item_type[], 1),
		           ($3, 'TASK', ARRAY['COMPLETION'], ARRAY['WORK_PACKAGE','ACTIVITY']::item_type[], 3)`,
			[]any{packages, noChildren, both}},
	} {
		if _, err := admin.Exec(ctx, step.sql, step.args...); err != nil {
			t.Fatalf("seeding step %d: %v", i, err)
		}
	}

	read := func(sql string, args ...any) string {
		t.Helper()
		var value string
		if err := admin.QueryRow(ctx, sql, args...).Scan(&value); err != nil {
			t.Fatalf("reading %q: %v", sql, err)
		}
		return value
	}
	const children = `SELECT allowed_child_types::text FROM item_capability_profile
		WHERE tenant_id = $1 AND type = $2::item_type`
	const others = `SELECT coalesce(string_agg(row(icp.*)::text, '|' ORDER BY tenant_id::text, type), '')
		FROM item_capability_profile icp
		WHERE tenant_id IS NULL OR tenant_id <> $1 OR type <> 'TASK'`
	othersBefore := read(others, packages)

	if err := dbtest.MigrateTo(ctx, ownerDSN, beforeWorkspaceTaskHoldsActivities+1); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	if got := read(children, packages, "TASK"); got != "{WORK_PACKAGE,ACTIVITY}" {
		t.Errorf("the task that offered work packages holds %s, want {WORK_PACKAGE,ACTIVITY}", got)
	}
	if got := read(others, packages); got != othersBefore {
		t.Errorf("another row changed:\nbefore %s\nafter  %s", othersBefore, got)
	}
	if got := read(children, noChildren, "TASK"); got != "{}" {
		t.Errorf("the narrowed task holds %s, want {}", got)
	}

	// The statement again, as the owner: idempotent, so it finds nothing to change.
	owner, err := pgxpool.New(ctx, ownerDSN)
	if err != nil {
		t.Fatalf("owner pool: %v", err)
	}
	defer owner.Close()
	tag, err := owner.Exec(ctx, upStatementOf(t, "0191_workspace_task_holds_activities.sql"))
	if err != nil {
		t.Fatalf("running the statement again: %v", err)
	}
	if tag.RowsAffected() != 0 {
		t.Errorf("a second run changed %d rows, want none", tag.RowsAffected())
	}
}
