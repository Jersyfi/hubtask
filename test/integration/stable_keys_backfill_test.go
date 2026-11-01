// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// The hubs and views written before 0286 are given their handles and keys by 0329's batches, past
// its first batch of 500 and oldest first on a clash; a walk stopped in its second batch resumes
// after the first. The refusing trigger is a deterministic stand-in for a killed migrator.
func TestHubsAndViewsWrittenBeforeTheStableKeysAreGivenTheirs(t *testing.T) {
	ctx := context.Background()
	pool, dsn := databaseMigratedTo(ctx, t, "stable_keys_backfill", 285)

	if _, err := pool.Exec(ctx, `
		WITH t AS (
		  INSERT INTO tenant (id, slug, display_name)
		  VALUES (gen_random_uuid(), 'stable-keys-backfill', 'Stable keys backfill') RETURNING id
		)
		INSERT INTO container (id, tenant_id, type, name, order_key, created_by, created_at)
		SELECT gen_random_uuid(), t.id, 'HUB', 'Hub ' || g, 'a' || g, gen_random_uuid(),
		       now() - make_interval(secs => 1000 - g)
		FROM t, generate_series(1, 600) g`); err != nil {
		t.Fatalf("seeding the hubs: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO saved_view (id, tenant_id, scope_type, name, layout, query, created_at)
		SELECT gen_random_uuid(), t.id, 'TENANT', 'Open work', 'LIST_COLLAPSED', '{}'::jsonb,
		       now() - make_interval(secs => 10 - g)
		FROM tenant t, generate_series(1, 3) g WHERE t.slug = 'stable-keys-backfill'`); err != nil {
		t.Fatalf("seeding the views: %v", err)
	}

	if err := dbtest.MigrateTo(ctx, dsn, 328); err != nil {
		t.Fatalf("migrating to 328: %v", err)
	}
	var stop string
	if err := pool.QueryRow(ctx, `
		SELECT c.id::text FROM container c JOIN tenant t ON t.id = c.tenant_id
		WHERE t.slug = 'stable-keys-backfill' ORDER BY c.created_at, c.id OFFSET 500 LIMIT 1`).Scan(&stop); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE FUNCTION stop_the_walk() RETURNS trigger LANGUAGE plpgsql AS $$
		 BEGIN
		   IF NEW.id = '` + stop + `' THEN RAISE EXCEPTION 'the walk is stopped here'; END IF;
		   RETURN NEW;
		 END $$`,
		`CREATE TRIGGER stop_the_walk BEFORE UPDATE ON container FOR EACH ROW EXECUTE FUNCTION stop_the_walk()`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatalf("stopping the walk: %v", err)
		}
	}
	if err := dbtest.MigrateTo(ctx, dsn, 329); err == nil {
		t.Fatal("the stopped walk reported success")
	}
	var given int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM container c JOIN tenant t ON t.id = c.tenant_id
		WHERE t.slug = 'stable-keys-backfill' AND c.handle IS NOT NULL`).Scan(&given); err != nil || given != 500 {
		t.Fatalf("the first batch gave %d hubs a handle (%v), want 500", given, err)
	}
	var position string
	if err := pool.QueryRow(ctx, `SELECT position FROM migration_cursor WHERE name = '0329_stable_keys_hubs'`).Scan(&position); err != nil {
		t.Fatalf("the stopped walk left no position: %v", err)
	}
	for _, statement := range []string{
		`DROP TRIGGER stop_the_walk ON container`,
		`DROP FUNCTION stop_the_walk()`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := dbtest.MigrateTo(ctx, dsn, 329); err != nil {
		t.Fatalf("the second run failed: %v", err)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM migration_cursor`).Scan(&left); err != nil || left != 0 {
		t.Errorf("the finished walk left %d ledger rows (%v)", left, err)
	}

	var without int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM container c JOIN tenant t ON t.id = c.tenant_id
		WHERE t.slug = 'stable-keys-backfill' AND c.handle IS NULL`).Scan(&without); err != nil {
		t.Fatal(err)
	}
	if without != 0 {
		t.Errorf("%d of 600 hubs have no handle", without)
	}

	rows, err := pool.Query(ctx, `
		SELECT v.key FROM saved_view v JOIN tenant t ON t.id = v.tenant_id
		WHERE t.slug = 'stable-keys-backfill' ORDER BY v.created_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key *string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		if key == nil {
			keys = append(keys, "<none>")
			continue
		}
		keys = append(keys, *key)
	}
	want := []string{"open_work", "open_work_2", "open_work_3"}
	if len(keys) != len(want) {
		t.Fatalf("keys %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("keys %v, want %v", keys, want)
			break
		}
	}
}
