// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// BusFanOutName is what the deduplication and the logs call this consumer. Stable across versions:
// renaming it would make every event it has already seen look new (core/port/eventbus).
const BusFanOutName = "bus"

// BusFanOut is the outbox's consumer for the optional message bus: one event in, one publish job
// out (H-14, ADR-0007's "and optionally NATS JetStream").
//
// It knows nothing about NATS, and that is the point of it being here rather than in the adapter.
// What it does is the one thing the port allows a subscriber to do with work that has a network on
// the other side: ask the queue for it. Which bus, how it authenticates and what happens when it is
// gone are the adapter's, behind the job.
//
// One job per event and not per subject. There is one bus, so splitting further isolates no
// failure; what the job buys is the retry ladder and the dead letter, which is the same thing the
// webhook delivery buys and for the same reason.
//
// # Why a replay reaches no bus
//
// It implements Subscriber and deliberately not TakesReplays. An external bus receiving a restore's
// events would fire whatever its consumers do - rules, mails, tickets in somebody else's system -
// and that is the first prohibition of backup-restore.md §8.4. The dispatcher enforces it rather
// than trusting this consumer to remember.
//
// # Why it is registered only when the bus is configured
//
// An installation with no bus registers no BusFanOut, so no job is written and no row is paid for.
// Registering it always and letting the handler decide would put a job per event in the queue of
// every installation that does not use this at all, which is the opposite of what "optional" has to
// mean (ADR-0041).
type BusFanOut struct {
	Jobs  Queue
	Clock clock.Clock
}

// Name identifies the consumer in the deduplication.
func (BusFanOut) Name() string { return BusFanOutName }

// Wants reports whether the bus is interested, and it always is.
//
// A bus is a transport rather than a subscriber: what is interesting is the consumer's business,
// expressed as its own subscription on the stream. Filtering here would be this system deciding
// what somebody else's consumers may see, with a redeploy needed to change its mind.
func (BusFanOut) Wants(event.Type) bool { return true }

// Deliver queues the publish.
//
// In the dispatcher's transaction, so the job and the mark that this event was consumed commit
// together: a job that committed without the mark would publish an event that is delivered again
// on the next round, and a mark without the job would be an event nobody ever put on the bus.
func (b BusFanOut) Deliver(ctx context.Context, envelope event.Envelope) error {
	_, err := b.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindBusPublish, TenantID: envelope.TenantID, RunAt: b.Clock.Now(),
		// The identifier and not the event: the handler reads it back at each attempt, so a retry
		// publishes what the first attempt would have rather than a copy that was made once and
		// then aged in a payload column.
		Payload: map[string]any{"event_id": envelope.ID.String()},
	})
	return err
}
