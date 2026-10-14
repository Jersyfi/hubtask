// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// beforeTheUniqueGrant is the last version without migration 0149. `up-to` stops at the highest
// migration at or below it, so a migration numbered below 0149 that merges later is included.
const beforeTheUniqueGrant = 148

// Migration 0149 keeps the oldest of equal memberships and adds the index that keeps them out.
//
// Run the way a managed service runs it: as the owner, neither superuser nor BYPASSRLS, so row
// level security applies to the dedupe - a superuser would delete through it and hide a migration
// that deletes nothing elsewhere. Seeded as the superuser, because the seed is the state an
// earlier release left, not something the migrator writes. A non-unique index stands under the new
// index's name, as an interrupted build would leave one; the run has to replace it rather than
// accept it.
func TestTheMigrationKeepsOneOfEqualMemberships(t *testing.T) {
	ctx := context.Background()
	const name = "membership_dedupe_migration"
	ownerDSN, _ := managedDatabase(ctx, t, name)
	if err := dbtest.MigrateTo(ctx, ownerDSN, beforeTheUniqueGrant); err != nil {
		t.Fatalf("migrating to %d: %v", beforeTheUniqueGrant, err)
	}

	adminDSN := regexp.MustCompile(`/[^/?]+(\?|$)`).ReplaceAllString(dbtest.Start(t).AdminDSN, "/"+name+"$1")
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin pool: %v", err)
	}
	defer admin.Close()

	tenant, other := freshID(t).String(), freshID(t).String()
	person, group, hub, stranger := freshID(t).String(), freshID(t).String(), freshID(t).String(), freshID(t).String()
	seed := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'A'), ($3, $4, 'B')`,
			[]any{tenant, "dedupe-a", other, "dedupe-b"}},
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Ines'), ($3, $4, 'Olaf')`,
			[]any{person, tenant, stranger, other}},
		{`INSERT INTO account_group (id, tenant_id, name) VALUES ($1, $2, 'Parents')`, []any{group, tenant}},
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		  VALUES ($1, $2, 'HUB', 'Home', 'a0', $3)`, []any{hub, tenant, person}},
		{`CREATE INDEX membership_grant_uq ON membership (tenant_id)`, nil},
	}
	for i, step := range seed {
		if _, err := admin.Exec(ctx, step.sql, step.args...); err != nil {
			t.Fatalf("seeding step %d: %v", i, err)
		}
	}

	// Each set: its rows oldest first, and how many of them the migration keeps.
	base := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	type row struct {
		id, tenant, account, group, scopeType, scope, role string
		at                                                 time.Time
	}
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }
	add := func(r row) row {
		r.id = freshID(t).String()
		return r
	}
	sets := []struct {
		what string
		rows []row
		kept int
	}{
		{"a person's tenant role, three times", []row{
			add(row{tenant: tenant, account: person, scopeType: "TENANT", role: "MEMBER", at: at(0)}),
			add(row{tenant: tenant, account: person, scopeType: "TENANT", role: "MEMBER", at: at(5)}),
			add(row{tenant: tenant, account: person, scopeType: "TENANT", role: "MEMBER", at: at(9)}),
		}, 1},
		{"a group's hub role, twice", []row{
			add(row{tenant: tenant, group: group, scopeType: "HUB", scope: hub, role: "VIEWER", at: at(1)}),
			add(row{tenant: tenant, group: group, scopeType: "HUB", scope: hub, role: "VIEWER", at: at(2)}),
		}, 1},
		{"the same person and scope with another role", []row{
			add(row{tenant: tenant, account: person, scopeType: "TENANT", role: "AUDITOR", at: at(3)}),
		}, 1},
		{"the same person at the hub", []row{
			add(row{tenant: tenant, account: person, scopeType: "HUB", scope: hub, role: "MEMBER", at: at(4)}),
		}, 1},
		{"another workspace's equal grant", []row{
			add(row{tenant: other, account: stranger, scopeType: "TENANT", role: "MEMBER", at: at(6)}),
		}, 1},
	}
	nullable := func(s string) any {
		if s == "" {
			return nil
		}
		return s
	}
	for _, set := range sets {
		// Written newest first, so that the order the rows went in cannot pass for the order kept.
		for i := len(set.rows) - 1; i >= 0; i-- {
			r := set.rows[i]
			if _, err := admin.Exec(ctx, `
				INSERT INTO membership (id, tenant_id, account_id, group_id, scope_type, scope_id, role, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
				r.id, r.tenant, nullable(r.account), nullable(r.group), r.scopeType, nullable(r.scope), r.role, r.at); err != nil {
				t.Fatalf("seeding %s: %v", set.what, err)
			}
		}
	}

	if err := dbtest.Migrate(ctx, ownerDSN); err != nil {
		t.Fatalf("the migration failed over equal memberships, as a migrator without superuser: %v", err)
	}

	for _, set := range sets {
		for i, r := range set.rows {
			var present bool
			if err := admin.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM membership WHERE id = $1)`, r.id).
				Scan(&present); err != nil {
				t.Fatalf("reading %s: %v", set.what, err)
			}
			if want := i < set.kept; present != want {
				t.Errorf("%s: row %d (of %d, oldest first) present=%v, want %v", set.what, i+1, len(set.rows), present, want)
			}
		}
	}

	var unique, valid bool
	var definition string
	if err := admin.QueryRow(ctx, `
		SELECT i.indisunique, i.indisvalid, pg_get_indexdef(i.indexrelid) FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid WHERE c.relname = 'membership_grant_uq'`).
		Scan(&unique, &valid, &definition); err != nil {
		t.Fatalf("the index is not in the catalogue: %v", err)
	}
	if !unique || !valid || !strings.Contains(definition, "NULLS NOT DISTINCT") {
		t.Errorf("membership_grant_uq is unique=%v valid=%v: %s - the leftover under its name was kept", unique, valid, definition)
	}

	var policies int
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM pg_policies WHERE tablename = 'membership' AND policyname <> 'tenant_isolation'`).
		Scan(&policies); err != nil {
		t.Fatalf("reading the policies: %v", err)
	}
	if policies != 0 {
		t.Errorf("%d policies of the migration's own are left on membership: the migrator still reads every workspace", policies)
	}

	_, err = admin.Exec(ctx, `
		INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'MEMBER')`,
		freshID(t).String(), tenant, person)
	if err == nil {
		t.Error("an equal membership was written after the migration: the index refuses nothing")
	}
}
