// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"maps"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxCausationDepth is where a chain of events that trigger each other is cut off. An automation
// rule reacting to the event of another rule is legitimate; five levels of it is a loop
// (automation.md §2, run status ABORTED_LOOP).
const MaxCausationDepth = 5

// Actor is who caused the event, in the vocabulary of audit.md §2. The label is deliberately
// absent: an event is delivered to external systems, and a name is personal data those systems do
// not need in order to react (ADR-0018).
type Actor struct {
	Kind shared.ActorKind
	ID   shared.ID
	// OnBehalfOf is the principal an automation or an agent runs as (`run_as`). Empty for a
	// person acting for themselves.
	OnBehalfOf shared.ID
}

// Envelope is one event: what happened, to what, caused by whom, and in which causal chain.
//
// Payload is a map rather than a typed struct per event because the type would have to be
// serialised by an adapter, and a switch over every event type in that adapter is a second place
// to register an event. The shape is not free-form for it: every event type has a JSON schema
// under api/events/, and the contract test judges what the use case produces by it.
type Envelope struct {
	ID       shared.ID
	Type     Type
	TenantID shared.ID
	// Subject is what the event is about, `<entity>/<id>` - the CloudEvents subject, which lets a
	// consumer filter without parsing the payload.
	Subject    string
	Actor      Actor
	OccurredAt time.Time
	// CorrelationID ties everything that came out of one original action together. A root event
	// is its own correlation, so the field is never empty and a consumer never has to special-case
	// the first event of a chain.
	CorrelationID shared.ID
	// CausationID is the event that caused this one. Empty for an action a person took.
	CausationID shared.ID
	// CausationDepth counts how far this event is from that original action; it is what the loop
	// protection reads.
	CausationDepth int
	Payload        map[string]any
}

// Cause is where an event comes from: nothing (a person acted), or another event.
type Cause struct {
	CorrelationID  shared.ID
	CausationID    shared.ID
	CausationDepth int
}

// NewEnvelope builds an event and checks what a consumer relies on.
//
// The identifier and the timestamp are parameters rather than taken here: the domain reads no
// clock and generates no identifier (rule 4), and the event's identifier has to be known before
// the transaction commits so that the row it describes can reference it.
func NewEnvelope(id shared.ID, eventType Type, tenantID shared.ID, subject string,
	actor Actor, occurredAt time.Time, cause Cause, payload map[string]any,
) (Envelope, error) {
	switch {
	case id.IsZero(), tenantID.IsZero(), subject == "", occurredAt.IsZero():
		return Envelope{}, shared.ErrInternal.WithDetail("events.envelope_incomplete")
	case !eventType.Valid():
		return Envelope{}, shared.ErrInternal.
			WithDetail("events.type_unknown").
			WithParams(map[string]string{"type": string(eventType)})
	case cause.CausationDepth < 0 || cause.CausationDepth > MaxCausationDepth:
		// Not a refusal of the event but of the chain: whatever is producing them is looping, and
		// an event emitted here would extend the loop by one (automation.md §2).
		return Envelope{}, shared.ErrConflict.
			WithDetail("events.causation_too_deep").
			WithParams(map[string]string{"maximum": "5"})
	}

	correlationID := cause.CorrelationID
	if correlationID.IsZero() {
		correlationID = id
	}

	return Envelope{
		ID:             id,
		Type:           eventType,
		TenantID:       tenantID,
		Subject:        subject,
		Actor:          actor,
		OccurredAt:     occurredAt.UTC(),
		CorrelationID:  correlationID,
		CausationID:    cause.CausationID,
		CausationDepth: cause.CausationDepth,
		// Copied, so that the caller cannot change a payload that has already been recorded.
		Payload: maps.Clone(payload),
	}, nil
}

// CausedBy is the cause of an event triggered by this one: the same chain, one level deeper.
func (e Envelope) CausedBy() Cause {
	return Cause{
		CorrelationID:  e.CorrelationID,
		CausationID:    e.ID,
		CausationDepth: e.CausationDepth + 1,
	}
}
