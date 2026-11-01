// SPDX-License-Identifier: Apache-2.0
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
//     deploy is a restore, not a reversal (rule 12, ADR-0003).
//
// The migrations travel inside the binary, so the schema a version brings and the code that reads
// it cannot come apart.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	// The pgx driver in its database/sql form: goose speaks database/sql, and a second driver for
	// the same database would be a second set of connection semantics to reason about.
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Jersyfi/hubtask/db"
)

// migrationLock is the advisory lock every migrator competes for. A constant, and a different one
// from the scheduler's (infrastructure/postgres/Leader.go): two locks that shared a number would
// make a migration wait for a scheduler.
const migrationLock int64 = 8_010_002

// timeouts. The wait for the advisory lock is long because waiting is the correct behaviour during
// a rollout; the run itself is bounded so that a migration that cannot finish fails visibly rather
// than hanging a deployment forever (HUBTASK_MIGRATE_TIMEOUT).
//
// A statement of a transactional migration waits at most tableLockTimeout for a table lock
// (HUBTASK_MIGRATE_LOCK_TIMEOUT). Waiting longer is the outage: PostgreSQL queues every later
// query on the table behind the waiting ALTER, reads included, so a constraint change that takes
// milliseconds would hold the table for as long as some report runs. The file is retried instead,
// lockRetries times with a growing pause. A NO TRANSACTION file - CREATE INDEX CONCURRENTLY, a
// batched backfill - takes locks writes pass, and runs without the bound: interrupted, a
// concurrent build leaves an invalid index, which the next run drops and builds again
// (invalidindex.go) rather than let IF NOT EXISTS keep it.
const (
	lockTimeout       = 5 * time.Minute
	defaultRunTimeout = 30 * time.Minute
	tableLockTimeout  = 5 * time.Second
	lockRetries       = 5
	lockRetryPause    = 2 * time.Second
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
	guarded, err := openGuarded(dsn, durationFrom("HUBTASK_MIGRATE_LOCK_TIMEOUT", tableLockTimeout))
	if err != nil {
		return err
	}
	defer func() { _ = guarded.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), durationFrom("HUBTASK_MIGRATE_TIMEOUT", defaultRunTimeout))
	defer cancel()

	if err := waitForDatabase(ctx, pool, connectWait()); err != nil {
		return err
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
		if err := up(ctx, pool, guarded, migrationFiles(), lockRetryPause); err != nil {
			return err
		}
		return grantAppLogin(ctx, pool)
	case "status":
		return status(ctx, pool)
	default:
		return fmt.Errorf("unknown command %q: up or status", command)
	}
}

