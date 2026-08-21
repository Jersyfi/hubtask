// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/Jersyfi/hubtask/test/dbtest"
)

// What the schema itself owes, proven against a real database rather than against schema.sql: that
// the migrations apply, that they apply twice, and that the application role is as narrow as
// multi-tenancy.md §2 says it is.

// A migration has to survive being applied to a database that already has it. goose keeps the
// ledger, but the acceptance criterion is about the rolling update: during one, an old and a new
// pod run against the same database, and the migration job may be retried.
func TestTheMigrationRunsAgainstAnExistingDatabase(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Start(t)
	admin := adminPool(ctx, t)

	var before int
	if err := admin.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version`).Scan(&before); err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}

	if err := dbtest.Migrate(ctx, db.AdminDSN); err != nil {
		t.Fatalf("the second run failed: %v", err)
	}

	var after int
	if err := admin.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version`).Scan(&after); err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}
	if after != before {
		t.Errorf("the second run applied something: version %d became %d", before, after)
	}

	// And the boundary still stands afterwards - a re-run that dropped a policy would be worse
	// than one that failed.
	var unprotected int
	if err := admin.QueryRow(ctx, `
		SELECT count(*) FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind IN ('r','p')
		  AND c.relname NOT IN ('job', 'goose_db_version')
		  AND NOT (c.relrowsecurity AND c.relforcerowsecurity)`).Scan(&unprotected); err != nil {
		t.Fatalf("catalogue query: %v", err)
	}
	if unprotected != 0 {
		t.Errorf("%d tables lost their row level security in the second run", unprotected)
	}
}

// The migrator the image ships is a different program from the goose binary this harness uses to
// prepare the database, and it is the one a self-hoster and every Kubernetes rollout run. It has
// to agree with the harness about what "migrated" means - and it has to be safe to run against a
// database that is already there, because a pre-upgrade hook runs on every deploy.
func TestTheShippedMigratorAppliesNothingToAMigratedDatabase(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Start(t)
	admin := adminPool(ctx, t)

	var before int64
	if err := admin.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version`).Scan(&before); err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}

	root, err := dbtest.RepositoryRoot()
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/migrate", "up")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HUBTASK_DB_DSN="+db.AdminDSN)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the shipped migrator failed: %v: %s", err, out)
	}

	var after int64
	if err := admin.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version`).Scan(&after); err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}
	if after != before {
		t.Errorf("the migrator moved the schema from %d to %d against an already migrated database", before, after)
	}
}

// The migrator's grant step: given the password, it gives hubtask_app its login - and nothing
// else. The role must come out of it exactly as multi-tenancy.md §2.1 demands: able to log in,
// and unable to bypass the boundary.
//
// The grant uses the password the harness already gave the role, so the shared container's other
// tests keep their working DSN whatever order the suite runs in.
func TestTheShippedMigratorGrantsTheApplicationRoleItsLogin(t *testing.T) {
	ctx := context.Background()
	db := dbtest.Start(t)

	root, err := dbtest.RepositoryRoot()
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/migrate", "up")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HUBTASK_DB_DSN="+db.AdminDSN,
		"HUBTASK_DB_APP_PASSWORD="+dbtest.AppPassword)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the migrator with a grant failed: %v: %s", err, out)
	}

	admin := adminPool(ctx, t)
	var canLogin, contained bool
	if err := admin.QueryRow(ctx,
		`SELECT rolcanlogin, NOT rolsuper AND NOT rolbypassrls
		 FROM pg_roles WHERE rolname = 'hubtask_app'`).Scan(&canLogin, &contained); err != nil {
		t.Fatalf("reading the role: %v", err)
	}
	if !canLogin {
		t.Error("the grant did not produce a login")
	}
	if !contained {
		t.Error("the grant handed the application role a way around row level security")
	}

	// And the login it granted is the one the application will use.
	app := appPool(ctx, t)
	var one int
	if err := app.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Errorf("connecting with the granted login failed: %v", err)
	}
}
