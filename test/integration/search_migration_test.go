// SPDX-License-Identifier: BUSL-1.1
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

// The migration that gives `work_item` its language-dependent document (C-08, migration 0019,
// ADR-0034), proved where the acceptance criterion puts it: against a table that already has rows
// in it.
//
// The other suites migrate an empty database, which is the one state in which this migration
// cannot go wrong. Here it runs the way it runs in production - over a populated table, from the
// version before it - and the first subtest is the whole reason the document is maintained by a
// trigger rather than generated: a STORED generated column would have rewritten the table, and a
// rewrite holds ACCESS EXCLUSIVE for the length of the rewrite.

// beforeTheSearchDocument is the last version that knows nothing about it.
const beforeTheSearchDocument = 18

func TestTheSearchDocumentMigration(t *testing.T) {
	ctx := context.Background()
	pool, dsn := databaseMigratedTo(ctx, t, "search_migration", beforeTheSearchDocument)

	collection := seedItemsToSearch(ctx, t, pool)

	// A table's filenode changes when and only when the table is rewritten. It is the exact
	// question the acceptance criterion asks, and it is deterministic - unlike timing the lock,
	// which would be a flake on a loaded machine.
	before := filenodeOf(ctx, t, pool)

	if err := dbtest.Migrate(ctx, dsn); err != nil {
		t.Fatalf("the migration failed on a populated table: %v", err)
	}

	t.Run("the table is not rewritten", func(t *testing.T) {
		if after := filenodeOf(ctx, t, pool); after != before {
			t.Errorf("work_item was rewritten: filenode %d became %d - a rewrite holds "+
				"ACCESS EXCLUSIVE for its whole length, which is the migration a rolling "+
				"update does not survive", before, after)
		}
	})

	t.Run("every row that was already there carries a document", func(t *testing.T) {
		var missing int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM work_item WHERE search_document IS NULL`).Scan(&missing); err != nil {
			t.Fatalf("counting: %v", err)
		}
		if missing != 0 {
			t.Errorf("%d rows were left without a document - the backfill wrote nothing for them, "+
				"and a search would answer a shorter list than the truth", missing)
		}
	})

	t.Run("both indexes are valid", func(t *testing.T) {
		for _, index := range []string{"wi_search_document_idx", "wi_search_trgm_idx"} {
			var valid, ready bool
			if err := pool.QueryRow(ctx, `
				SELECT i.indisvalid, i.indisready FROM pg_index i
				JOIN pg_class c ON c.oid = i.indexrelid WHERE c.relname = $1`,
				index).Scan(&valid, &ready); err != nil {
				t.Fatalf("%s is not in the catalogue: %v", index, err)
			}
			// CONCURRENTLY leaves an invalid index behind when its build is interrupted, and an
			// invalid index is one the planner never uses - a search that silently seq-scans.
			if !valid || !ready {
				t.Errorf("%s is not usable: valid=%v ready=%v", index, valid, ready)
			}
		}
	})

	t.Run("the trigger maintains what is written after it", func(t *testing.T) {
		id := insertItem(ctx, t, pool, collection, "Rechnungsprüfung", "", "de")

		var document string
		if err := pool.QueryRow(ctx,
			`SELECT search_document::text FROM work_item WHERE id = $1`, id).Scan(&document); err != nil {
			t.Fatalf("reading the document: %v", err)
		}
		if document == "" {
			t.Fatal("a row written after the migration has no document at all")
		}

		// And a change to the title is a change to the document: the trigger fires on UPDATE OF
		// the three columns it is built from.
		if _, err := pool.Exec(ctx,
			`UPDATE work_item SET title = 'Angebote vergleichen' WHERE id = $1`, id); err != nil {
			t.Fatalf("updating: %v", err)
		}
		var stale bool
		if err := pool.QueryRow(ctx, `
			SELECT search_document @@ websearch_to_tsquery('german', 'Rechnungsprüfung')
			FROM work_item WHERE id = $1`, id).Scan(&stale); err != nil {
			t.Fatalf("reading the document: %v", err)
		}
		if stale {
			t.Error("the document still matches the old title - the trigger did not fire on UPDATE")
		}
	})

	// The point of the whole migration, at the level the database answers it. The same three
	// questions are asked of the use case in search_test.go; here they prove that the configuration
	// the trigger chose is the one the row was indexed under.
	t.Run("a German query finds a compound word", func(t *testing.T) {
		if !matches(ctx, t, pool, "german", "Hausaufgabenbetreuungen", "Hausaufgabenbetreuung") {
			t.Error("the German entry was not indexed under the german configuration")
		}
		// What the generated column would have answered: `simple` indexes the word as it is
		// written, so the inflected query finds nothing. Without this control the subtest above
		// would pass on a database that never learned any German.
		if theOldColumnMatches(ctx, t, pool, "Hausaufgabenbetreuungen", "Hausaufgabenbetreuung") {
			t.Error("`simple` matched an inflected form, so this proves nothing about the configuration")
		}
	})

	t.Run("an English query finds a stemmed form", func(t *testing.T) {
		if !matches(ctx, t, pool, "english", "run", "Running the numbers") {
			t.Error("the English entry was not indexed under the english configuration")
		}
		if theOldColumnMatches(ctx, t, pool, "run", "Running the numbers") {
			t.Error("`simple` matched a stem, so this proves nothing about the configuration")
		}
	})

	t.Run("a CJK query matches a substring through the trigram index", func(t *testing.T) {
		// No word boundaries, so the whole run of characters is one token and a tsquery for part
		// of it matches nothing. The trigram index is not an optimisation here - it is the search
		// (i18n-l10n.md §5).
		var found bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM work_item
			  WHERE (coalesce(title, '') || ' ' || coalesce(notes, '')) ILIKE '%' || $1 || '%')`,
			"議事").Scan(&found); err != nil {
			t.Fatalf("querying: %v", err)
		}
		if !found {
			t.Error("the substring of a Japanese title was not found")
		}
		if matches(ctx, t, pool, "simple", "議事", "会議の議事録") ||
			theOldColumnMatches(ctx, t, pool, "議事", "会議の議事録") {
			t.Error("a tsquery found a substring of a CJK token, which would make the trigram " +
				"index unnecessary - check the fixture rather than believing it")
		}
	})
}

