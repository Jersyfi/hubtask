// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// ArrivalProposalsName is how this subscriber is known in the deduplication and in the logs.
// Stable across versions: renaming it makes every event it has already seen look new.
const ArrivalProposalsName = "jumble.arrival_proposals"

// WorkspaceFinder reads the transaction's workspace.
type WorkspaceFinder interface {
	Find(ctx context.Context) (identity.Workspace, error)
}

// ArrivalProposals asks for a proposal about each new jumble entry where the workspace switched
// that on (UC-JUM-07 check 10).
//
// A seed and nothing more, `SeedEmbedding`'s shape: it reads the switch and enqueues, in the
// dispatcher's transaction, so the job and the event's consumption commit together (ADR-0007).
// Whether AI is available, whose content stays out, the day's cap and whether the entry is still
// undecided are the run's to judge - each may change before it runs, and the run is where the
// outcome is written down. One subscriber rather than a line in each door: every door publishes
// the arrival (UC-JUM-08 check 1), and a door added later is covered with nothing to remember.
//
// Not a `TakesReplays` subscriber: an entry a restore wrote back was asked about, or not, when it
// first arrived.
type ArrivalProposals struct {
	Workspaces WorkspaceFinder
	Jobs       Jobs
	Audit      audit.Sink
	Clock      clock.Clock
}

var _ eventbus.Subscriber = ArrivalProposals{}

// Name identifies the subscriber.
func (s ArrivalProposals) Name() string { return ArrivalProposalsName }

// Wants reports whether the event is an arrival.
func (s ArrivalProposals) Wants(eventType event.Type) bool {
	return eventType == event.JumbleEntryReceived
}

// Deliver asks, where the workspace said to.
func (s ArrivalProposals) Deliver(ctx context.Context, envelope event.Envelope) error {
	if !s.Wants(envelope.Type) || envelope.Replay || envelope.TenantID.IsZero() {
		return nil
	}
	workspace, err := s.Workspaces.Find(ctx)
	if err != nil {
		return err
	}
	if !workspace.Settings.AiArrivalProposals {
		return nil
	}
	written, _ := envelope.Payload["id"].(string)
	entryID, err := shared.ParseID(written)
	if err != nil {
		return shared.ErrInternal.WithDetail("events.jumble_entry_without_id")
	}

	// The entry and who made it - account identifiers, never content (rule 10) - because the entry
	// records its submitter but not the principal an agent or a rule acted for, and whose content
	// stays out is judged against them at the run.
	payload := map[string]any{"target_id": entryID.String()}
	if submitter := envelope.Actor; !submitter.ID.IsZero() &&
		(submitter.Kind == shared.ActorUser || submitter.Kind == shared.ActorServiceAccount || submitter.Kind == shared.ActorAIAgent) {
		payload["submitted_by"] = submitter.ID.String()
	}
	if principal := envelope.Actor.OnBehalfOf; !principal.IsZero() {
		payload["on_behalf_of"] = principal.String()
	}
	if _, err := s.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindAiSuggestArrival, TenantID: envelope.TenantID, Payload: payload,
		// One job per arrival: a redelivered event, or one a second dispatcher saw, asks once. The
		// entry's identifier is also how a card finds the job it is waiting on.
		DedupeKey:   entryID.String(),
		MaxAttempts: queue.AiSuggestArrivalAttempts,
	}); err != nil {
		return err
	}

	// The moment a workspace's content is queued to leave for a provider, as a person's ask
	// records it - asked by the system for the workspace, which is what the actor says. Who
	// switched it on is the trail's `ai.arrival_proposals_set`.
	return s.Audit.Append(ctx, audit.Entry{
		TenantID:   envelope.TenantID,
		OccurredAt: s.Clock.Now(),
		Action:     EntrySuggestionAskedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  appshared.ActorSystem,
		TargetType: entryTarget,
		TargetID:   entryID,
	})
}
