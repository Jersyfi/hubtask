// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/Jersyfi/hubtask/db"
)

func TestIndexBuildsReadsTheConcurrentBuildsOfTheUpHalfOnly(t *testing.T) {
	files := fstest.MapFS{
		"0001_a.sql": {Data: []byte("-- +goose NO TRANSACTION\n-- +goose Up\n" +
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS a_idx\n  ON a (x) WHERE y;\n" +
			"CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS \"b_idx\" ON b (z);\n" +
			"-- +goose Down\nCREATE INDEX CONCURRENTLY IF NOT EXISTS down_idx ON a (x);\n")},
		"0002_tx.sql": {Data: []byte("-- +goose Up\nCREATE INDEX IF NOT EXISTS tx_idx ON a (x);\n")},
	}
	builds, err := indexBuilds(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(builds) != 2 || !strings.HasSuffix(builds["a_idx"], "WHERE y;") || builds["b_idx"] == "" {
		t.Errorf("builds = %q", builds)
	}
}

func TestEveryEmbeddedIndexBuildIsFound(t *testing.T) {
	builds, err := indexBuilds(migrationFiles())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(builds["wi_status_idx"], "ON work_item") {
		t.Errorf("0224's build was not found: %q", builds["wi_status_idx"])
	}
}

// An interrupted CREATE INDEX CONCURRENTLY left wi_status_idx invalid and the retry's IF NOT
// EXISTS recorded 0224 as applied over it; the next run builds it again.
func TestAnIndexLeftInvalidByAnInterruptedBuildIsBuiltAgain(t *testing.T) {
	if testing.Short() {
		t.Skip("this test starts a database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	dsn := startDatabase(ctx, t)
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(db.Migrations)
	if err := goose.UpToContext(ctx, pool, "migrations", 224); err != nil {
		t.Fatalf("bringing the database to 0224: %v", err)
	}
	// What the interrupted build leaves, as the catalogue sees it.
	if _, err := pool.ExecContext(ctx,
		`UPDATE pg_index SET indisvalid = false WHERE indexrelid = 'public.wi_status_idx'::regclass`); err != nil {
		t.Fatal(err)
	}
	if err := up(ctx, pool, pool, migrationFiles(), time.Millisecond); err != nil {
		t.Fatalf("the run after an interrupted build: %v", err)
	}
	invalid, err := invalidIndexes(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(invalid) != 0 {
		t.Errorf("invalid indexes after the run: %v", invalid)
	}
}