// matches asks whether one title's document answers a query parsed under one configuration.
func matches(ctx context.Context, t *testing.T, pool *pgxpool.Pool, config, query, title string) bool {
	t.Helper()

	var found bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM work_item
		  WHERE title = $1 AND search_document @@ websearch_to_tsquery($2::regconfig, $3))`,
		title, config, query).Scan(&found); err != nil {
		t.Fatalf("querying: %v", err)
	}
	return found
}

// theOldColumnMatches asks the same question of `to_tsvector('simple', …)` - the expression the
// generated column was built with, and therefore the answer this whole migration replaces.
func theOldColumnMatches(ctx context.Context, t *testing.T, pool *pgxpool.Pool, query, title string) bool {
	t.Helper()

	var found bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM work_item
		  WHERE title = $1
		    AND to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(notes, ''))
		        @@ websearch_to_tsquery('simple', $2))`,
		title, query).Scan(&found); err != nil {
		t.Fatalf("querying: %v", err)
	}
	return found
}

// databaseMigratedTo creates a database of its own in the same cluster and migrates it to one
// version, so that a test can populate the state a later migration then runs against.
func databaseMigratedTo(
	ctx context.Context, t *testing.T, name string, version int,
) (*pgxpool.Pool, string) {
	t.Helper()
	db := dbtest.Start(t)
	admin := adminPool(ctx, t)

	// The name is a constant of this file, not anything a caller supplies; CREATE DATABASE takes
	// no parameters.
	if _, err := admin.Exec(ctx, `DROP DATABASE IF EXISTS `+name); err != nil {
		t.Fatalf("dropping %s: %v", name, err)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}

	dsn := regexp.MustCompile(`/[^/?]+(\?|$)`).ReplaceAllString(db.AdminDSN, "/"+name+"$1")
	if err := dbtest.MigrateTo(ctx, dsn, version); err != nil {
		t.Fatalf("migrating %s to %d: %v", name, version, err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting to %s: %v", name, err)
	}
	t.Cleanup(pool.Close)
	return pool, dsn
}

// seedItemsToSearch populates the table the migration then runs against, and answers the collection
// its entries are in.
//
// Written as SQL rather than through the repository, because the repository speaks to the suite's
// own database and this one is a version behind it. Enough rows that the backfill takes more than
// one look at the table, and three whose language is the point.
func seedItemsToSearch(ctx context.Context, t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	var tenant, hub, collection, author string
	if err := pool.QueryRow(ctx, `
		WITH t AS (
		  INSERT INTO tenant (id, slug, display_name)
		  VALUES (gen_random_uuid(), 'search-migration', 'Search migration') RETURNING id
		), h AS (
		  INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		  SELECT gen_random_uuid(), t.id, 'HUB', 'Hub', 'a0', gen_random_uuid() FROM t
		  RETURNING id, tenant_id, created_by
		), c AS (
		  INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		  SELECT gen_random_uuid(), h.tenant_id, 'COLLECTION', h.id, 'Everything', 'a0', h.created_by
		  FROM h RETURNING id
		)
		SELECT h.tenant_id, h.id, c.id, h.created_by FROM h, c`,
	).Scan(&tenant, &hub, &collection, &author); err != nil {
		t.Fatalf("seeding the workspace: %v", err)
	}

	// The bulk: enough rows that the backfill's keyset walk runs more than once at its batch size
	// would be minutes of fixture, so the volume here is about the table being populated at all -
	// what the walk itself does is proved by the count of documents afterwards.
	if _, err := pool.Exec(ctx, `
		INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key,
		                       content_language, created_by)
		SELECT gen_random_uuid(), $1, $2, 'TASK', '/' || gen_random_uuid() || '/', 1,
		       'Existing entry ' || g, 'a' || g, 'en', $3
		FROM generate_series(1, 500) g`, tenant, collection, author); err != nil {
		t.Fatalf("seeding the entries: %v", err)
	}

	insertItem(ctx, t, pool, collection, "Hausaufgabenbetreuung", "Bäume gießen", "de-AT")
	insertItem(ctx, t, pool, collection, "Running the numbers", "", "en")
	insertItem(ctx, t, pool, collection, "会議の議事録", "", "ja")

	return collection
}

// insertItem writes one entry into the seeded collection and answers its identifier.
func insertItem(
	ctx context.Context, t *testing.T, pool *pgxpool.Pool, collection, title, notes, language string,
) string {
	t.Helper()

	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, notes,
		                       order_key, content_language, created_by)
		SELECT gen_random_uuid(), c.tenant_id, c.id, 'TASK', '/' || gen_random_uuid() || '/', 1,
		       $2, nullif($3, ''), 'z' || floor(random() * 1000000)::text, $4, c.created_by
		FROM container c WHERE c.id = $1
		RETURNING id`, collection, title, notes, language).Scan(&id); err != nil {
		t.Fatalf("inserting %q: %v", title, err)
	}
	return id
}

// filenodeOf is the physical file behind the table. A rewrite gives the table a new one; every
// change that is only a catalogue entry leaves it alone.
func filenodeOf(ctx context.Context, t *testing.T, pool *pgxpool.Pool) uint32 {
	t.Helper()

	var filenode uint32
	if err := pool.QueryRow(ctx, `SELECT pg_relation_filenode('work_item')`).Scan(&filenode); err != nil {
		t.Fatalf("reading the filenode: %v", err)
	}
	return filenode
}
