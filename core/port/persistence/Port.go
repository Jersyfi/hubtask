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

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Scope is who the work is done for and by. It comes from authentication, never from a request
// body (multi-tenancy.md §2.2).
type Scope struct {
	// TenantID is mandatory. A unit of work without it is refused by the adapter.
	TenantID shared.ID
	// ActorID is the account acting. Empty when the system itself acts - a scheduled job
	// looping over tenants has a tenant but no user.
	ActorID shared.ID
}

// IsValid reports whether the scope can bound a transaction at all.
func (s Scope) IsValid() bool { return !s.TenantID.IsZero() }

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

// ScopeFromContext returns the scope a unit of work was opened with. It exists so that a
// repository can assert what it is operating under rather than trusting a parameter - the
// value is put there by the adapter, not by a caller.
type ScopeSource interface {
	ScopeFromContext(ctx context.Context) (Scope, bool)
}
