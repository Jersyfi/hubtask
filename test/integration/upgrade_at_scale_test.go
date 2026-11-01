// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// lastReleasedMigration is the schema v0.9.1 shipped with.
const lastReleasedMigration = 206

// How long the upgrade from the last release takes on a workspace of HUBTASK_UPGRADE_ENTRIES
// entries, migration by migration (versioning-release.md §4). A measurement, not a gate: it runs
// only when the variable is set, because seeding a million entries is minutes, and it logs every
// migration that takes longer than a second. It fails only when the upgrade does.
func TestTheUpgradeFromTheLastReleaseAtScale(t *testing.T) {
	entries, _ := strconv.Atoi(os.Getenv("HUBTASK_UPGRADE_ENTRIES"))
	if entries <= 0 {
		t.Skip("HUBTASK_UPGRADE_ENTRIES not set")
	}
	ctx := context.Background()
	pool, dsn := databaseMigratedTo(ctx, t, "upgrade_at_scale", lastReleasedMigration)

	seeded := time.Now()
	// Ten collections in one workspace, the entries spread over them.
	if _, err := pool.Exec(ctx, `
		WITH t AS (
		  INSERT INTO tenant (id, slug, display_name)
		  VALUES (gen_random_uuid(), 'upgrade-at-scale', 'Upgrade at scale') RETURNING id
		), h AS (
		  INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		  SELECT gen_random_uuid(), t.id, 'HUB', 'Hub', 'a0', gen_random_uuid() FROM t
		  RETURNING id, tenant_id, created_by
		)
		INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		SELECT gen_random_uuid(), h.tenant_id, 'COLLECTION', h.id, 'Collection ' || g, 'a' || g, h.created_by
		FROM h, generate_series(0, 9) g`); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}
	// In batches, one transaction each, as entries arrive: one statement of a million rows bumps each
	// collection's number counter a hundred thousand times in one transaction, and that row's
	// version chain makes the seed quadratic.
	const batch = 10_000
	for from := 1; from <= entries; from += batch {
		if _, err := pool.Exec(ctx, `
			WITH c AS (
			  SELECT id, tenant_id, created_by, row_number() OVER (ORDER BY id) - 1 AS n
			  FROM container WHERE type = 'COLLECTION'
			)
			INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, notes, order_key,
			                       content_language, created_by)
			SELECT gen_random_uuid(), c.tenant_id, c.id, 'TASK', '/' || gen_random_uuid() || '/', 1,
			       'Entry ' || g, 'Notes of entry ' || g, 'a' || g, 'en', c.created_by
			FROM generate_series($1::int, least($1::int + $2::int - 1, $3::int)) g JOIN c ON c.n = g % 10`,
			from, batch, entries); err != nil {
			t.Fatalf("seeding the entries: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `ANALYZE`); err != nil {
		t.Fatal(err)
	}
	t.Logf("seeded %d entries in %s", entries, time.Since(seeded).Round(time.Millisecond))

	latest := latestMigration(t)
	started := time.Now()
	for version := lastReleasedMigration + 1; version <= latest; version++ {
		step := time.Now()
		if err := dbtest.MigrateTo(ctx, dsn, version); err != nil {
			t.Fatalf("migrating to %d: %v", version, err)
		}
		if took := time.Since(step); took > time.Second {
			t.Logf("migration %04d: %s", version, took.Round(time.Millisecond))
		}
	}
	t.Logf("upgrade %04d -> %04d on %d entries: %s", lastReleasedMigration, latest, entries,
		time.Since(started).Round(time.Millisecond))
}

// latestMigration is the highest number in db/migrations.
func latestMigration(t *testing.T) int {
	t.Helper()
	files, err := os.ReadDir("../../db/migrations")
	if err != nil {
		t.Fatalf("listing the migrations: %v", err)
	}
	latest := 0
	for _, file := range files {
		if len(file.Name()) < 4 {
			continue
		}
		if number, err := strconv.Atoi(file.Name()[:4]); err == nil && number > latest {
			latest = number
		}
	}
	return latest
}
