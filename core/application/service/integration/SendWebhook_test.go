// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// eventStore answers the events a send may name.
type eventStore struct{ envelopes map[shared.ID]event.Envelope }

func (s eventStore) FindEvent(_ context.Context, id shared.ID) (event.Envelope, error) {
	envelope, found := s.envelopes[id]
	if !found {
		return event.Envelope{}, shared.ErrNotFound.WithDetail("events.event_not_found")
	}
	return envelope, nil
}

func storedEvent(replay bool) eventStore {
	return eventStore{envelopes: map[shared.ID]event.Envelope{
		eventID: {ID: eventID, Type: event.Type(someType), TenantID: tenant, Replay: replay},
	}}
}

// The acceptance criterion: a send delivers through the same pipeline as a subscription - the same
// delivery table, the same job kind, the same retry discipline from there on.
func TestASendQueuesADeliveryOfTheNamedEvent(t *testing.T) {
	h := withSubscription(t)
	queued := &jobs{}

	sent, err := SendWebhook{Writer: h.writer, Jobs: queued, Events: storedEvent(false)}.
		Execute(t.Context(), actor(), hookID, eventID)
	if err != nil {
		t.Fatalf("sending: %v", err)
	}

	if sent.EventID != eventID || sent.SubscriptionID != hookID {
		t.Errorf("the delivery is %+v", sent)
	}
	if sent.Attempt != 1 {
		t.Errorf("attempt = %d, want a first attempt - a send is not a replay", sent.Attempt)
	}
	if stored, err := h.delivered.Find(t.Context(), sent.ID); err != nil || stored.ID != sent.ID {
		t.Errorf("the delivery row was not written: %v", err)
	}
	if len(queued.requests) != 1 {
		t.Fatalf("queued %d jobs, want one", len(queued.requests))
	}
	request := queued.requests[0]
	if request.Kind != queue.KindWebhookDeliver || request.TenantID != tenant {
		t.Errorf("the job is %+v", request)
	}
	if request.Payload["event_id"] != eventID.String() ||
		request.Payload["subscription_id"] != hookID.String() {
		t.Errorf("the job names %v", request.Payload)
	}

	// The act is audited: somebody decided to send this, and the trail says who.
	last := h.sink.entries[len(h.sink.entries)-1]
	if last.Action != WebhookSentAction || last.TargetID != sent.ID {
		t.Errorf("entry = %s on %s", last.Action, last.TargetID)
	}
}

// A paused subscription is somebody's deliberate hold and a disabled one is a target this system
// concluded is unreachable: both refuse rather than queueing a delivery that will fail.
func TestASendToASubscriptionThatIsNotActiveIsRefused(t *testing.T) {
	for _, state := range []domain.SubscriptionState{
		domain.SubscriptionPaused, domain.SubscriptionDisabled,
	} {
		t.Run(string(state), func(t *testing.T) {
			h := withSubscription(t)
			stored := h.store.rows[hookID]
			stored.Subscription.State = state
			h.store.rows[hookID] = stored

			_, err := SendWebhook{Writer: h.writer, Jobs: &jobs{}, Events: storedEvent(false)}.
				Execute(t.Context(), actor(), hookID, eventID)
			if !errors.Is(err, shared.ErrConflict) {
				t.Fatalf("error = %v, want a conflict", err)
			}
			if code := shared.AsError(err).DetailCode; code != "webhooks.subscription_not_active" {
				t.Errorf("detail code %s", code)
			}
		})
	}
}

// backup-restore.md §8.4's first prohibition: an event a restore produced reaches no external
// system, not even when a caller asks for it by name.
func TestAReplayedEventIsNeverSent(t *testing.T) {
	h := withSubscription(t)
	queued := &jobs{}

	_, err := SendWebhook{Writer: h.writer, Jobs: queued, Events: storedEvent(true)}.
		Execute(t.Context(), actor(), hookID, eventID)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error = %v, want a validation refusal", err)
	}
	if code := shared.AsError(err).DetailCode; code != "webhooks.replay_not_deliverable" {
		t.Errorf("detail code %s", code)
	}
	if len(queued.requests) != 0 {
		t.Error("a replayed event was queued for delivery")
	}
}

// An event the sweep has taken cannot be rendered, and the caller is told now rather than through
// a dead letter later.
func TestAnEventThatIsGoneRefusesTheSend(t *testing.T) {
	h := withSubscription(t)

	_, err := SendWebhook{
		Writer: h.writer, Jobs: &jobs{},
		Events: eventStore{envelopes: map[shared.ID]event.Envelope{}},
	}.Execute(t.Context(), actor(), hookID, eventID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("error = %v, want not found", err)
	}
}

// The descriptor reaches the same work, which is what makes SEND_WEBHOOK an action.
func TestASendReachesItsWorkThroughTheDescriptor(t *testing.T) {
	h := withSubscription(t)
	queued := &jobs{}

	descriptor := SendWebhook{Writer: h.writer, Jobs: queued, Events: storedEvent(false)}.Descriptor()
	if descriptor.AutomationAction() != "SEND_WEBHOOK" {
		t.Fatalf("the action kind is %q", descriptor.AutomationAction())
	}

	out, err := descriptor.Handler.Invoke(t.Context(), actor(), map[string]any{
		"subscription_id": hookID.String(),
		"event_id":        eventID.String(),
	})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	if out["event_id"] != eventID.String() {
		t.Errorf("the answer is %v", out)
	}
	if len(queued.requests) != 1 {
		t.Errorf("queued %d jobs through the descriptor", len(queued.requests))
	}
}
