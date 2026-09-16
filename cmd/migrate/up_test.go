// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"database/sql"
	"io/fs"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

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
	if err := up(ctx, pool); err != nil {
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
