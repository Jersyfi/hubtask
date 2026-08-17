// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// Outbox is both halves of the table (ADR-0007): the write path leaves a row in the transaction of
// the change it describes, and the dispatcher takes it from here afterwards.
//
// That split is the point of the pattern - the write path owes nothing but a row, so an
// unreachable webhook target cannot fail a user's request - but the two halves are one adapter
// because they are one table, and a claim that disagreed with the insert about what a row means
// would be a bug nobody could see from either side alone.
type Outbox struct{}

func NewOutbox() Outbox { return Outbox{} }

var (
	_ outbox.Events  = Outbox{}
	_ outbox.Pending = Outbox{}
)

// Append writes the event.
//
// The tenant is not a parameter: it comes from current_tenant_id(), the same value row level
// security compares against, so an event cannot be written into another tenant's stream even
// deliberately.
func (o Outbox) Append(ctx context.Context, envelope event.Envelope) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(envelope.ID)
	if err != nil {
		return err
	}
	actorID, err := optionalUUID(envelope.Actor.ID)
	if err != nil {
		return err
	}
	correlationID, err := optionalUUID(envelope.CorrelationID)
	if err != nil {
		return err
	}
	causationID, err := optionalUUID(envelope.CausationID)
	if err != nil {
		return err
	}

	// The payload is serialised here rather than in the domain: JSON is a wire format, and the
	// domain does not serialise itself (project-structure.md §3).
	payload, err := json.Marshal(envelope.Payload)
	if err != nil {
		return shared.ErrInternal.
			WithDetail("events.payload_unserialisable").
			WithCause(fmt.Errorf("serialising the payload of %s: %w", envelope.Type, err))
	}

	err = queries.AppendOutboxEvent(ctx, sqlc.AppendOutboxEventParams{
		ID:            id,
		EventType:     envelope.Type.String(),
		Subject:       optionalText(envelope.Subject),
		Payload:       payload,
		ActorType:     string(envelope.Actor.Kind),
		ActorID:       actorID,
		CorrelationID: correlationID,
		CausationID:   causationID,
		// The depth is bounded by event.MaxCausationDepth, which NewEnvelope refuses to exceed.
		//nolint:gosec // G115: 0..5 by construction, checked in core/domain/event
		CausationDepth: int32(envelope.CausationDepth),
		OccurredAt:     timestampOf(envelope.OccurredAt),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the outbox event: %w", err))
	}
	return nil
}

// Claim takes up to limit undelivered events of the running tenant and holds them for the length
// of the transaction.
//
// The tenant is not a parameter here either: row level security decides which rows exist, so a
// dispatcher opened for tenant A cannot claim tenant B's events even by asking for them.
func (o Outbox) Claim(ctx context.Context, limit int) ([]event.Envelope, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ClaimPendingEvents(ctx, boundedBatch(limit))
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("claiming pending events: %w", err))
	}

	envelopes := make([]event.Envelope, 0, len(rows))
	for _, row := range rows {
		envelope, err := envelopeFrom(row)
		if err != nil {
			return nil, err
		}
		envelopes = append(envelopes, envelope)
	}
	return envelopes, nil
}

// MarkDispatched records the delivery, in the same transaction as the delivery itself.
func (o Outbox) MarkDispatched(ctx context.Context, ids []shared.ID, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	marked := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		value, err := uuidOf(id)
		if err != nil {
			return err
		}
		marked = append(marked, value)
	}

	if err := queries.MarkEventsDispatched(ctx, sqlc.MarkEventsDispatchedParams{
		DispatchedAt: timestampOf(at),
		Ids:          marked,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("marking %d events as dispatched: %w", len(ids), err))
	}
	return nil
}

// CountPending is how the dispatcher decides between another round now and going back to sleep.
func (o Outbox) CountPending(ctx context.Context) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	pending, err := queries.CountPendingEvents(ctx)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting pending events: %w", err))
	}
	return int(pending), nil
}

// envelopeFrom rebuilds an event from its row.
//
// It builds the struct rather than going through event.NewEnvelope, and that is deliberate. The
// constructor refuses an event type it does not know, which is right on the way in - but on the
// way out it would mean that during a rolling update the old pod dead-letters the events the new
// pod writes (ADR-0003, expand/contract). What is in the table was validated when it was written;
// reading it back is not the place to have second thoughts.
func envelopeFrom(row sqlc.ClaimPendingEventsRow) (event.Envelope, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return event.Envelope{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return event.Envelope{}, err
	}
	actorID, err := optionalID(row.ActorID)
	if err != nil {
		return event.Envelope{}, err
	}
	correlationID, err := optionalID(row.CorrelationID)
	if err != nil {
		return event.Envelope{}, err
	}
	causationID, err := optionalID(row.CausationID)
	if err != nil {
		return event.Envelope{}, err
	}

	payload := map[string]any{}
	if len(row.Payload) > 0 {
		if err := json.Unmarshal(row.Payload, &payload); err != nil {
			return event.Envelope{}, shared.ErrInternal.
				WithDetail("events.payload_unreadable").
				WithCause(fmt.Errorf("reading the payload of event %s: %w", id, err))
		}
	}

	return event.Envelope{
		ID:       id,
		Type:     event.Type(row.EventType),
		TenantID: tenantID,
		Subject:  stringFrom(row.Subject),
		Actor: event.Actor{
			Kind: shared.ActorKind(row.ActorType),
			ID:   actorID,
		},
		OccurredAt:     timeFrom(row.OccurredAt),
		CorrelationID:  correlationID,
		CausationID:    causationID,
		CausationDepth: int(row.CausationDepth),
		Payload:        payload,
	}, nil
}

// Consumption is the record of what a subscriber has already seen (ADR-0007). It lives here rather
// than beside the subscribers because the deduplication has to be in the same transaction as the
// reaction - two places would be two commits, and the gap between them is exactly the duplicate
// the record exists to prevent.
type Consumption struct {
	now clock.Clock
}

func NewConsumption(now clock.Clock) Consumption { return Consumption{now: now} }

var _ eventbus.Consumption = Consumption{}

// Claim records that consumer is about to handle eventID and reports whether it is the first to
// do so. The insert is the question: an insert that changed nothing means somebody already asked.
func (c Consumption) Claim(ctx context.Context, consumer string, eventID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	if consumer == "" {
		return false, shared.ErrInternal.WithDetail("events.consumer_unnamed")
	}

	id, err := uuidOf(eventID)
	if err != nil {
		return false, err
	}

	affected, err := queries.ClaimEventConsumption(ctx, sqlc.ClaimEventConsumptionParams{
		Consumer:   consumer,
		EventID:    id,
		ConsumedAt: timestampOf(c.now.Now()),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("claiming event %s for %s: %w", eventID, consumer, err))
	}
	return affected == 1, nil
}
