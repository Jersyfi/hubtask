// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command migrate applies the database migrations.
//
// It is a program of its own rather than a step in the server's startup, and that is the whole
// point of it: with several replicas, a migration at startup means one migrator per pod and no
// order between them. Here it runs once - as a Compose service the application waits for, and as a
// Helm pre-upgrade hook (deployment.md §2).
//
// Two properties matter more than anything else it does:
//
//   - Only one migrator at a time. A PostgreSQL advisory lock held for the length of the run, so a
//     second one waits rather than interleaves. A retried hook, a rollout started twice, an
//     operator running it by hand during a deploy - all of them wait.
//   - Forward only. goose applies what has not been applied; there is no down. Recovery from a bad
//     deploy is a restore, not a reversal (CLAUDE.md rule 12, ADR-0003).
//
// The migrations travel inside the binary, so the schema a version brings and the code that reads
// it cannot come apart.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	// The pgx driver in its database/sql form: goose speaks database/sql, and a second driver for
	// the same database would be a second set of connection semantics to reason about.
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Jersyfi/hubtask/db"
)

// migrationLock is the advisory lock every migrator competes for. A constant, and a different one
// from the scheduler's (infrastructure/postgres/Leader.go): two locks that shared a number would
// make a migration wait for a scheduler.
const migrationLock int64 = 8_010_002

// timeouts. The lock wait is long because waiting is the correct behaviour during a rollout; the
// run itself is bounded so that a migration blocked on a lock held by a long-running query fails
// visibly rather than hanging a deployment forever.
const (
	lockTimeout = 5 * time.Minute
	runTimeout  = 30 * time.Minute
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := run(os.Args[1:]); err != nil {
		slog.Error("migration failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "up"
	if len(args) > 0 {
		command = args[0]
	}

	dsn, err := dataSource()
	if err != nil {
		return err
	}

	// The DSN carries a password, so nothing about it is logged - not on failure either
	// (security.md §9).
	pool, err := sql.Open("pgx", dsn)
	if err != nil {
		return errors.New("the connection string could not be read")
	}
	defer func() { _ = pool.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	if err := pool.PingContext(ctx); err != nil {
		return fmt.Errorf("the database is unreachable: %w", err)
	}

	goose.SetBaseFS(db.Migrations)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("dialect: %w", err)
	}

	release, err := lock(ctx, pool)
	if err != nil {
		return err
	}
	defer release()

	switch command {
	case "up":
		return up(ctx, pool)
	case "status":
		return status(ctx, pool)
	default:
		return fmt.Errorf("unknown command %q: up or status", command)
	}
}

func up(ctx context.Context, pool *sql.DB) error {
	before, err := goose.GetDBVersionContext(ctx, pool)
	if err != nil {
		return fmt.Errorf("reading the ledger: %w", err)
	}

	if err := goose.UpContext(ctx, pool, "migrations"); err != nil {
		return fmt.Errorf("applying: %w", err)
	}

	after, err := goose.GetDBVersionContext(ctx, pool)
	if err != nil {
		return fmt.Errorf("reading the ledger: %w", err)
	}

	// The unchanged case is not a warning: during a rolling update the hook runs again against a
	// database that is already there, and that is what forward-only migrations are for.
	slog.Info("migrations applied",
		slog.Int64("from", before),
		slog.Int64("to", after),
		slog.Bool("changed", after != before))
	return nil
}

func status(ctx context.Context, pool *sql.DB) error {
	version, err := goose.GetDBVersionContext(ctx, pool)
	if err != nil {
		return fmt.Errorf("reading the ledger: %w", err)
	}
	slog.Info("schema version", slog.Int64("version", version))
	return nil
}

// lock takes the advisory lock on a connection of its own and hands back the release.
//
// The lock lives on that one session for as long as the migration runs. goose does its work on
// other connections of the same pool, which is what makes this a gate in front of the run rather
// than a lock around a single statement.
func lock(ctx context.Context, pool *sql.DB) (func(), error) {
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("connection for the lock: %w", err)
	}

	waiting, cancel := context.WithTimeout(ctx, lockTimeout)
	defer cancel()

	slog.Info("waiting for the migration lock")
	if _, err := conn.ExecContext(waiting, `SELECT pg_advisory_lock($1)`, migrationLock); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("another migration is running and did not finish within %s: %w", lockTimeout, err)
	}

	return func() {
		// The unlock gets a context of its own: the run's may already be spent, and a lock that is
		// only released when the process exits would hold up the next deploy for as long as the
		// connection lingers.
		releasing, cancelRelease := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancelRelease()
		if _, err := conn.ExecContext(releasing, `SELECT pg_advisory_unlock($1)`, migrationLock); err != nil {
			slog.Warn("releasing the migration lock failed", slog.String("error", err.Error()))
		}
		_ = conn.Close()
	}, nil
}

// dataSource reads the one variable this program needs. It fails closed, like every other entry
// point: a migrator without a database is a job that would otherwise report success for doing
// nothing (ADR-0015).
func dataSource() (string, error) {
	if dsn := os.Getenv("HUBTASK_DB_DSN"); dsn != "" {
		return dsn, nil
	}
	// The same file convention as the server, so that Docker and Kubernetes secrets work without
	// the detour through an environment variable.
	if path := os.Getenv("HUBTASK_DB_DSN_FILE"); path != "" {
		content, err := os.ReadFile(path) //nolint:gosec // G304: the path is configuration, and reading it is the point
		if err != nil {
			return "", errors.New("HUBTASK_DB_DSN_FILE points to a file that cannot be read")
		}
		return string(content), nil
	}
	return "", errors.New("HUBTASK_DB_DSN is not set")
}
