// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

// Package dbtest starts the PostgreSQL every suite that needs a real one runs against.
//
// Extracted from test/integration when a second suite needed it (B-10: the retention evidence runs
// the whole deletion path, and it belongs beside the RE catalogue rather than among the repository
// tests). A copy per suite would be two places for the migration step, the role grant and the type
// registration to drift - and the third of those is the one that fails as a scan error nobody
// connects to the setup.
//
// One container per test binary, not per test: starting PostgreSQL per test would cost minutes, and
// the suites keep to their own tenants instead.
package dbtest

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

// AppPassword is the login the tests grant the application role. The migration creates the role
// without one, because a credential has no business being in a migration.
const AppPassword = "test-only-not-a-secret"

type Database struct {
	// adminDSN connects as the superuser: the fixtures need to write across tenants, which is
	// exactly what the application role must not be able to do.
	AdminDSN string
	// appDSN connects as hubtask_app - no BYPASSRLS, not an owner. Everything under test uses
	// this one.
	AppDSN string
}

var (
	sharedDB   Database
	sharedOnce sync.Once
	sharedErr  error
)

// Start returns the container for this test binary, starting it on first use. Starting PostgreSQL per test would
// cost minutes; the tests keep to their own tenants instead.
func Start(t *testing.T) Database {
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

func startDatabase() (Database, error) {
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
		return Database{}, fmt.Errorf("starting the container: %w", err)
	}

	adminDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return Database{}, fmt.Errorf("connection string: %w", err)
	}

	if err := Migrate(ctx, adminDSN); err != nil {
		return Database{}, err
	}

	appDSN, err := grantLoginToApp(ctx, container, adminDSN)
	if err != nil {
		return Database{}, err
	}

	return Database{AdminDSN: adminDSN, AppDSN: appDSN}, nil
}

// Migrate applies db/migrations the way production does - through goose, not by loading
// schema.sql. A migration that only works when applied by a test is not a migration.
func Migrate(ctx context.Context, dsn string) error {
	root, err := RepositoryRoot()
	if err != nil {
		return err
	}
	goose := filepath.Join(root, ".tools", "goose")
	if _, err := os.Stat(goose); err != nil {
		return fmt.Errorf("goose is missing - run 'make tools': %w", err)
	}

	// The binary is the one `make tools` installed, under the repository this file is in, and the
	// arguments are the repository's own migration directory and a DSN this package produced. There
	// is nothing here a caller supplies.
	//nolint:gosec // G204: every argument is derived from the repository, not from a caller
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
		fmt.Sprintf("ALTER ROLE hubtask_app WITH LOGIN PASSWORD %s", quoteLiteral(AppPassword))); err != nil {
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
		AppPassword, host, port.Port()), nil
}

// quoteLiteral is for the one statement that cannot take a placeholder: ALTER ROLE has no
// parameters. The value is a constant in this file, and it is quoted anyway - the habit is what
// keeps rule 9 intact when someone later makes it a variable.
func quoteLiteral(s string) string {
	return "'" + string([]rune(s)) + "'"
}

func RepositoryRoot() (string, error) {
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

// AppPool connects as the application role, the way the server does.
func AppPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	return openPool(ctx, t, Start(t).AppDSN, "application")
}

// AdminPool connects as the superuser, for fixtures and for catalogue queries.
func AdminPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	return openPool(ctx, t, Start(t).AdminDSN, "admin")
}
