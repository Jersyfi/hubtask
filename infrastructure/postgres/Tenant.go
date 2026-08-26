// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// UnitOfWork is the only place in the system that sets the tenant context (CLAUDE.md rule 3,
// ADR-0010). Everything else reaches the database through a transaction opened here.
//
// SET LOCAL rather than SET: the setting is bound to the transaction and disappears with it, so
// a connection returned to the pool carries no tenant with it. That is also what makes the
// arrangement safe behind pgbouncer in transaction pooling mode (multi-tenancy.md §2.1).
type UnitOfWork struct {
	pool *pgxpool.Pool
}

func NewUnitOfWork(pool *pgxpool.Pool) *UnitOfWork { return &UnitOfWork{pool: pool} }

var (
	_ persistence.UnitOfWork  = (*UnitOfWork)(nil)
	_ persistence.ScopeSource = (*UnitOfWork)(nil)
	_ persistence.Snapshot    = (*UnitOfWork)(nil)
)

// contextKey is unexported, so nothing outside this package can put a transaction into a
// context and pretend it came from here.
type contextKey struct{ name string }

var (
	txKey    = contextKey{"postgres.tx"}
	scopeKey = contextKey{"postgres.scope"}
)

// Within runs fn in a read-write transaction bound to the scope.
func (u *UnitOfWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return u.run(ctx, scope, pgx.ReadWrite, fn)
}

// WithinReadOnly runs fn in a read-only transaction. PostgreSQL enforces the read-only part, so
// a write that slipped into a query path fails loudly rather than quietly succeeding.
func (u *UnitOfWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return u.run(ctx, scope, pgx.ReadOnly, fn)
}

// WithinSnapshot runs fn against one consistent point in time (backup-restore.md §5).
//
// REPEATABLE READ and read-only together. The isolation level is what makes the archive one point
// in time rather than a mixture; read-only is what lets PostgreSQL take the cheaper snapshot and
// what stops an export ever writing. A long-running snapshot holds back vacuum for its duration,
// which is the cost of a consistent backup and the reason §5 puts the run on a bulkhead pool of
// its own rather than on the API path.
func (u *UnitOfWork) WithinSnapshot(
	ctx context.Context, scope persistence.Scope, fn func(context.Context, time.Time) error,
) error {
	if !scope.IsValid() {
		return shared.ErrInternal.WithDetail("postgres.scope_missing")
	}
	// A transaction's isolation level is fixed when it begins, so joining a running one would
	// quietly hand the caller READ COMMITTED under a method that promises a snapshot.
	if _, joined := txFromContext(ctx); joined {
		return shared.ErrInternal.WithDetail("postgres.snapshot_in_transaction")
	}

	tx, err := u.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.begin_failed").
			WithCause(fmt.Errorf("beginning the snapshot: %w", err))
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
		defer cancel()
		// A read-only transaction has nothing to commit. Rolling back is the cheaper end and the
		// one that cannot fail in a way the caller would have to care about.
		_ = tx.Rollback(rollbackCtx)
	}()

	if err := applyScope(ctx, tx, scope); err != nil {
		return err
	}

	// now() inside a transaction is the transaction's start time, which under REPEATABLE READ is
	// the instant the snapshot represents. Taken from the database rather than from this process:
	// it is the clock the rows' own timestamps were written by, and a process clock a second ahead
	// would leave a hole in the chain that nothing would ever report.
	var at time.Time
	if err := tx.QueryRow(ctx, "SELECT now()").Scan(&at); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.snapshot_failed").
			WithCause(fmt.Errorf("reading the snapshot instant: %w", err))
	}

	return fn(withScope(withTx(ctx, tx), scope), at.UTC())
}

