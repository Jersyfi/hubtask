// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	dbfiles "github.com/Jersyfi/hubtask/db"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/test/dbtest"
)

// The readiness probe compares the database's migrations with the binary's (deployment.md §5). The
// application role cannot read goose's ledger, and must not; it reads the applied versions through
// applied_schema_versions(), and nothing else.

// readLedger reads the applied migrations as the server does: the application role, inside the
// transaction wrapper.
func readLedger(ctx context.Context, t *testing.T, pool *pgxpool.Pool) (applied []int64, older bool) {
	t.Helper()
	applied, older, err := postgres.SchemaLedger{UnitOfWork: postgres.NewUnitOfWork(pool)}.Applied(ctx)
	if err != nil {
		t.Fatalf("reading the ledger as the application role: %v", err)
	}
	return applied, older
}

func TestTheApplicationRoleReadsEveryAppliedMigration(t *testing.T) {
	ctx := context.Background()
	applied, older := readLedger(ctx, t, appPool(ctx, t))
	if older {
		t.Fatal("a migrated database reads as older than its reader")
	}
	for _, version := range dbfiles.Versions() {
		if !slices.Contains(applied, version) {
			t.Errorf("migration %d is embedded and not among the applied %v", version, applied)
		}
	}
}

// On a database an operator brings - owned by a role that is not a superuser - the function reads
// the ledger the migrator owns; and a database migrated to the version before the function reads
// as older, which readiness takes as behind.
func TestTheLedgerReadsBeforeAndAfterItsReaderOnAManagedDatabase(t *testing.T) {
	ctx := context.Background()
	ownerDSN, appDSN := managedDatabase(ctx, t, "hubtask_managed_ledger")
	if err := dbtest.MigrateTo(ctx, ownerDSN, 143); err != nil {
		t.Fatalf("migrating to the version before the reader: %v", err)
	}
	app, err := pgxpool.New(ctx, appDSN)
	if err != nil {
		t.Fatalf("application pool: %v", err)
	}
	defer app.Close()

	if _, older := readLedger(ctx, t, app); !older {
		t.Error("a database without the reader did not read as older")
	}

	if err := dbtest.Migrate(ctx, ownerDSN); err != nil {
		t.Fatalf("migrating the rest: %v", err)
	}
	applied, older := readLedger(ctx, t, app)
	if older || !slices.Contains(applied, 146) || !slices.Contains(applied, 143) {
		t.Errorf("after the migration: older %v, applied %v", older, applied)
	}
}
