// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package eventbus is the port for the delivery half of the outbox (ADR-0007).
//
// The write half is a repository: an event is a row written in the transaction of the change it
// describes (core/application/repository/outbox). What happens afterwards is this port - a
// dispatcher reads those rows and hands each event to every subscriber: the automation engine, the
// webhook delivery, the live stream, the search index.
//
// The delivery guarantee is at-least-once, and the correction for it is here as well. A dispatcher
// that dies between delivering and recording delivers again, and a subscriber that reacted twice
// to one event has sent two emails or created two tasks. Consumers are therefore idempotent, and
// Consumption is the shared way of being idempotent rather than each subscriber inventing its own
// (test RT-4).
package eventbus

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Subscriber reacts to domain events.
//
// A subscriber runs inside the dispatcher's transaction, so what it writes commits with the
// delivery being recorded. That is what makes an in-process consumer exactly-once and it is why a
// subscriber does not call the outside world: everything with a network on the other side becomes
// a job, whose retries are the queue's business rather than a held transaction's
// (observability-reliability.md §8).
type Subscriber interface {
	// Name identifies the subscriber in the deduplication and in the logs. It is stable across
	// versions - renaming one makes every event it has already seen look new.
	Name() string

	// Wants reports whether this event is of interest. Asked before Deliver, so that a
	// subscriber for one event type is not woken by every other.
	Wants(eventType event.Type) bool

	// Deliver reacts. An error stops the whole batch: the events after this one have not been
	// delivered either, and delivering them out of order would break the one ordering guarantee
	// the outbox gives (per aggregate, ADR-0007).
	Deliver(ctx context.Context, envelope event.Envelope) error
}

// TakesReplays is the opt-in for the events a restore wrote (backup-restore.md §8.4).
//
// A restore's changes are real changes and an index that ignored them would be an index that had
// lost a month. They are also, every one of them, a state that is already old, and §8.4 is
// unambiguous about what must not happen to them: no rule fires, no webhook goes out, no
// notification is sent. So the default is not to deliver a replay, and a subscriber that wants
// them says so.
//
// The default is the safe one rather than the convenient one on purpose. A subscriber added in a
// later version by somebody who has never read §8.4 is not delivered a replay by accident; the
// cost of the mistake in the other direction is a stale search index, and the cost in this one is
// four hundred emails about last month.
type TakesReplays interface {
	Subscriber

	// TakesReplays is a marker. It says the subscriber has been read against §8.4 and is one of
	// the consumers a replay is meant for.
	TakesReplays()
}

// WantsReplay reports whether a subscriber may be given a replayed event.
func WantsReplay(subscriber Subscriber) bool {
	_, takes := subscriber.(TakesReplays)
	return takes
}

// Consumption is the record of what a subscriber has already seen.
type Consumption interface {
	// Claim records that consumer is about to handle eventID, and reports whether it is the first
	// to do so. A repeat returns false, and the caller skips the subscriber rather than letting
	// it react again.
	//
	// It is a claim rather than a question because asking and recording separately is a race: two
	// dispatchers would both be told "not seen" before either wrote anything. The insert is the
	// question.
	Claim(ctx context.Context, consumer string, eventID shared.ID) (bool, error)
}

// RetentionWindow is how long a consumption record is kept: long enough that no redelivery this
// system can produce outlives it, short enough that the table does not become the outbox's
// unbounded twin.
//
// Seven days, the same period the dispatched events themselves get. That is the bound that makes
// it safe: a record whose event has been swept can say nothing about an event nobody can deliver
// again, so the two periods are one decision rather than two that could drift apart
// (data-retention.md §3, ADR-0007).
const RetentionWindow = 7 * 24 * time.Hour

// Once runs work for one event and one consumer, exactly once, and reports whether it ran.
//
// This is the library function ADR-0007's third countermeasure names, and the whole of it. Every
// consumer of the stream calls this rather than reimplementing it - the dispatcher today, the
// webhook delivery of G-03 and the rule engine of G-07 next - because the order of the two
// operations is the part that is easy to get wrong, and getting it wrong is invisible until an
// event is acted on twice.
//
// A free function beside the interface, like WantsReplay above and for the same reason: an adapter
// may import a port and may not import a use case (project-structure.md §2), so a helper that
// lived in the application layer would be a helper no consumer could reach.
//
// # Why the claim comes first
//
// It is a write rather than a question. Two dispatchers asking "has this been consumed" would both
// be told no and both proceed; an insert that changed nothing is the answer and the record in one
// statement. That is why the port is called Claim and not Seen.
//
// # Why there is no in-memory cache in front of it
//
// It would be unsafe, and the reason is written down here so that the next person to notice the
// round trip does not add one. The claim lives in the caller's transaction: if that transaction
// rolls back, the claim is undone and the event is correctly redelivered. A process-local memory
// of "I have already done this" would survive the rollback that the claim does not, and would then
// skip the redelivery - losing the event silently, which is the one failure an outbox exists to
// rule out. The round trip is what makes the record and the work commit or fail together.
func Once(
	ctx context.Context, consumed Consumption,
	consumer string, envelope event.Envelope,
	work func(context.Context) error,
) (bool, error) {
	first, err := consumed.Claim(ctx, consumer, envelope.ID)
	if err != nil {
		return false, err
	}
	if !first {
		// Not an error. A repeat is the at-least-once guarantee doing exactly what it says.
		return false, nil
	}
	if err := work(ctx); err != nil {
		return false, err
	}
	return true, nil
}
