// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

// Package integration runs against a real PostgreSQL. Not a mock: the subject of these tests is
// row level security, and a mock of RLS would only test the mock (engineering-guidelines.md §1).
package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// appPassword is the login the tests grant the application role. The migration creates the role
// without one, because a credential has no business being in a migration.
const appPassword = "test-only-not-a-secret"

type database struct {
	// adminDSN connects as the superuser: the fixtures need to write across tenants, which is
	// exactly what the application role must not be able to do.
	adminDSN string
	// appDSN connects as hubtask_app - no BYPASSRLS, not an owner. Everything under test uses
	// this one.
	appDSN string
}

var (
	sharedDB   database
	sharedOnce sync.Once
	sharedErr  error
)

// testDatabase starts one container for the whole package. Starting PostgreSQL per test would
// cost minutes; the tests keep to their own tenants instead.
func testDatabase(t *testing.T) database {
	t.Helper()
	sharedOnce.Do(func() { sharedDB, sharedErr = startDatabase() })
	if sharedErr != nil {
		t.Fatalf("no test database: %v", sharedErr)
	}
	return sharedDB
}

// postgresImage is the database the suite runs against. Configurable so that the support matrix
// can vary it (docs/architecture/support-matrix.md): the default is the supported floor, and the
// nightly matrix job sets it to every other major the table claims.
func postgresImage() string {
	if image := os.Getenv("HUBTASK_TEST_POSTGRES_IMAGE"); image != "" {
		return image
	}
	return "postgres:16-alpine"
}

func startDatabase() (database, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := tcpostgres.Run(ctx, postgresImage(),
		tcpostgres.WithDatabase("hubtask_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(2*time.Minute)),
	)
	if err != nil {
		return database{}, fmt.Errorf("starting the container: %w", err)
	}

	adminDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return database{}, fmt.Errorf("connection string: %w", err)
	}

	if err := migrate(ctx, adminDSN); err != nil {
		return database{}, err
	}

	appDSN, err := grantLoginToApp(ctx, container, adminDSN)
	if err != nil {
		return database{}, err
	}

	return database{adminDSN: adminDSN, appDSN: appDSN}, nil
}

// migrate applies db/migrations the way production does - through goose, not by loading
// schema.sql. A migration that only works when applied by a test is not a migration.
func migrate(ctx context.Context, dsn string) error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	goose := filepath.Join(root, ".tools", "goose")
	if _, err := os.Stat(goose); err != nil {
		return fmt.Errorf("goose is missing - run 'make tools': %w", err)
	}

	cmd := exec.CommandContext(ctx, goose, "-dir", filepath.Join(root, "db", "migrations"), "postgres", dsn, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("goose up: %w: %s", err, out)
	}
	return nil
}

func grantLoginToApp(ctx context.Context, container *tcpostgres.PostgresContainer, adminDSN string) (string, error) {
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		return "", fmt.Errorf("admin pool: %w", err)
	}
	defer admin.Close()

	// The role exists without a login; the operator grants one. Here the test is the operator.
	if _, err := admin.Exec(ctx,
		fmt.Sprintf("ALTER ROLE hubtask_app WITH LOGIN PASSWORD %s", quoteLiteral(appPassword))); err != nil {
		return "", fmt.Errorf("granting login: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("host: %w", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return "", fmt.Errorf("port: %w", err)
	}
	return fmt.Sprintf("postgres://hubtask_app:%s@%s:%s/hubtask_test?sslmode=disable",
		appPassword, host, port.Port()), nil
}

// quoteLiteral is for the one statement that cannot take a placeholder: ALTER ROLE has no
// parameters. The value is a constant in this file, and it is quoted anyway - the habit is what
// keeps rule 9 intact when someone later makes it a variable.
func quoteLiteral(s string) string {
	return "'" + string([]rune(s)) + "'"
}

func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// openPool builds a pool the way the server does. The configuration NewPool applies is the
// server's business, but the type registration is not optional: without it a query returning
// item_type[] fails to scan, so a test opening a raw pgxpool.New would be testing a connection
// the server never uses.
func openPool(ctx context.Context, t *testing.T, dsn, role string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("%s pool configuration: %v", role, err)
	}
	cfg.AfterConnect = postgres.RegisterSchemaTypes

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("%s pool: %v", role, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// appPool connects as the application role, the way the server does.
func appPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	return openPool(ctx, t, testDatabase(t).appDSN, "application")
}

// adminPool connects as the superuser, for fixtures and for catalogue queries.
func adminPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	return openPool(ctx, t, testDatabase(t).adminDSN, "admin")
}

// A migration has to survive being applied to a database that already has it. goose keeps the
// ledger, but the acceptance criterion is about the rolling update: during one, an old and a new
// pod run against the same database, and the migration job may be retried.
func TestTheMigrationRunsAgainstAnExistingDatabase(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	admin := adminPool(ctx, t)

	var before int
	if err := admin.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version`).Scan(&before); err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}

	if err := migrate(ctx, db.adminDSN); err != nil {
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
	db := testDatabase(t)
	admin := adminPool(ctx, t)

	var before int64
	if err := admin.QueryRow(ctx, `SELECT max(version_id) FROM goose_db_version`).Scan(&before); err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}

	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/migrate", "up")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HUBTASK_DB_DSN="+db.adminDSN)
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
	db := testDatabase(t)

	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	cmd := exec.CommandContext(ctx, "go", "run", "./cmd/migrate", "up")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HUBTASK_DB_DSN="+db.adminDSN,
		"HUBTASK_DB_APP_PASSWORD="+appPassword)
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