// grantAppLogin gives hubtask_app its login, when the operator hands this process the password.
//
// The migration creates the role without one, because a credential has no business being in a
// migration (db/migrations/0001_init.sql) - but without a login the application cannot connect as
// the role row level security was built for, and a reference stack that quietly connected as the
// owner instead is exactly what multi-tenancy.md §2.1 rules out. The migrator is the natural
// operator: it already runs before the application, once per start, as a role that may ALTER ROLE.
//
// Nothing is assembled from strings (rule 9). ALTER ROLE takes no bind parameters, so the
// password travels as a parameterised server-side setting and format(%L) quotes it inside the
// server. Both statements run on one connection, because a setting lives in its session.
func grantAppLogin(ctx context.Context, pool *sql.DB) error {
	password, err := appPassword()
	if err != nil {
		return err
	}
	if password == "" {
		// Not configured. Kubernetes installations manage the role themselves; only the
		// installations that hand the migrator the password get the grant.
		return nil
	}

	conn, err := pool.Conn(ctx)
	if err != nil {
		return fmt.Errorf("connection for the grant: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx,
		`SELECT set_config('hubtask.app_password', $1, false)`, password); err != nil {
		return errors.New("staging the application password failed")
	}
	if _, err := conn.ExecContext(ctx, `
		DO $grant$ BEGIN
			EXECUTE format('ALTER ROLE hubtask_app WITH LOGIN PASSWORD %L',
				current_setting('hubtask.app_password'));
		END $grant$`); err != nil {
		// Deliberately without the driver's message: it can echo the statement, and the
		// statement now carries a credential (T-18).
		return errors.New("granting the application role its login failed")
	}
	// The connection goes back to a pool; the setting must not linger there.
	if _, err := conn.ExecContext(ctx,
		`SELECT set_config('hubtask.app_password', '', false)`); err != nil {
		return errors.New("clearing the staged password failed")
	}

	slog.Info("application role login granted", slog.String("role", "hubtask_app"))
	return nil
}

// appPassword reads the optional grant password, with the same file convention as every other
// secret, so Docker and Kubernetes secrets work without the detour through an environment
// variable.
func appPassword() (string, error) {
	if password := os.Getenv("HUBTASK_DB_APP_PASSWORD"); password != "" {
		return password, nil
	}
	if path := os.Getenv("HUBTASK_DB_APP_PASSWORD_FILE"); path != "" {
		content, err := os.ReadFile(path) //nolint:gosec // G304: the path is configuration, and reading it is the point
		if err != nil {
			return "", errors.New("HUBTASK_DB_APP_PASSWORD_FILE points to a file that cannot be read")
		}
		return strings.TrimSpace(string(content)), nil
	}
	return "", nil
}

func up(ctx context.Context, pool, guarded *sql.DB, files fs.FS, pause time.Duration) error {
	before, err := goose.GetDBVersionContext(ctx, pool)
	if err != nil {
		return fmt.Errorf("reading the ledger: %w", err)
	}
	if err := dropLeftoverBackfillPolicies(ctx, pool); err != nil {
		return err
	}
	if err := repairInvalidIndexes(ctx, pool, files); err != nil {
		return err
	}

	// A migration merged after a higher-numbered one is applied rather than refused. Numbers are
	// taken when a branch is cut and the branches merge in whatever order review finishes them, so
	// a database that took 0091 before 0090 existed is the ordinary outcome of two pull requests,
	// not a corrupted ledger. ADR-0003's migrations are expand-only and assume nothing but the
	// schema they name, and the test beside this proves the case applies. Each pending file runs
	// on its own, so that a transactional one gets the bounded lock wait and a NO TRANSACTION one
	// does not.
	plain, err := goose.NewProvider(goose.DialectPostgres, pool, files, goose.WithAllowOutofOrder(true))
	if err != nil {
		return fmt.Errorf("reading the migrations: %w", err)
	}
	bounded, err := goose.NewProvider(goose.DialectPostgres, guarded, files, goose.WithAllowOutofOrder(true))
	if err != nil {
		return fmt.Errorf("reading the migrations: %w", err)
	}
	states, err := plain.Status(ctx)
	if err != nil {
		return fmt.Errorf("reading the ledger: %w", err)
	}
	for _, state := range states {
		if state.State != goose.StatePending {
			continue
		}
		provider := bounded
		if outsideTransaction(files, state.Source.Path) {
			provider = plain
		}
		if err := applyWithRetry(ctx, pool, provider, state.Source.Version, pause); err != nil {
			return fmt.Errorf("applying %d: %w", state.Source.Version, err)
		}
	}
	if err := refuseInvalidIndexes(ctx, pool, files); err != nil {
		return err
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

// applyWithRetry runs one file, and again after a pause while it gave up waiting for a table lock.
func applyWithRetry(ctx context.Context, pool *sql.DB, provider *goose.Provider, version int64, pause time.Duration) error {
	for attempt := 0; ; attempt++ {
		_, err := provider.ApplyVersion(ctx, version, true)
		if err == nil || !isLockTimeout(err) || attempt >= lockRetries {
			return err
		}
		logBlockers(ctx, pool, version, attempt+1)
		if !sleep(ctx, pause<<attempt) {
			return err
		}
	}
}

// isLockTimeout is SQLSTATE 55P03, lock_not_available.
func isLockTimeout(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "55P03"
}

// logBlockers names the open transactions a retry waited behind: their process, kind and the
// first word of their statement - never the statement, which may quote a row (rule 10).
func logBlockers(ctx context.Context, pool *sql.DB, version int64, attempt int) {
	rows, err := pool.QueryContext(ctx, `
		SELECT pid, backend_type, upper(split_part(ltrim(query), ' ', 1)),
		       extract(epoch FROM now() - xact_start)::bigint
		FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid() AND xact_start IS NOT NULL
		ORDER BY xact_start LIMIT 5`)
	if err != nil {
		slog.Warn("a migration waited too long for a table lock; retrying",
			slog.Int64("version", version), slog.Int("attempt", attempt))
		return
	}
	defer func() { _ = rows.Close() }()
	var blockers []string
	for rows.Next() {
		var (
			pid             int64
			kind, statement string
			seconds         int64
		)
		if rows.Scan(&pid, &kind, &statement, &seconds) == nil {
			blockers = append(blockers, fmt.Sprintf("%d:%s:%s:%ds", pid, kind, statementClass(statement), seconds))
		}
	}
	slog.Warn("a migration waited too long for a table lock; retrying",
		slog.Int64("version", version), slog.Int("attempt", attempt),
		slog.String("open_transactions", strings.Join(blockers, ",")))
}

// statementClass keeps a statement's first word only where it is a keyword's shape.
func statementClass(word string) string {
	for _, r := range word {
		if r < 'A' || r > 'Z' {
			return "OTHER"
		}
	}
	if word == "" {
		return "IDLE"
	}
	return word
}

// dropLeftoverBackfillPolicies removes the policy a batched backfill gives itself (0019, 0212,
// 0228), left behind by a run killed between its batches: the file's next run would otherwise
// fail on CREATE POLICY for ever. Under the advisory lock no backfill is running, so every such
// policy present is a leftover.
func dropLeftoverBackfillPolicies(ctx context.Context, pool *sql.DB) error {
	_, err := pool.ExecContext(ctx, `
		DO $leftover$
		DECLARE p record;
		BEGIN
		  FOR p IN SELECT schemaname, tablename, policyname FROM pg_policies
		           WHERE policyname LIKE '%\_backfill' LOOP
		    EXECUTE format('DROP POLICY IF EXISTS %I ON %I.%I', p.policyname, p.schemaname, p.tablename);
		  END LOOP;
		END $leftover$`)
	if err != nil {
		return fmt.Errorf("dropping a backfill's leftover policy: %w", err)
	}
	return nil
}

// outsideTransaction reports whether a migration file asks to run without a transaction.
func outsideTransaction(files fs.FS, path string) bool {
	content, err := fs.ReadFile(files, path)
	return err == nil && strings.Contains(string(content), "-- +goose NO TRANSACTION")
}

// migrationFiles is the embedded directory, rooted where the files are.
func migrationFiles() fs.FS {
	files, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		return db.Migrations
	}
	return files
}

// openGuarded is a second pool on the same database whose sessions wait at most wait for a lock.
func openGuarded(dsn string, wait time.Duration) (*sql.DB, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("the connection string could not be read")
	}
	config.RuntimeParams["lock_timeout"] = strconv.FormatInt(wait.Milliseconds(), 10)
	return stdlib.OpenDB(*config), nil
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// durationFrom reads a positive duration, or answers the fallback.
func durationFrom(variable string, fallback time.Duration) time.Duration {
	raw := os.Getenv(variable)
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		slog.Warn("not a positive duration - taking the default", slog.String("variable", variable), slog.String("value", raw))
		return fallback
	}
	return value
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

// waitForDatabase keeps pinging until the database answers or the wait is spent.
//
// A migration that starts before its database has finished starting is the ordinary case rather
// than the exception in the one place it matters: a first Kubernetes sync creates the database
// in an earlier wave than this job, and whether the sync waited for the database to be healthy
// is the platform's knowledge, not ours (deployment.md §2.2). Without a wait the job fails, the
// job controller retries with its own backoff, and the release reports a failure for a database
// that was thirty seconds away from answering. The wait is bounded, and zero - the default when
// nothing configures it - keeps the old behaviour of one attempt.
func waitForDatabase(ctx context.Context, pool *sql.DB, wait time.Duration) error {
	const between = 5 * time.Second
	deadline := time.Now().Add(wait)
	for {
		ping, cancel := context.WithTimeout(ctx, between)
		err := pool.PingContext(ping)
		cancel()
		if err == nil {
			return nil
		}
		if wait == 0 || time.Now().After(deadline) {
			return fmt.Errorf("the database is unreachable: %w", err)
		}
		slog.Info("the database is not answering yet - waiting",
			slog.String("giving_up_at", deadline.UTC().Format(time.RFC3339)))
		select {
		case <-ctx.Done():
			return fmt.Errorf("the database is unreachable: %w", ctx.Err())
		case <-time.After(between):
		}
	}
}

// connectWait reads how long waitForDatabase may keep trying. Unset or unparsable means one
// attempt, and an unparsable value is said rather than silently treated as zero.
func connectWait() time.Duration {
	raw := os.Getenv("HUBTASK_DB_CONNECT_WAIT")
	if raw == "" {
		return 0
	}
	wait, err := time.ParseDuration(raw)
	if err != nil || wait < 0 {
		slog.Warn("HUBTASK_DB_CONNECT_WAIT is not a duration - trying once", slog.String("value", raw))
		return 0
	}
	return wait
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
