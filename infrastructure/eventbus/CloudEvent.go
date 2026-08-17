// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package eventbus renders domain events in the format subscribers read.
//
// CloudEvents 1.0, structured JSON (ADR-0007): the format n8n, Zapier and Knative already
// understand, so an integration is configuration rather than a parser. The delivery itself - the
// dispatcher, the retries, the dead letter - arrives with A-08; what is here is the shape it will
// put on the wire, and the contract test judges it against the schemas under api/events/.
package eventbus

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
)

// ToCloudEvent renders one envelope.
//
// The tenant, the actor and the causal chain travel as CloudEvents extension attributes rather
// than inside `data`: an attribute can be filtered on by a broker and by a webhook subscription
// without parsing the payload, which is what makes "deliver every event of this tenant" a
// routing rule rather than a consumer's problem. Extension names are lower case without
// separators, as the specification requires.
func ToCloudEvent(envelope event.Envelope, source string) map[string]any {
	cloudEvent := map[string]any{
		"specversion":     "1.0",
		"id":              envelope.ID.String(),
		"source":          source,
		"type":            envelope.Type.String(),
		"subject":         envelope.Subject,
		"time":            envelope.OccurredAt.UTC().Format(time.RFC3339Nano),
		"datacontenttype": "application/json",

		"tenantid":       envelope.TenantID.String(),
		"actortype":      string(envelope.Actor.Kind),
		"correlationid":  envelope.CorrelationID.String(),
		"causationdepth": envelope.CausationDepth,

		"data": envelope.Payload,
	}

	// Absent rather than empty: a consumer distinguishes "no causing event" from "an event with
	// an empty identifier", and only one of those is a thing that can happen.
	for name, id := range map[string]string{
		"actorid":     envelope.Actor.ID.String(),
		"onbehalfof":  envelope.Actor.OnBehalfOf.String(),
		"causationid": envelope.CausationID.String(),
	} {
		if id != "" {
			cloudEvent[name] = id
		}
	}
	return cloudEvent
}
