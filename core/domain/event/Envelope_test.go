// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	eventID    = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	tenantID   = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	hubID      = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	collection = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	accountID  = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	occurredAt = time.Date(2026, 8, 17, 9, 0, 0, 0, time.UTC)
	actor      = event.Actor{Kind: "USER", ID: accountID}
)

func hub() work.Container {
	return work.Container{
		ID: hubID, TenantID: tenantID, Type: work.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: occurredAt, Version: 1,
	}
}

// A root event is its own correlation, so a consumer never has to special-case the first event of
// a chain.
func TestARootEventCorrelatesWithItself(t *testing.T) {
	envelope, err := event.NewEnvelope(eventID, event.ContainerCreated, tenantID, "container/x",
		actor, occurredAt, event.Cause{}, map[string]any{"id": "x"})
	if err != nil {
		t.Fatalf("the envelope was refused: %v", err)
	}

	if envelope.CorrelationID != eventID || !envelope.CausationID.IsZero() || envelope.CausationDepth != 0 {
		t.Errorf("unexpected causal chain: %+v", envelope)
	}
}

// The chain an event triggered by this one carries: same correlation, one level deeper.
func TestCausedByContinuesTheChain(t *testing.T) {
	first, err := event.NewEnvelope(eventID, event.ContainerCreated, tenantID, "container/x",
		actor, occurredAt, event.Cause{}, nil)
	if err != nil {
		t.Fatalf("the envelope was refused: %v", err)
	}

	next := first.CausedBy()
	if next.CorrelationID != first.CorrelationID || next.CausationID != first.ID || next.CausationDepth != 1 {
		t.Errorf("the chain does not continue: %+v", next)
	}
}

// The loop protection of automation.md §2, one level below the rule engine: an event at the limit
// cannot be built, so a chain cannot extend itself past it even if a rule tried.
func TestTheChainIsCutOffAtTheLimit(t *testing.T) {
	deep := event.Cause{CorrelationID: eventID, CausationID: eventID, CausationDepth: event.MaxCausationDepth + 1}

	_, err := event.NewEnvelope(eventID, event.ContainerCreated, tenantID, "container/x",
		actor, occurredAt, deep, nil)
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("error %v, want a conflict", err)
	}
	if got := shared.AsError(err).DetailCode; got != "events.causation_too_deep" {
		t.Errorf("detail code %s, want events.causation_too_deep", got)
	}

	atTheLimit := event.Cause{CausationDepth: event.MaxCausationDepth}
	if _, err := event.NewEnvelope(eventID, event.ContainerCreated, tenantID, "container/x",
		actor, occurredAt, atTheLimit, nil); err != nil {
		t.Errorf("the last permitted level was refused: %v", err)
	}
}

func TestNewEnvelopeRefusesWhatAConsumerReliesOn(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(*event.Envelope)
		detailCode string
	}{
		{name: "without an identifier", mutate: func(e *event.Envelope) { e.ID = "" }, detailCode: "events.envelope_incomplete"},
		{name: "without a tenant", mutate: func(e *event.Envelope) { e.TenantID = "" }, detailCode: "events.envelope_incomplete"},
		{name: "without a subject", mutate: func(e *event.Envelope) { e.Subject = "" }, detailCode: "events.envelope_incomplete"},
		{name: "without a time", mutate: func(e *event.Envelope) { e.OccurredAt = time.Time{} }, detailCode: "events.envelope_incomplete"},
		{name: "with an unknown type", mutate: func(e *event.Envelope) { e.Type = "de.hubtask.work.container.invented.v1" }, detailCode: "events.type_unknown"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			complete := event.Envelope{
				ID: eventID, Type: event.ContainerCreated, TenantID: tenantID,
				Subject: "container/x", OccurredAt: occurredAt,
			}
			c.mutate(&complete)

			_, err := event.NewEnvelope(complete.ID, complete.Type, complete.TenantID,
				complete.Subject, actor, complete.OccurredAt, event.Cause{}, nil)
			if err == nil {
				t.Fatalf("no error, want %s", c.detailCode)
			}
			if got := shared.AsError(err).DetailCode; got != c.detailCode {
				t.Errorf("detail code %s, want %s", got, c.detailCode)
			}
		})
	}
}

