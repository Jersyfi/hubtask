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
