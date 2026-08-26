// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build resilience

package resilience

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// The RT tests that need a database use a real one, for the same reason the integration suite
// does: what they are about - a lease that expires, a transaction that rolls back when a process
// dies - is behaviour of PostgreSQL, and a fake of it would only test the fake.
//
// Everything here reaches the database the way the application does, through
// infrastructure/postgres. Nothing in this package holds the driver: the tenant boundary is
// test/integration's subject, and this one has no fixtures the adapters cannot create.

const appPassword = "test-only-not-a-secret"

var (
	sharedDSN string
	// sharedContainer is kept for one purpose: RT-10 needs a tenant and an account before it can
	// have a series, and creating a tenant is the control plane's act rather than the
	// application's (db/migrations/0001_init.sql). It is done with psql *inside* the container, so
	// that this package still holds no database driver - the tenant boundary is proved by going
	// through the application role, and a package that could open its own connection could quietly
	// stop doing that.
	sharedContainer *tcpostgres.PostgresContainer
	sharedOnce      sync.Once
	sharedErr       error
)

// testDSN starts one container for the whole package and returns the application role's DSN.
func testDSN(t *testing.T) string {
	t.Helper()
	sharedOnce.Do(func() { sharedDSN, sharedContainer, sharedErr = startDatabase() })
	if sharedErr != nil {
		t.Fatalf("no test database: %v", sharedErr)
	}
	return sharedDSN
}

// execAsOwner runs one statement as the database owner, inside the container. The one fixture that
// needs it is the tenant and the account behind it; everything else in this package goes through
// the application role and its transaction wrapper.
func execAsOwner(ctx context.Context, t *testing.T, statement string) {
	t.Helper()
	testDSN(t)

	code, reader, err := sharedContainer.Exec(ctx, []string{
		"psql", "-U", "postgres", "-d", "hubtask_test", "-v", "ON_ERROR_STOP=1", "-c", statement,
	})
	if err != nil {
		t.Fatalf("running the fixture statement: %v", err)
	}
	if code != 0 {
		out, _ := io.ReadAll(reader)
		t.Fatalf("the fixture statement failed (%d): %s", code, out)
	}
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

func startDatabase() (appDSN string, running *tcpostgres.PostgresContainer, err error) {
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
		return "", nil, fmt.Errorf("starting the container: %w", err)
	}

	adminDSN, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return "", nil, fmt.Errorf("connection string: %w", err)
	}
	if err := migrate(ctx, adminDSN); err != nil {
		return "", nil, err
	}

	// The migration creates the application role without a login; granting one is the operator's
	// job, and here the test is the operator. It runs inside the container, so this package needs
	// no database driver of its own.
	if _, _, err := container.Exec(ctx, []string{
		"psql", "-U", "postgres", "-d", "hubtask_test", "-c",
		"ALTER ROLE hubtask_app WITH LOGIN PASSWORD '" + appPassword + "'",
	}); err != nil {
		return "", nil, fmt.Errorf("granting login: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("host: %w", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return "", nil, fmt.Errorf("port: %w", err)
	}
	return fmt.Sprintf("postgres://hubtask_app:%s@%s:%s/hubtask_test?sslmode=disable",
		appPassword, host, port.Port()), container, nil
}

// migrate applies db/migrations through goose, the way production does.
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
