// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// EmbeddingSeedName is how this subscriber is known in the deduplication and in the logs. Stable
// across versions, for the reason every consumer name is: renaming it makes every event it has
// already seen look new.
const EmbeddingSeedName = "search.embedding"

// EmbeddingQueue is the slice of the job queue this subscriber needs. Narrow, so what it can do is
// one line: ask for work.
type EmbeddingQueue interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// SeedEmbedding asks for a workspace's vectors to be brought up to date when one of its entries
// changes (J-10).
//
// It is a *seed*, not the work: what it writes is one job saying "this workspace owes embeddings",
// deduplicated per tenant, and the pass behind that job works out which entries actually owe one.
// That is why an event that changed no text is harmless - the pass compares fingerprints and
// finds nothing.
//
// Seeded by the write rather than by a scheduler, like every per-tenant duty in this system:
// nothing may enumerate tenants (multi-tenancy.md §2.1), so nothing can create one job per
// workspace on a timer. A workspace nobody writes in owes nothing and costs nothing.
//
// It runs inside the dispatcher's transaction, which is the reliability argument (ADR-0007): the
// job and the event's consumption commit together, so an event that was seen is an event whose job
// exists.
type SeedEmbedding struct {
	Jobs EmbeddingQueue
	// Delay is how long after the write the pass runs. Short, but not zero: a burst of edits to
	// one workspace should become one pass rather than one per keystroke, and the deduplication
	// key only collapses jobs that are still pending.
	Delay time.Duration
	Clock clock.Clock
}

var _ eventbus.Subscriber = SeedEmbedding{}

// Name identifies the subscriber.
func (s SeedEmbedding) Name() string { return EmbeddingSeedName }

// Wants reports whether this event could have changed an entry's text.
//
// The two that carry a title or notes. A deletion is deliberately absent: the store's rows go with
// the entry by cascade, so nothing has to be asked for. So is everything else an entry can have
// happen to it - a label, an assignment, a move - because none of them changes the words the index
// is built from, and asking for a pass after each would be a provider call for a card somebody
// dragged.
func (s SeedEmbedding) Wants(eventType event.Type) bool {
	return eventType == event.ItemCreated || eventType == event.ItemUpdated
}

// Deliver asks for a pass.
func (s SeedEmbedding) Deliver(ctx context.Context, envelope event.Envelope) error {
	if !s.Wants(envelope.Type) || envelope.TenantID.IsZero() {
		return nil
	}
	_, err := s.Jobs.Enqueue(ctx, queue.Request{
		Kind:     queue.KindAiEmbed,
		TenantID: envelope.TenantID,
		// One pending job per workspace, whatever happened. "This workspace owes embeddings" is
		// one fact however many entries changed, and the pass reads what is owed rather than what
		// the event said.
		DedupeKey: string(queue.KindAiEmbed) + ":" + envelope.TenantID.String(),
		RunAt:     s.Clock.Now().Add(s.Delay),
	})
	return err
}
