// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// beforeTaskHoldsActivities is the last version without migration 0185.
const beforeTaskHoldsActivities = 184

// Migration 0185 lets a task hold activities (domain-model.md §2): the system TASK row gains
// ACTIVITY after WORK_PACKAGE, and a workspace's own TASK row - a narrowing - is left exactly as it
// was. Run as the owner, neither superuser nor BYPASSRLS, which is the role that has to be able to
// write the system rows (ADR-0052). Applied twice, the statement changes nothing the second time.
func TestTheMigrationLetsATaskHoldActivitiesAndLeavesNarrowingsAlone(t *testing.T) {
	ctx := context.Background()
	const name = "task_holds_activities_migration"
	ownerDSN, _ := managedDatabase(ctx, t, name)
	if err := dbtest.MigrateTo(ctx, ownerDSN, beforeTaskHoldsActivities); err != nil {
		t.Fatalf("migrating to %d: %v", beforeTaskHoldsActivities, err)
	}
	adminDSN := regexp.MustCompile(`/[^/?]+(\?|$)`).ReplaceAllString(dbtest.Start(t).AdminDSN, "/"+name+"$1")
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer admin.Close()

	// Two workspaces with a TASK row of their own: one narrowed to work packages only, one to no
	// children at all.
	packagesOnly, noChildren := freshID(t).String(), freshID(t).String()
	for i, step := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'tha-a', 'A'), ($2, 'tha-b', 'B')`,
			[]any{packagesOnly, noChildren}},
		{`INSERT INTO item_capability_profile (tenant_id, type, capabilities, allowed_child_types, max_depth)
		    VALUES ($1, 'TASK', ARRAY['COMPLETION','NOTES'], ARRAY['WORK_PACKAGE']::item_type[], 3),
		           ($2, 'TASK', ARRAY['COMPLETION'], ARRAY[]::item_type[], 1)`, []any{packagesOnly, noChildren}},
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
	const tenantRows = `SELECT string_agg(row(icp.*)::text, '|' ORDER BY tenant_id::text)
		FROM item_capability_profile icp WHERE tenant_id IS NOT NULL`
	const systemTask = `SELECT allowed_child_types::text FROM item_capability_profile
		WHERE tenant_id IS NULL AND type = 'TASK'`
	if got := read(systemTask); got != "{WORK_PACKAGE}" {
		t.Fatalf("before 0185 the system TASK row holds %s", got)
	}
	tenantsBefore := read(tenantRows)

	// To 0185 alone: 0191 widens the first of these rows, and has its own test.
	if err := dbtest.MigrateTo(ctx, ownerDSN, beforeTaskHoldsActivities+1); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	if got := read(systemTask); got != "{WORK_PACKAGE,ACTIVITY}" {
		t.Errorf("the system TASK row holds %s, want {WORK_PACKAGE,ACTIVITY}", got)
	}
	if got := read(tenantRows); got != tenantsBefore {
		t.Errorf("a workspace's own row changed:\nbefore %s\nafter  %s", tenantsBefore, got)
	}
	for _, other := range []string{"WORK_PACKAGE", "ACTIVITY"} {
		got := read(`SELECT allowed_child_types::text FROM item_capability_profile
			WHERE tenant_id IS NULL AND type = $1::item_type`, other)
		want := map[string]string{"WORK_PACKAGE": "{ACTIVITY}", "ACTIVITY": "{}"}[other]
		if got != want {
			t.Errorf("the system %s row holds %s, want %s", other, got, want)
		}
	}

	// The statement again, as the owner: idempotent, so it finds nothing to change.
	owner, err := pgxpool.New(ctx, ownerDSN)
	if err != nil {
		t.Fatalf("owner pool: %v", err)
	}
	defer owner.Close()
	tag, err := owner.Exec(ctx, upStatementOf(t, "0185_task_holds_activities.sql"))
	if err != nil {
		t.Fatalf("running the statement again: %v", err)
	}
	if tag.RowsAffected() != 0 {
		t.Errorf("a second run changed %d rows, want none", tag.RowsAffected())
	}
}

// upStatementOf reads a migration's Up section, so the second run is the file's own statement
// rather than a copy of it that could drift.
func upStatementOf(t *testing.T, file string) string {
	t.Helper()
	root, err := dbtest.RepositoryRoot()
	if err != nil {
		t.Fatalf("finding the repository: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, "db", "migrations", file))
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	text := string(content)
	start := strings.Index(text, "-- +goose Up")
	end := strings.Index(text, "-- +goose Down")
	if start < 0 || end < start {
		t.Fatalf("%s has no Up section", file)
	}
	return text[start+len("-- +goose Up") : end]
}
