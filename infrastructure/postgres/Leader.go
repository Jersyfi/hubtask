// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// LockKey identifies what leadership is being held over.
//
// Advisory locks share one namespace per database, so the numbers are chosen here rather than
// derived from a name: two leaders whose names happened to hash to the same number would look like
// "the other one is always the leader", and nothing in a log would say why.
type LockKey int64

// SchedulerLock is the lock the scheduler role competes for (ADR-0008). The value is arbitrary but
// fixed - changing it while a cluster is running would let an old pod and a new pod both believe
// they are the leader, which is the one state this mechanism exists to rule out.
const SchedulerLock LockKey = 8_010_001

// Leader is leadership as a PostgreSQL advisory lock held on one connection.
//
// # Why this holds a connection, and why that is not a hole in rule 3
//
// Rule 3 says every query goes through the transaction wrapper, because that wrapper is the only
// place that sets the tenant context - a query that got to the database another way would run
// without a tenant and read what it must not. Nothing that happens here is such a query: the two
// statements are pg_try_advisory_lock and pg_advisory_unlock, they take a number, they touch no
// table, and no other code can reach this connection - it is unexported and this type offers no
// way to run anything on it.
//
// It has to be a held connection because a session-level advisory lock belongs to the session that
// took it. A transaction-scoped lock would be released at the next commit, which would mean
// leadership for the length of one tick and a race for it after every one; a lease in a table
// would mean waiting out an expiry after a crash. A lock on a held connection is neither: when the
// process dies, the operating system closes the socket, PostgreSQL releases the lock, and the
// standby has it on its next tick (observability-reliability.md §9).
//
// One connection of the role's pool is therefore occupied for as long as the process is the
// leader. That is the cost, and it is why a scheduler runs with more than one connection.
type Leader struct {
	pool *pgxpool.Pool
	key  LockKey

	// mu guards conn. Acquire and Release are called from the scheduler loop and from the
	// shutdown path, which are two goroutines by construction.
	mu   sync.Mutex
	conn *pgxpool.Conn
}

func NewLeader(pool *pgxpool.Pool, key LockKey) *Leader {
	return &Leader{pool: pool, key: key}
}

var _ queue.Leadership = (*Leader)(nil)

// Acquire tries to become the leader. Being turned down is the normal state of a standby, not an
// error: it returns false and the caller tries again on its next tick.
func (l *Leader) Acquire(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn != nil {
		// Already the leader. Asking the lock again would succeed - an advisory lock is
		// re-entrant within its session - and would then need a second unlock to let go, which is
		// how a former leader keeps a lock nobody can take from it.
		return true, nil
	}

	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.pool_unavailable").
			WithCause(fmt.Errorf("acquiring the leader connection: %w", err))
	}

	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, int64(l.key)).Scan(&acquired); err != nil {
		conn.Release()
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("trying the advisory lock: %w", err))
	}
	if !acquired {
		// Somebody else is the leader. The connection goes back to the pool immediately - a
		// standby that kept one would occupy a connection for doing nothing.
		conn.Release()
		return false, nil
	}

	l.conn = conn
	return true, nil
}

// Confirm reports whether leadership is still held.
//
// It asks the connection rather than trusting the field, because the failure this guards against
// is invisible from inside the process: a cut network, a database that restarted, a connection
// killed by an administrator. The lock went with it, another instance has taken over, and a former
// leader that keeps working is the double execution the lock exists to prevent.
func (l *Leader) Confirm(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn == nil {
		return false, nil
	}
	if l.conn.Conn().IsClosed() {
		l.drop()
		return false, nil
	}

	// The statement is deliberately the cheapest one there is. What is being tested is whether the
	// session is still alive, not what it can do - if it answers at all, it is the same session
	// that took the lock, and the lock is still ours.
	var alive int
	if err := l.conn.QueryRow(ctx, `SELECT 1`).Scan(&alive); err != nil {
		l.drop()
		// Not an error to the caller: losing leadership is a state, and the caller's answer to it
		// is the same as never having had it - stand by and try again.
		return false, nil
	}
	return true, nil
}

// Release gives leadership up, so that a standby can take over at once rather than after the
// connection has timed out. A graceful shutdown calls it; a crash does not need to.
func (l *Leader) Release(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.conn == nil {
		return nil
	}

	_, err := l.conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, int64(l.key))
	l.drop()
	if err != nil {
		// The lock is gone either way - the connection was discarded with it. Reporting the error
		// is still right: it means the shutdown could not tell the database, and the standby waits
		// for the socket to close instead of taking over immediately.
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("releasing the advisory lock: %w", err))
	}
	return nil
}

// drop returns the connection to the pool and forgets it. Called with the mutex held.
//
// A connection that is released after a failed statement is not reused by the pool if the driver
// found it broken, which is what makes losing the lock and losing the connection the same event.
func (l *Leader) drop() {
	l.conn.Release()
	l.conn = nil
}