// The payload is copied on the way in: an event that has been recorded must not change because
// the caller reused its map.
func TestThePayloadIsCopied(t *testing.T) {
	payload := map[string]any{"name": "Private"}

	envelope, err := event.NewEnvelope(eventID, event.ContainerCreated, tenantID, "container/x",
		actor, occurredAt, event.Cause{}, payload)
	if err != nil {
		t.Fatalf("the envelope was refused: %v", err)
	}

	payload["name"] = "Renamed"
	if envelope.Payload["name"] != "Private" {
		t.Error("the recorded payload changed under the event")
	}
}

func TestContainerCreatedCarriesTheSnapshot(t *testing.T) {
	t.Run("a hub", func(t *testing.T) {
		envelope, err := event.NewContainerCreated(eventID, hub(), actor, occurredAt, event.Cause{})
		if err != nil {
			t.Fatalf("the event was refused: %v", err)
		}

		if envelope.Type != event.ContainerCreated || envelope.Subject != "container/"+hubID.String() {
			t.Errorf("unexpected envelope: %+v", envelope)
		}
		if envelope.Payload["parent_id"] != nil {
			t.Errorf("a hub reports a parent: %v", envelope.Payload["parent_id"])
		}
		for field, want := range map[string]any{
			"id": hubID.String(), "type": "HUB", "name": "Private",
			"order_key": "a0", "created_by": accountID.String(), "version": 1,
		} {
			if envelope.Payload[field] != want {
				t.Errorf("%s is %v, want %v", field, envelope.Payload[field], want)
			}
		}
	})

	t.Run("a collection reports its hub", func(t *testing.T) {
		container := hub()
		container.ID, container.Type, container.ParentID = collection, work.ContainerCollection, hubID
		container.Description, container.Icon, container.ColorToken = "Weekly", "cart", "blue"

		envelope, err := event.NewContainerCreated(eventID, container, actor, occurredAt, event.Cause{})
		if err != nil {
			t.Fatalf("the event was refused: %v", err)
		}
		if envelope.Payload["parent_id"] != hubID.String() {
			t.Errorf("parent_id is %v, want the hub", envelope.Payload["parent_id"])
		}
		for field, want := range map[string]any{"description": "Weekly", "icon": "cart", "color_token": "blue"} {
			if envelope.Payload[field] != want {
				t.Errorf("%s is %v, want %v", field, envelope.Payload[field], want)
			}
		}
	})

	// An optional field that is not set is left out rather than sent as null: a client tolerates
	// unknown fields, and an absent one says "not set" with fewer bytes.
	t.Run("unset optional fields are absent", func(t *testing.T) {
		envelope, err := event.NewContainerCreated(eventID, hub(), actor, occurredAt, event.Cause{})
		if err != nil {
			t.Fatalf("the event was refused: %v", err)
		}
		for _, field := range []string{"description", "icon", "color_token"} {
			if _, present := envelope.Payload[field]; present {
				t.Errorf("%s is present although it was never set", field)
			}
		}
	})
}

// Every type in the set is a type the constructor accepts, and nothing else is.
func TestTheEventTypesAreAClosedSet(t *testing.T) {
	if len(event.Types()) == 0 {
		t.Fatal("no event types at all")
	}
	for _, eventType := range event.Types() {
		if !eventType.Valid() {
			t.Errorf("%s is in the set and not valid", eventType)
		}
	}
	if event.Type("de.hubtask.work.container.created.v2").Valid() {
		t.Error("a version that does not exist is accepted")
	}
}
