// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// The half of ADR-0050 the ordinary gate cannot make.
//
// Every other suite runs against a database that *has* pgvector, which is the decision that lets
// `support-matrix.md` §1 call semantic search supported at all. What that arrangement cannot prove
// is the claim the ADR actually rests on: **an installation without the extension still migrates**.
// Neither `postgres:16-alpine` nor the chart's CloudNativePG image ships it, so an unconditional
// `CREATE EXTENSION` would fail on every installation that upgrades without changing its database
// image - and a failed migration is a stack that will not start.
//
// So this test starts a plain PostgreSQL of its own and migrates it from nothing. It is the
// expensive kind of test that earns its cost once: it is the only place the absence path is
// exercised, and the alternative to running it is finding out from somebody's upgrade.
func TestTheMigrationsApplyWithoutPgvector(t *testing.T) {
	if testing.Short() {
		t.Skip("this test starts a second database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	image := "postgres:16-alpine"
	if override := os.Getenv("HUBTASK_TEST_PLAIN_POSTGRES_IMAGE"); override != "" {
		image = override
	}

	container, err := tcpostgres.Run(ctx, image,
		tcpostgres.WithDatabase("hubtask_plain"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		t.Fatalf("starting a PostgreSQL without pgvector: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	// The whole assertion, in one line: the migrations apply.
	if err := dbtest.Migrate(ctx, dsn); err != nil {
		t.Fatalf("the migrations did not apply to a database without pgvector, which is exactly "+
			"what ADR-0050 exists to prevent: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer pool.Close()

	// And the two consequences, which are what "detected rather than demanded" means in practice.
	var extension bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')`).Scan(&extension); err != nil {
		t.Fatalf("asking for the extension: %v", err)
	}
	if extension {
		t.Fatal("this image has pgvector after all, so the test proved nothing - " +
			"set HUBTASK_TEST_PLAIN_POSTGRES_IMAGE to one without it")
	}

	var store bool
	if err := pool.QueryRow(ctx,
		`SELECT to_regclass('public.item_embedding') IS NOT NULL`).Scan(&store); err != nil {
		t.Fatalf("asking for the store: %v", err)
	}
	if store {
		t.Error("the embedding store exists on a database with no pgvector")
	}

	// The rest of the schema is there, which is the point: what is missing is one optional table
	// and not a feature of the product.
	for _, table := range []string{"work_item", "ai_suggestion", "ai_provider"} {
		var present bool
		if err := pool.QueryRow(ctx,
			`SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&present); err != nil {
			t.Fatalf("asking for %s: %v", table, err)
		}
		if !present {
			t.Errorf("%s is missing, so the migrations did not really finish", table)
		}
	}
}
