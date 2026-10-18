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

// beforeThePasskeyMethod is the last version without migration 0175.
const beforeThePasskeyMethod = 174

// Migration 0175 puts PASSKEY into every stored list of sign-in methods that holds PASSWORD - a
// workspace's, the installation's and a plan's - and leaves a list without the password, and a
// level that stores none, alone (milestone PK, Decision 7). Run as the owner, neither superuser nor
// BYPASSRLS, so the forced row security on `tenant` applies to it.
func TestTheMigrationPutsPasskeysBesideThePassword(t *testing.T) {
	ctx := context.Background()
	const name = "passkey_method_migration"
	ownerDSN, _ := managedDatabase(ctx, t, name)
	if err := dbtest.MigrateTo(ctx, ownerDSN, beforeThePasskeyMethod); err != nil {
		t.Fatalf("migrating to %d: %v", beforeThePasskeyMethod, err)
	}
	adminDSN := regexp.MustCompile(`/[^/?]+(\?|$)`).ReplaceAllString(dbtest.Start(t).AdminDSN, "/"+name+"$1")
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer admin.Close()

	withPassword, withoutPassword, inheriting := freshID(t).String(), freshID(t).String(), freshID(t).String()
	plan := freshID(t).String()
	for i, step := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name, settings) VALUES
		    ($1, 'pk-a', 'A', '{"sign_in_policy": {"methods": ["PASSWORD", "OIDC"], "min_length": 14}}'),
		    ($2, 'pk-b', 'B', '{"sign_in_policy": {"methods": ["OIDC"]}}'),
		    ($3, 'pk-c', 'C', '{"locale": "de"}')`, []any{withPassword, withoutPassword, inheriting}},
		{`INSERT INTO instance_setting (key, value) VALUES ('sign_in.methods', '["PASSWORD"]')
		    ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, nil},
		{`INSERT INTO plan (id, name, display_name, created_at, updated_at) VALUES ($1, 'pk-plan', 'Plan', now(), now())`, []any{plan}},
		{`INSERT INTO plan_setting (plan_id, key, value) VALUES ($1, 'sign_in.methods', '["PASSWORD", "OIDC"]')`, []any{plan}},
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
			t.Fatalf("reading %q: %v", sql, err)
		}
		return value
	}
	for tenant, want := range map[string]string{
		withPassword:    `{"methods": ["PASSWORD", "OIDC", "PASSKEY"], "min_length": 14}`,
		withoutPassword: `{"methods": ["OIDC"]}`,
	} {
		if got := read(`SELECT (settings -> 'sign_in_policy')::text FROM tenant WHERE id = $1`, tenant); got != want {
			t.Errorf("tenant %s: %s, want %s", tenant, got, want)
		}
	}
	if got := read(`SELECT settings::text FROM tenant WHERE id = $1`, inheriting); got != `{"locale": "de"}` {
		t.Errorf("a workspace that stores no list gained one: %s", got)
	}
	if got := read(`SELECT value::text FROM instance_setting WHERE key = 'sign_in.methods'`); got != `["PASSWORD", "PASSKEY"]` {
		t.Errorf("the installation's list: %s", got)
	}
	if got := read(`SELECT value::text FROM plan_setting WHERE plan_id = $1`, plan); got != `["PASSWORD", "OIDC", "PASSKEY"]` {
		t.Errorf("the plan's list: %s", got)
	}
}
