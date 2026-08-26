// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package persistence is the port for transactional storage.
//
// The point of this port is the tenant boundary. Every unit of work carries a Scope, and the
// adapter turns that Scope into `SET LOCAL app.tenant_id` in the one place that is allowed to do
// so (ADR-0010, multi-tenancy.md §2.1). There is deliberately no way to obtain a connection
// without a scope: a query that escapes the boundary should be impossible to write, not merely
// forbidden.
//
// Row level security is the backstop underneath, not the mechanism: without a context set, every
// query returns nothing. A programming error therefore reads as "not found" - never as another
// tenant's data.
package persistence

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Scope is who the work is done for and by. It comes from authentication, never from a request
// body (multi-tenancy.md §2.2).
type Scope struct {
	// TenantID is mandatory unless Installation is set. A unit of work without either is refused
	// by the adapter.
	TenantID shared.ID
	// ActorID is the account acting. Empty when the system itself acts - a scheduled job
	// looping over tenants has a tenant but no user.
	ActorID shared.ID
	// Installation marks a read of the rows that belong to no tenant: the system-defined
	// capability profiles, which every tenant may read and none may write
	// (db/schema.sql, item_capability_profile).
	//
	// It is not a way around the boundary but the strictest position inside it. The adapter still
	// sets the tenant context, to the empty value, so `current_tenant_id()` is NULL and every
	// policy comparing against it is false. A row owned by any tenant is therefore invisible -
	// which is more than a tenant scope guarantees, not less.
	Installation bool
	// System is background work that has no tenant yet: a worker reading the job queue does not
	// know whose job the next one is until it has claimed it.
	//
	// It sets the same empty tenant context as an installation scope and is subject to the same
	// row level security, so every table with a tenant column stays invisible and unwritable
	// under it. The one table it can reach is the one the schema deliberately left without a
	// policy - `job`, which is partly tenant-less and readable by worker roles only
	// (db/migrations/0001_init.sql). It is read-write for that table's sake and for no other:
	// a write to a tenant's data under this scope is refused by the database, not by a review.
	System bool
}

// InstallationScope reads what belongs to no tenant. Read-only by construction: the adapter
// refuses a read-write transaction under it, because every WITH CHECK compares against a tenant
// this scope deliberately does not have.
func InstallationScope() Scope { return Scope{Installation: true} }

// SystemScope is the scope of the queue: no tenant, and write access to the one table that has
// none either. Everything it touches beyond that is refused by row level security.
func SystemScope() Scope { return Scope{System: true} }

// IsValid reports whether the scope can bound a transaction at all.
func (s Scope) IsValid() bool { return s.Installation || s.System || !s.TenantID.IsZero() }

// UnitOfWork runs work inside one transaction, bound to one tenant.
//
// The transaction travels in the context: repositories take a context and find it there, which
// is what keeps `Execute(ctx, cmd)` free of a transaction parameter threaded through every
// layer. A repository never opens a transaction of its own (project-structure.md §3).
type UnitOfWork interface {
	// Within runs fn in a read-write transaction. It commits when fn returns nil and rolls
	// back on any error or panic.
	//
	// Nested calls join the running transaction rather than opening a second one: two
	// transactions on two connections would deadlock against each other, and the second would
	// not see the first one's writes.
	Within(ctx context.Context, scope Scope, fn func(ctx context.Context) error) error

	// WithinReadOnly runs fn in a read-only transaction, which may be served by a read replica.
	// Beware replication lag: after a write, read from the primary within the same request
	// (multi-tenancy.md §7).
	WithinReadOnly(ctx context.Context, scope Scope, fn func(ctx context.Context) error) error
}

// Snapshot runs work against one consistent point in time.
//
// A separate interface from UnitOfWork rather than a third method on it, and the reason is
// practical rather than doctrinal: almost nothing needs this. A request reads what is there now,
// and READ COMMITTED is right for that - it sees each statement's own view and holds no snapshot
// across a long call. An export is the exception, and an interface every double in the repository
// has to implement for the sake of one caller is a tax on the other ninety.
//
// What it buys is the guarantee backup-restore.md §5 requires: the archive represents one point in
// time rather than a mixture of before and after. Without it a run that reads containers, then
// items three minutes later, then comments after that produces an archive in which an item belongs
// to a container that does not exist yet - which restores as a foreign key violation on the worst
// possible day.
type Snapshot interface {
	// WithinSnapshot runs fn in a read-only transaction with a REPEATABLE READ snapshot, and
	// tells it when that snapshot was taken.
	//
	// The instant comes from the database rather than from the caller's clock, and that is
	// deliberate: it is what `backup_run.snapshot_at` records, what the manifest carries, and
	// what the next incremental run reads from. One clock for the whole chain, and it is the
	// clock the rows' own timestamps were written by - a process clock a second ahead would
	// produce a chain with a hole in it that nothing would ever report.
	//
	// A snapshot cannot join a transaction that is already running: the isolation level of a
	// transaction is fixed when it begins, so joining would silently give the caller READ
	// COMMITTED under a method that promises otherwise.
	WithinSnapshot(ctx context.Context, scope Scope, fn func(ctx context.Context, at time.Time) error) error
}

// ScopeFromContext returns the scope a unit of work was opened with. It exists so that a
// repository can assert what it is operating under rather than trusting a parameter - the
// value is put there by the adapter, not by a caller.
type ScopeSource interface {
	ScopeFromContext(ctx context.Context) (Scope, bool)
}
