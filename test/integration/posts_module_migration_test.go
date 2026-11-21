// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"regexp"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// beforeThePostsModule is the last version without migration 0444.
const beforeThePostsModule = 443

// Migration 0444 switches posts on wherever knowledge is switched - a workspace's, a hub's, a
// plan's and the installation's switch, with its hold or lock - and leaves a place that already
// decides posts alone, in every workspace. Run as the owner, neither superuser nor BYPASSRLS, so
// the forced row security on `setting_value` applies to it.
func TestTheMigrationSwitchesPostsWhereKnowledgeIs(t *testing.T) {
	ctx := context.Background()
	const name = "posts_module_migration"
	ownerDSN, _ := managedDatabase(ctx, t, name)
	if err := dbtest.MigrateTo(ctx, ownerDSN, beforeThePostsModule); err != nil {
		t.Fatalf("migrating to %d: %v", beforeThePostsModule, err)
	}
	adminDSN := regexp.MustCompile(`/[^/?]+(\?|$)`).ReplaceAllString(dbtest.Start(t).AdminDSN, "/"+name+"$1")
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer admin.Close()

	one, other := freshID(t).String(), freshID(t).String()
	hub, decided, plan := freshID(t).String(), freshID(t).String(), freshID(t).String()
	var version int64
	if err := admin.QueryRow(ctx, `SELECT value::text::bigint FROM instance_setting WHERE key = 'level.version'`).Scan(&version); err != nil {
		t.Fatalf("reading the installation level's version: %v", err)
	}
	for i, step := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'posts-a', 'A'), ($2, 'posts-b', 'B')`, []any{one, other}},
		{`INSERT INTO setting_value (id, tenant_id, level, scope_id, key, value, hold, updated_at) VALUES
		    (gen_random_uuid(), $1, 'WORKSPACE', $1, 'module.knowledge', 'true', 'LOCK', now()),
		    (gen_random_uuid(), $1, 'HUB', $3, 'module.knowledge', 'false', 'SET', now()),
		    (gen_random_uuid(), $2, 'WORKSPACE', $2, 'module.knowledge', 'true', 'SET', now()),
		    (gen_random_uuid(), $2, 'HUB', $4, 'module.knowledge', 'true', 'SET', now()),
		    (gen_random_uuid(), $2, 'HUB', $4, 'module.posts', 'false', 'SET', now()),
		    (gen_random_uuid(), $2, 'WORKSPACE', $2, 'module.planning', 'true', 'SET', now())`, []any{one, other, hub, decided}},
		{`INSERT INTO plan (id, name, display_name, created_at, updated_at) VALUES ($1, 'posts-plan', 'Plan', now(), now())`, []any{plan}},
		{`INSERT INTO plan_setting (plan_id, key, value, locked) VALUES ($1, 'module.knowledge', 'false', true)`, []any{plan}},
		{`INSERT INTO instance_setting (key, value, lock_origin) VALUES ('module.knowledge', 'true', 'INSTANCE')
		    ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, lock_origin = EXCLUDED.lock_origin`, nil},
	} {
		if _, err := admin.Exec(ctx, step.sql, step.args...); err != nil {
			t.Fatalf("seeding step %d: %v", i, err)
		}
	}

	if err := dbtest.Migrate(ctx, ownerDSN); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	read := func(sql string, args ...any) string {
		t.Helper()
		var value string
		if err := admin.QueryRow(ctx, sql, args...).Scan(&value); err != nil {
			t.Fatalf("reading %q %v: %v", sql, args, err)
		}
		return value
	}
	switched := `SELECT value::text || ' ' || hold FROM setting_value WHERE tenant_id = $1 AND scope_id = $2 AND key = 'module.posts'`
	for _, row := range []struct{ tenant, scope, want string }{
		{one, one, "true LOCK"},
		{one, hub, "false SET"},
		{other, other, "true SET"},
		{other, decided, "false SET"}, // decided before: left alone
	} {
		if got := read(switched, row.tenant, row.scope); got != row.want {
			t.Errorf("posts at %s of %s: %s, want %s", row.scope, row.tenant, got, row.want)
		}
	}
	if got := read(`SELECT count(*)::text FROM setting_value WHERE key = 'module.posts' AND tenant_id IN ($1, $2)`, one, other); got != "4" {
		t.Errorf("%s posts switches, want one per knowledge switch", got)
	}
	if got := read(`SELECT value::text || ' ' || locked::text FROM plan_setting WHERE plan_id = $1 AND key = 'module.posts'`, plan); got != "false true" {
		t.Errorf("the plan's posts: %s, want locked off as its knowledge", got)
	}
	if got := read(`SELECT value::text || ' ' || lock_origin FROM instance_setting WHERE key = 'module.posts'`); got != "true INSTANCE" {
		t.Errorf("the installation's posts: %s, want locked on as its knowledge", got)
	}
	if got := read(`SELECT value::text FROM instance_setting WHERE key = 'level.version'`); got != strconv.FormatInt(version+1, 10) {
		t.Errorf("the installation level's version is %s, want %d", got, version+1)
	}
	if got := read(`SELECT count(*)::text FROM pg_policies WHERE tablename = 'setting_value' AND policyname = 'posts_module_backfill'`); got != "0" {
		t.Errorf("the migration left its own policy behind")
	}
}
