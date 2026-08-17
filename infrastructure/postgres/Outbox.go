// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// Outbox writes domain events into the same transaction as the change they describe (ADR-0007).
//
// It only fills the table. Reading it, delivering, retrying, and the dead letter are the
// dispatcher's half and arrive with A-08 - which is exactly the split the outbox pattern exists
// for: the write path owes nothing but a row, so an unreachable webhook target cannot fail a
// user's request.
type Outbox struct{}

func NewOutbox() Outbox { return Outbox{} }

var _ outbox.Events = Outbox{}

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
