// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"database/sql"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/db"
)

// withoutFS hides one migration file, so that a database can be brought to the state two pull
// requests leave behind: a later number applied, an earlier one never seen.
type withoutFS struct {
	fs.FS
	hidden string
}

func (w withoutFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(w.FS, name)
	if err != nil {
		return nil, err
	}
	kept := entries[:0]
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), w.hidden) {
			kept = append(kept, entry)
		}
	}
	return kept, nil
}

// The integration environment stopped deploying on 2026-09-16 with exactly this ledger: 0091
// applied by the pull request that merged first, 0090 merged after it and refused by goose as
// "missing". The migrator applies it.
func TestUpAppliesAMigrationMergedAfterAHigherNumberedOne(t *testing.T) {
	if testing.Short() {
		t.Skip("this test starts a database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	container, err := tcpostgres.Run(ctx, "pgvector/pgvector:pg16",
		tcpostgres.WithDatabase("hubtask"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		t.Fatalf("starting PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	defer func() { _ = pool.Close() }()

	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("dialect: %v", err)
	}
	goose.SetLogger(goose.NopLogger())

	// Everything but 0090: the ledger 0091's pull request left behind.
	goose.SetBaseFS(withoutFS{FS: db.Migrations, hidden: "0090_"})
	if err := goose.UpContext(ctx, pool, "migrations"); err != nil {
		t.Fatalf("bringing the database to the state without 0090: %v", err)
	}
	before, err := goose.GetDBVersionContext(ctx, pool)
	if err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}
	if before < 91 {
		t.Fatalf("the ledger is at %d, so the setup did not reach 0091", before)
	}

	// Then 0090 arrives, as it did on 2026-09-16.
	goose.SetBaseFS(db.Migrations)
	if err := up(ctx, pool, pool, migrationFiles(), time.Millisecond); err != nil {
		t.Fatalf("the migrator refused a migration merged after a higher-numbered one: %v", err)
	}

	var applied bool
	if err := pool.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id = 90 AND is_applied)`).Scan(&applied); err != nil {
		t.Fatalf("asking the ledger: %v", err)
	}
	if !applied {
		t.Fatal("0090 is not in the ledger after the migrator ran")
	}
}

func startDatabase(ctx context.Context, t *testing.T) string {
	t.Helper()
	container, err := tcpostgres.Run(ctx, "pgvector/pgvector:pg16",
		tcpostgres.WithDatabase("hubtask"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		t.Fatalf("starting PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.WithoutCancel(ctx)) })
	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	return dsn
}

// A transactional migration behind a long read waits a bounded time, not the read's length, and
// is retried until the read is gone: the table is never held for longer than the bound. Behind a
// read that does not end, the run fails with the lock's own code instead of queueing the table.
func TestATransactionalMigrationWaitsBoundedAndIsRetried(t *testing.T) {
	if testing.Short() {
		t.Skip("this test starts a database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	dsn := startDatabase(ctx, t)
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pool.Close() }()
	guarded, err := openGuarded(dsn, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = guarded.Close() }()
	if _, err := pool.ExecContext(ctx, `CREATE TABLE notification (id int)`); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"0001_add.sql":   {Data: []byte("-- +goose Up\nALTER TABLE notification ADD COLUMN read_at timestamptz;\n")},
		"0002_again.sql": {Data: []byte("-- +goose Up\nALTER TABLE notification ADD COLUMN seen_at timestamptz;\n")},
	}

	// A read that holds its lock for two seconds.
	reader, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ExecContext(ctx, `LOCK TABLE notification IN ACCESS SHARE MODE`); err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	concurrency.Go(ctx, "test.reader", func(context.Context) {
		time.Sleep(2 * time.Second)
		_ = reader.Rollback()
		close(released)
	})
	started := time.Now()
	if err := up(ctx, pool, guarded, files, 500*time.Millisecond); err != nil {
		t.Fatalf("the migration behind a read that ends: %v", err)
	}
	<-released
	if elapsed := time.Since(started); elapsed < 2*time.Second {
		t.Errorf("applied after %v, before the read let go", elapsed)
	}

	// A read that does not end within the retries.
	if _, err := pool.ExecContext(ctx, `CREATE TABLE stuck (id int)`); err != nil {
		t.Fatal(err)
	}
	holder, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.ExecContext(ctx, `LOCK TABLE stuck IN ACCESS SHARE MODE`); err != nil {
		t.Fatal(err)
	}
	files["0003_stuck.sql"] = &fstest.MapFile{Data: []byte("-- +goose Up\nALTER TABLE stuck ADD COLUMN x int;\n")}
	if err := up(ctx, pool, guarded, files, time.Millisecond); !isLockTimeout(err) {
		t.Errorf("behind a read that does not end: %v, want lock_not_available", err)
	}
}

// A backfill killed between two batches left its policy behind (0235); the next run drops it and
// the file applies, rather than failing on CREATE POLICY at every retry.
func TestALeftoverBackfillPolicyDoesNotStopTheNextRun(t *testing.T) {
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
	if err := goose.UpToContext(ctx, pool, "migrations", 234); err != nil {
		t.Fatalf("bringing the database to 0234: %v", err)
	}
	// What a killed 0235 leaves: the policy, committed with its first batch, and no ledger row.
	if _, err := pool.ExecContext(ctx, `CREATE POLICY side_rows_backfill ON work_item TO CURRENT_USER USING (true) WITH CHECK (true)`); err != nil {
		t.Fatal(err)
	}
	if err := up(ctx, pool, pool, migrationFiles(), time.Millisecond); err != nil {
		t.Fatalf("the run after a killed backfill: %v", err)
	}
	var left int
	if err := pool.QueryRowContext(ctx, `SELECT count(*) FROM pg_policies WHERE policyname LIKE '%\_backfill'`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d backfill policies left after the run", left)
	}
}
