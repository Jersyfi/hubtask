// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// A backfill killed between batches resumes after the last batch it committed when the migrator
// runs again (versioning-release.md §4): 0219's walk is stopped in its second batch by a trigger
// that refuses one entry's row, as a killed process would stop it, and the second run neither
// starts over nor leaves its ledger row behind.
func TestAnInterruptedBackfillResumesWhereItStopped(t *testing.T) {
	ctx := context.Background()
	pool, dsn := databaseMigratedTo(ctx, t, "backfill_resume", 218)

	if _, err := pool.Exec(ctx, `
		WITH t AS (
		  INSERT INTO tenant (id, slug, display_name)
		  VALUES (gen_random_uuid(), 'backfill-resume', 'Backfill resume') RETURNING id
		), h AS (
		  INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		  SELECT gen_random_uuid(), t.id, 'HUB', 'Hub', 'a0', gen_random_uuid() FROM t
		  RETURNING id, tenant_id, created_by
		), c AS (
		  INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  SELECT gen_random_uuid(), h.tenant_id, 'COLLECTION', h.id, 'Everything', 'a0', h.created_by
		  FROM h RETURNING id, tenant_id, created_by
		), d AS (
		  INSERT INTO custom_field_definition (id, tenant_id, collection_id, key, kind)
		  SELECT gen_random_uuid(), c.tenant_id, NULL, 'amount', 'NUMBER' FROM c RETURNING id
		)
		INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key,
		                       content_language, created_by, custom_fields, custom_field_refs)
		SELECT gen_random_uuid(), c.tenant_id, c.id, 'TASK', '/' || gen_random_uuid() || '/', 1,
		       'Entry ' || g, 'a' || g, 'en', c.created_by,
		       jsonb_build_object('amount', g), jsonb_build_object('amount', d.id::text)
		FROM c, d, generate_series(1, 4500) g`); err != nil {
		t.Fatalf("seeding the entries: %v", err)
	}
	// 0218's trigger projected them already; the backfill is for entries written before it.
	if _, err := pool.Exec(ctx, `DELETE FROM entry_field_value`); err != nil {
		t.Fatal(err)
	}

	var first, stop string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM work_item ORDER BY id LIMIT 1`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	// The 2001st entry, the first of the walk's second batch of 2000.
	if err := pool.QueryRow(ctx, `SELECT id::text FROM work_item ORDER BY id OFFSET 2000 LIMIT 1`).Scan(&stop); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE FUNCTION stop_the_walk() RETURNS trigger LANGUAGE plpgsql AS $$
		 BEGIN
		   IF NEW.entry_id = '` + stop + `' THEN RAISE EXCEPTION 'the walk is stopped here'; END IF;
		   RETURN NEW;
		 END $$`,
		`CREATE TRIGGER stop_the_walk BEFORE INSERT ON entry_field_value
		 FOR EACH ROW EXECUTE FUNCTION stop_the_walk()`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("stopping the walk: %v", err)
		}
	}

	if err := dbtest.MigrateTo(ctx, dsn, 219); err == nil {
		t.Fatal("the stopped walk reported success")
	}
	var position string
	if err := pool.QueryRow(ctx, `SELECT position FROM migration_cursor WHERE name = '0219_entry_field_value_backfill'`).Scan(&position); err != nil {
		t.Fatalf("the stopped walk left no position: %v", err)
	}
	var done int
	if err := pool.QueryRow(ctx, `SELECT count(DISTINCT entry_id) FROM entry_field_value`).Scan(&done); err != nil || done != 2000 {
		t.Fatalf("the first batch wrote %d entries' values (%v), want 2000", done, err)
	}

	// What the first run committed is not done again: the first entry's value, taken away now,
	// stays away when the walk resumes after the position.
	for _, statement := range []string{
		`DROP TRIGGER stop_the_walk ON entry_field_value`,
		`DROP FUNCTION stop_the_walk()`,
		`DELETE FROM entry_field_value WHERE entry_id = '` + first + `'`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := dbtest.MigrateTo(ctx, dsn, 219); err != nil {
		t.Fatalf("the second run failed: %v", err)
	}

	var missing, total, left int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM entry_field_value v WHERE v.entry_id = wi.id)),
		       count(*)
		FROM work_item wi`).Scan(&missing, &total); err != nil {
		t.Fatal(err)
	}
	if total != 4500 || missing != 1 {
		t.Errorf("%d of %d entries have no value after the second run, want only the one taken away", missing, total)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM migration_cursor`).Scan(&left); err != nil || left != 0 {
		t.Errorf("the finished walk left %d ledger rows (%v)", left, err)
	}
}
