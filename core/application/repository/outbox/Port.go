// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package outbox declares how a domain event leaves the write path.
//
// A transactional outbox (ADR-0007): the event is written to a table in the same transaction as
// the change it describes, and a dispatcher delivers it afterwards. That is what makes "no change
// without its event, and no event without its change" true rather than hoped for - calling a
// webhook from inside the request would leave one of the two half done on any failure.
//
// It is a repository port rather than a bus port for the same reason: what happens here is an
// insert inside somebody else's transaction. The publish/subscribe side - who receives an event
// and how a repeated delivery is made harmless - is core/port/eventbus; what this port owns is the
// table, both the writing of a row and the reading of the ones nobody has delivered yet.
package outbox

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Events records what happened, for delivery later.
type Events interface {
	// Append writes the event inside the caller's transaction. It fails the transaction rather
	// than swallowing the error: an event that was dropped quietly is a change automation and
	// webhooks never hear about, and nothing later can tell that it is missing.
	Append(ctx context.Context, envelope event.Envelope) error
}

// Pending is the dispatcher's side of the same table: what has been written and not yet delivered,
// for the tenant of the running transaction.
//
// There is no way to read across tenants here, and that is not an oversight. Row level security
// stands over outbox_event like over every other table with a tenant column, so a dispatcher works
// one tenant at a time (multi-tenancy.md §2.1) - which is also what keeps one busy tenant from
// filling every batch while the others wait.
type Pending interface {
	// Claim takes up to limit undelivered events, oldest first, and locks them for the duration
	// of the transaction. Rows another dispatcher already holds are skipped rather than waited
	// for: two workers on the same tenant divide the work instead of queueing behind each other.
	Claim(ctx context.Context, limit int) ([]event.Envelope, error)

	// MarkDispatched records the delivery. Called in the same transaction as the delivery itself,
	// so the two cannot come apart - a mark that committed without its delivery would lose the
	// event silently, which is the one failure an outbox exists to rule out.
	MarkDispatched(ctx context.Context, ids []shared.ID, at time.Time) error

	// CountPending reports how much is left for this tenant after a round, which is how the
	// dispatcher decides between running again immediately and going back to sleep.
	CountPending(ctx context.Context) (int, error)
}

// Position is where a poll stands in the outbox: one row's place in the table's own order.
//
// The order is `(occurred_at, id)` - what the dispatcher claims in, and what the polling index is
// built on. A position is therefore a fact about the table rather than a number a process was
// holding, which is what makes a cursor survive a restart and a failover (G-04).
type Position struct {
	OccurredAt time.Time
	ID         shared.ID
}

// Pollable is the outbox as an external trigger reads it: one event type, oldest first, from a
// position (automation.md §3.2).
//
// A port of its own rather than two more methods on Pending, because the readers have nothing to do
// with each other. The dispatcher takes what nobody has delivered and marks it; a poller takes what
// happened, delivered or not, and marks nothing - the pull half is a second transport rather than a
// second delivery, and an interface that held both would let one be handed where the other belongs.
type Pollable interface {
	// Poll answers up to limit events of one type after the position, in the table's order.
	//
	// Horizon is the newest moment a row may carry to be answered. It is the endpoint's guarantee
	// that a cursor never steps over an event: `occurred_at` is stamped by the writing transaction
	// rather than by its commit, so a transaction that began before one already answered can still
	// commit a row sorting behind the cursor. Withholding what is newer than the horizon costs an
	// event a few seconds; answering it would lose the ones still in flight behind it.
	Poll(ctx context.Context, eventType event.Type, after Position, horizon time.Time, limit int) (
		[]event.Envelope, error)
}