func (u *UnitOfWork) run(
	ctx context.Context,
	scope persistence.Scope,
	access pgx.TxAccessMode,
	fn func(context.Context) error,
) error {
	if !scope.IsValid() {
		// Fail closed. Without a tenant, row level security would return empty results and the
		// caller would read that as "nothing there" - a silent wrong answer is worse than an
		// error (multi-tenancy.md §2.2).
		return shared.ErrInternal.WithDetail("postgres.scope_missing")
	}
	if scope.Installation && access != pgx.ReadOnly {
		// An installation scope has no tenant, so every WITH CHECK would compare against NULL
		// and every write would be refused by the database anyway. Refusing here makes it a
		// programming error with a name rather than a confusing policy violation at run time.
		//
		// A system scope passes: it sets the same empty tenant context but is allowed to write,
		// because the one table it exists for has no policy at all (job). For every other table
		// the database gives it the same refusal this branch describes - the difference is who
		// says no, and for the job table nobody has to.
		return shared.ErrInternal.WithDetail("postgres.installation_scope_is_read_only")
	}

	// A transaction already running joins rather than nesting: a second transaction on a second
	// connection would not see the first one's writes, and the two would wait for each other.
	if existing, ok := txFromContext(ctx); ok {
		return u.joinExisting(ctx, existing, scope, fn)
	}

	tx, err := u.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: access})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.begin_failed").
			WithCause(fmt.Errorf("beginning the transaction: %w", err))
	}

	// Rollback on any path that is not an explicit commit, panic included. A panic that left a
	// transaction open would hold its locks until the connection expired.
	committed := false
	defer func() {
		if !committed {
			// The context may already be cancelled, and a rollback still has to reach the
			// server, so it gets a context of its own.
			rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
			defer cancel()
			_ = tx.Rollback(rollbackCtx)
		}
	}()

	if err := applyScope(ctx, tx, scope); err != nil {
		return err
	}

	if err := fn(withScope(withTx(ctx, tx), scope)); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.commit_failed").
			WithCause(fmt.Errorf("committing: %w", err))
	}
	committed = true
	return nil
}

// joinExisting continues in the running transaction. Changing tenant mid-transaction is refused:
// the SET LOCAL of the outer scope is still in force, so the inner work would silently run under
// the wrong tenant - which is the exact failure this port exists to prevent.
func (u *UnitOfWork) joinExisting(
	ctx context.Context,
	tx pgx.Tx,
	scope persistence.Scope,
	fn func(context.Context) error,
) error {
	outer, ok := scopeFromContext(ctx)
	if ok && outer.TenantID != scope.TenantID {
		return shared.ErrInternal.
			WithDetail("postgres.tenant_switch_in_transaction").
			WithParams(map[string]string{"outer": outer.TenantID.String(), "inner": scope.TenantID.String()})
	}
	return fn(withScope(withTx(ctx, tx), scope))
}

// rollbackTimeout bounds the rollback of a transaction whose context is already gone. Short:
// the work is lost either way, and holding the connection helps nobody.
const rollbackTimeout = 5 * time.Second

// applyScope is the SET LOCAL. Parameters are bound rather than interpolated - a tenant
// identifier is data, and SQL is never assembled from data (CLAUDE.md rule 9).
//
// set_config with is_local = true is the parameterised form of SET LOCAL; SET LOCAL itself takes
// no placeholders.
// An installation scope sets the tenant to the empty string rather than skipping the call.
// current_tenant_id() reads that as NULL, so every policy comparing against it is false and only
// the rows that belong to no tenant remain visible. Skipping the call instead would leave
// whatever the connection last carried, which is the one outcome this port exists to prevent.
func applyScope(ctx context.Context, tx pgx.Tx, scope persistence.Scope) error {
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, scope.TenantID.String()); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.tenant_context_failed").
			WithCause(fmt.Errorf("setting the tenant context: %w", err))
	}
	// The actor is for the audit trigger and for pg_stat_activity, not for isolation. Absent
	// when the system acts, and an empty string is the honest representation of that.
	if _, err := tx.Exec(ctx, `SELECT set_config('app.actor_id', $1, true)`, scope.ActorID.String()); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.actor_context_failed").
			WithCause(fmt.Errorf("setting the actor context: %w", err))
	}
	return nil
}

func withTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey, tx)
}

func withScope(ctx context.Context, scope persistence.Scope) context.Context {
	return context.WithValue(ctx, scopeKey, scope)
}

func txFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey).(pgx.Tx)
	return tx, ok
}

// ScopeFromContext reports the scope the running transaction was opened with. A repository uses
// it to state what it is operating under; the value is put there by this package, never by a
// caller.
func (u *UnitOfWork) ScopeFromContext(ctx context.Context) (persistence.Scope, bool) {
	return scopeFromContext(ctx)
}

func scopeFromContext(ctx context.Context) (persistence.Scope, bool) {
	scope, ok := ctx.Value(scopeKey).(persistence.Scope)
	return scope, ok
}

// FromContext returns the transaction to a repository. The error is deliberate: a repository
// called outside a unit of work is a programming error, and it must not fall back to the pool.
func FromContext(ctx context.Context) (pgx.Tx, error) {
	tx, ok := txFromContext(ctx)
	if !ok {
		return nil, shared.ErrInternal.WithDetail("postgres.no_transaction_in_context")
	}
	return tx, nil
}

// ErrNoRows lets the application layer recognise an empty result without importing the driver.
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
