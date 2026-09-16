// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

type disableNotices struct {
	recipients []shared.ID
	err        error
}

func (n *disableNotices) WebhookDisabled(_ context.Context, _, recipient, _ shared.ID) error {
	if n.err != nil {
		return n.err
	}
	n.recipients = append(n.recipients, recipient)
	return nil
}

func anEvent(t *testing.T, eventType event.Type) event.Envelope {
	t.Helper()
	envelope, err := event.NewEnvelope(
		shared.ID("01936f2a-7c1e-7000-8000-000000000f21"), eventType, tenant,
		"item/01936f2a-7c1e-7000-8000-000000000f22",
		event.Actor{Kind: shared.ActorUser, ID: author}, now, event.Cause{}, map[string]any{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

func fanOut(h *harness, queued *jobs) FanOut {
	return FanOut{
		Subscriptions: h.store, Deliveries: h.delivered, Jobs: queued,
		Clock: h.writer.Clock, IDs: ids{next: deliveryID},
	}
}

// One event in, a delivery job per interested subscription out. The fan-out does not deliver: it
// records what is owed and asks the queue for it, which is what keeps one unreachable target from
// holding up every other subscriber's events.
func TestAnEventBecomesADeliveryJobPerInterestedSubscription(t *testing.T) {
	h := withSubscription(t)
	queued := &jobs{}

	if err := fanOut(h, queued).Deliver(t.Context(), anEvent(t, event.ItemCreated)); err != nil {
		t.Fatalf("fanning out: %v", err)
	}

	if len(h.delivered.rows) != 1 {
		t.Fatalf("recorded %d deliveries, want one", len(h.delivered.rows))
	}
	delivery := h.delivered.rows[0]
	if delivery.Status != domain.DeliveryPending || delivery.Attempt != 1 {
		t.Errorf("delivery = %+v", delivery)
	}
	if len(queued.requests) != 1 || queued.requests[0].Kind != queue.KindWebhookDeliver {
		t.Fatalf("queued %+v", queued.requests)
	}
	if queued.requests[0].Payload["delivery_id"] != delivery.ID.String() {
		t.Error("the job does not name the delivery it is for")
	}
}

// The events of one push that name the same subscription, subject and type owe one delivery
// for the push, with the last event's payload (N-10, offline-sync.md §8): the pending delivery
// is repointed rather than a second one recorded, and its job waits the grace so that the push's
// events fold before the first attempt. A delivery already attempted stands for what it sent.
// An event with no push in its cause collapses with nothing.
func TestTheEventsOfOnePushCollapseToOneDelivery(t *testing.T) {
	h := withSubscription(t)
	queued := &jobs{}
	push := shared.ID("01936f2a-7c1e-7000-8000-000000000f31")
	first := anEvent(t, event.ItemCreated).Pushed(push, now.Add(-time.Hour))
	second := first
	second.ID = shared.ID("01936f2a-7c1e-7000-8000-000000000f32")
	fan := fanOut(h, queued)

	for _, envelope := range []event.Envelope{first, second} {
		if err := fan.Deliver(t.Context(), envelope); err != nil {
			t.Fatalf("fanning out: %v", err)
		}
	}
	if len(h.delivered.rows) != 1 || h.delivered.rows[0].EventID != second.ID {
		t.Fatalf("recorded %+v, want one delivery standing for the second event", h.delivered.rows)
	}
	if !h.delivered.rows[0].Collapse.Collapses() || h.delivered.rows[0].Collapse.PushID != push {
		t.Errorf("the delivery carries %+v, want the push's key", h.delivered.rows[0].Collapse)
	}
	if len(queued.requests) != 1 || !queued.requests[0].RunAt.Equal(now.Add(DefaultCollapseGrace)) {
		t.Errorf("queued %+v, want one job after the grace", queued.requests)
	}

	// Another subject of the same push is its own delivery; an attempt already made is not
	// repointed, and the next event gets a delivery of its own.
	other := first
	other.ID, other.Subject = shared.ID("01936f2a-7c1e-7000-8000-000000000f33"), "item/01936f2a-7c1e-7000-8000-000000000f34"
	if err := fan.Deliver(t.Context(), other); err != nil {
		t.Fatalf("fanning out another subject: %v", err)
	}
	h.delivered.rows[0].Status = domain.DeliverySucceeded
	third := first
	third.ID = shared.ID("01936f2a-7c1e-7000-8000-000000000f35")
	if err := fan.Deliver(t.Context(), third); err != nil {
		t.Fatalf("fanning out after the attempt: %v", err)
	}
	if len(h.delivered.rows) != 3 {
		t.Errorf("recorded %d deliveries, want the push's, the other subject's and the one after the attempt", len(h.delivered.rows))
	}

	// Two online events about the same subject stay two deliveries, at once.
	h, queued = withSubscription(t), &jobs{}
	online := anEvent(t, event.ItemCreated)
	again := online
	again.ID = shared.ID("01936f2a-7c1e-7000-8000-000000000f36")
	fan = fanOut(h, queued)
	for _, envelope := range []event.Envelope{online, again} {
		if err := fan.Deliver(t.Context(), envelope); err != nil {
			t.Fatalf("fanning out online: %v", err)
		}
	}
	if len(h.delivered.rows) != 2 || h.delivered.rows[0].Collapse.Collapses() {
		t.Errorf("online events recorded %+v, want two deliveries with no key", h.delivered.rows)
	}
	if !queued.requests[0].RunAt.Equal(now) {
		t.Errorf("an online delivery waits: %v", queued.requests[0].RunAt)
	}
}

// A subscription that did not ask for this type, and one that is not active, both get nothing.
func TestOnlyAnActiveSubscriptionThatAskedForItIsDelivered(t *testing.T) {
	h := withSubscription(t)
	queued := &jobs{}

	if err := fanOut(h, queued).Deliver(t.Context(), anEvent(t, event.ItemCompleted)); err != nil {
		t.Fatalf("fanning out an unsubscribed type: %v", err)
	}
	if len(queued.requests) != 0 {
		t.Errorf("a type nobody subscribed to produced %d jobs", len(queued.requests))
	}

	paused := h.store.rows[hookID]
	paused.Subscription = paused.Subscription.Paused()
	h.store.rows[hookID] = paused

	if err := fanOut(h, queued).Deliver(t.Context(), anEvent(t, event.ItemCreated)); err != nil {
		t.Fatalf("fanning out to a paused subscription: %v", err)
	}
	if len(queued.requests) != 0 {
		t.Errorf("a paused subscription produced %d jobs", len(queued.requests))
	}
}

// A restore would otherwise report last month's states to every external system that has ever
// subscribed, which is the first prohibition of backup-restore.md §8.4. The dispatcher enforces it
// from this declaration rather than trusting the consumer to remember.
func TestTheFanOutDoesNotTakeReplays(t *testing.T) {
	if eventbus.WantsReplay(FanOut{}) {
		t.Fatal("the webhook fan-out asked for replays; a restore would reach every subscriber")
	}
	// And it is a subscriber at all, which is what puts it on the stream.
	var _ eventbus.Subscriber = FanOut{}
	if (FanOut{}).Name() != FanOutName {
		t.Error("the consumer name is not the stable one the deduplication keys on")
	}
}

// Only a delivery that has stopped counts. Counting attempts would disable a subscription after
// three retries of a single event - a target briefly unreachable rather than one that is gone.
func TestOnlyADeadLetteredDeliveryCountsAgainstTheSubscription(t *testing.T) {
	h := withSubscription(t)
	notices := &disableNotices{}
	outcomes := Outcomes{
		Subscriptions: h.store, Audit: h.sink, Notifier: notices, Clock: h.writer.Clock,
	}

	for range 10 {
		if err := outcomes.Failed(t.Context(), hookID, "webhooks.target_unreachable", false); err != nil {
			t.Fatalf("recording a retryable failure: %v", err)
		}
	}
	if h.store.rows[hookID].Subscription.FailureCount != 0 {
		t.Fatalf("retryable attempts counted against the subscription: %+v", h.store.rows[hookID].Subscription)
	}
	if len(notices.recipients) != 0 {
		t.Error("a retryable attempt notified the owner")
	}
}

// Auto-disable, its trail entry and the notification to the owner - one sentence of
// automation.md §3.1, and three things that have to happen together.
func TestARunOfDeadLettersDisablesAndTellsTheOwner(t *testing.T) {
	h := withSubscription(t)
	notices := &disableNotices{}
	outcomes := Outcomes{
		Subscriptions: h.store, Audit: h.sink, Notifier: notices, Clock: h.writer.Clock,
	}
	entriesBefore := len(h.sink.entries)

	for range domain.MaxConsecutiveFailures {
		if err := outcomes.Failed(t.Context(), hookID, "webhooks.target_unreachable", true); err != nil {
			t.Fatalf("recording a dead letter: %v", err)
		}
	}

	subscription := h.store.rows[hookID].Subscription
	if !subscription.IsDisabled() || subscription.DisabledAt.IsZero() {
		t.Fatalf("the run did not disable the subscription: %+v", subscription)
	}
	if len(notices.recipients) != 1 || notices.recipients[0] != author {
		t.Errorf("notified %v, want the owner once", notices.recipients)
	}

	entry := h.sink.entries[len(h.sink.entries)-1]
	if len(h.sink.entries) != entriesBefore+1 {
		t.Errorf("wrote %d entries, want one for the disabling", len(h.sink.entries)-entriesBefore)
	}
	if entry.Action != WebhookDisabledAction {
		t.Errorf("entry = %s, want the disabling", entry.Action)
	}
	// The system rather than a person: nobody decided this, and an entry naming the last person to
	// touch the subscription would be saying they did.
	if entry.ActorKind != shared.ActorSystem || !entry.ActorID.IsZero() {
		t.Errorf("the disabling is attributed to %s/%s", entry.ActorKind, entry.ActorID)
	}

	// And a further failure does not notify again: the owner is told once.
	if err := outcomes.Failed(t.Context(), hookID, "webhooks.target_unreachable", true); err != nil {
		t.Fatalf("recording a failure after the disabling: %v", err)
	}
	if len(notices.recipients) != 1 {
		t.Errorf("the owner was notified %d times", len(notices.recipients))
	}
}

// A success ends the run, and costs no write when there is nothing to clear - otherwise a healthy
// subscription would be a row update per event.
func TestASuccessClearsTheRunAndCostsNothingWhenThereIsNoRun(t *testing.T) {
	h := withSubscription(t)
	outcomes := Outcomes{Subscriptions: h.store, Audit: h.sink, Clock: h.writer.Clock}

	writesBefore := h.store.updates
	if err := outcomes.Delivered(t.Context(), hookID); err != nil {
		t.Fatalf("recording a success: %v", err)
	}
	if h.store.updates != writesBefore {
		t.Error("a success against a healthy subscription cost a write")
	}

	if err := outcomes.Failed(t.Context(), hookID, "webhooks.target_unreachable", true); err != nil {
		t.Fatalf("recording a dead letter: %v", err)
	}
	if err := outcomes.Delivered(t.Context(), hookID); err != nil {
		t.Fatalf("recording the recovery: %v", err)
	}
	after := h.store.rows[hookID].Subscription
	if after.FailureCount != 0 || after.LastError != "" {
		t.Errorf("the success did not clear the run: %+v", after)
	}
}

var _ = time.Second
