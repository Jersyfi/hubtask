// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package job is the outbound port for background work as a caller sees it (E-01).
//
// It sits beside core/port/queue rather than inside it, and the split is the point: the queue port
// is how a worker claims, leases, retries and finishes work, and this one is how the person who
// asked for it finds out how it went. A single interface would have put the payload and the
// attempt count within reach of a request handler.
package job

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/job"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Jobs reads and stops one tenant's background work.
//
// Neither method takes a tenant, for the reason no repository in this system does: the tenant is
// the transaction's, set by the unit of work from the actor and from nowhere a request could
// reach (ADR-0010, multi-tenancy.md §2.2). The `job` table is the one table without row level
// security, so the adapter states the condition itself rather than a policy stating it - which is
// exactly why a cross-tenant negative test travels with each of these methods.
type Jobs interface {
	// Find answers the job, or ErrNotFound with the same body for a job in another tenant, a job
	// belonging to no tenant, and a job that never existed. Three different truths, one answer:
	// a caller that could tell them apart could enumerate what other tenants are running.
	Find(ctx context.Context, id shared.ID) (domain.Job, error)

	// Cancel moves a queued or running job to CANCELLED and answers it as it now stands.
	//
	// ErrNotFound where Find would say so. ErrConflict where the job reached a terminal state
	// first - including the narrow race where it did so between the read and this write, which is
	// why the state condition is in the statement rather than only in the caller's check.
	Cancel(ctx context.Context, id shared.ID, now time.Time) (domain.Job, error)
}
