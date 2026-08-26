// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package eventbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	eventbusport "github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var (
	now      = time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	tenantID = shared.ID("018f3a1c-0000-7000-8000-00000000000a")
)

// pendingDouble is the outbox of one tenant.
type pendingDouble struct {
	events     []event.Envelope
	remaining  int
	dispatched []shared.ID
	claimErr   error
}

func (p *pendingDouble) Claim(_ context.Context, limit int) ([]event.Envelope, error) {
	if p.claimErr != nil {
		return nil, p.claimErr
	}
	if limit < len(p.events) {
		return p.events[:limit], nil
	}
	return p.events, nil
}

func (p *pendingDouble) MarkDispatched(_ context.Context, ids []shared.ID, _ time.Time) error {
	p.dispatched = append(p.dispatched, ids...)
	return nil
}

func (p *pendingDouble) CountPending(context.Context) (int, error) { return p.remaining, nil }

// consumptionDouble is the record of what a subscriber has seen, keyed the way the table is.
type consumptionDouble struct {
	seen map[string]bool
}

func newConsumption() *consumptionDouble { return &consumptionDouble{seen: map[string]bool{}} }

func (c *consumptionDouble) Claim(_ context.Context, consumer string, eventID shared.ID) (bool, error) {
	key := consumer + "/" + eventID.String()
	if c.seen[key] {
		return false, nil
	}
	c.seen[key] = true
	return true, nil
}

type subscriberDouble struct {
	name      string
	wants     event.Type
	delivered []shared.ID
	err       error
}

func (s *subscriberDouble) Name() string            { return s.name }
func (s *subscriberDouble) Wants(t event.Type) bool { return s.wants == "" || s.wants == t }
func (s *subscriberDouble) Deliver(_ context.Context, envelope event.Envelope) error {
	if s.err != nil {
		return s.err
	}
	s.delivered = append(s.delivered, envelope.ID)
	return nil
}

func envelopeAt(id string, occurredAt time.Time) event.Envelope {
	return event.Envelope{
		ID:         shared.ID(id),
		Type:       event.ContainerCreated,
		TenantID:   tenantID,
		Subject:    "container/" + id,
		OccurredAt: occurredAt,
	}
}

func dispatcher(pending *pendingDouble, consumed *consumptionDouble, subscribers ...*subscriberDouble) Dispatcher {
	registered := make([]eventbusport.Subscriber, 0, len(subscribers))
	for _, s := range subscribers {
		registered = append(registered, s)
	}
	return Dispatcher{
		Events:      pending,
		Consumed:    consumed,
		Subscribers: registered,
		Clock:       clock.Fixed(now),
		Batch:       10,
		MinInterval: time.Second,
		MaxInterval: 15 * time.Second,
	}
}

// dispatchJob is what the runner hands in: the job names the tenant whose transaction is open.
func dispatchJob() queue.Job {
	return queue.Job{
		ID:       shared.ID("018f3a1c-0000-7000-8000-0000000000ff"),
		TenantID: tenantID,
		Kind:     queue.KindOutboxDispatch,
		Lease:    now.Add(time.Minute),
	}
}

func TestEveryPendingEventIsDeliveredAndMarked(t *testing.T) {
	pending := &pendingDouble{events: []event.Envelope{
		envelopeAt("018f3a1c-0000-7000-8000-000000000001", now.Add(-2*time.Second)),
		envelopeAt("018f3a1c-0000-7000-8000-000000000002", now.Add(-time.Second)),
	}}
	subscriber := &subscriberDouble{name: "search"}

	result, err := dispatcher(pending, newConsumption(), subscriber).Run(t.Context(), dispatchJob())
	if err != nil {
		t.Fatalf("dispatching: %v", err)
	}

	if len(subscriber.delivered) != 2 {
		t.Errorf("%d events reached the subscriber, want 2", len(subscriber.delivered))
	}
	if len(pending.dispatched) != 2 {
		t.Errorf("%d events were marked, want 2", len(pending.dispatched))
	}
	// A poller does not finish: the round that emptied the outbox schedules the next one.
	if !result.Repeat {
		t.Error("the dispatch job completed instead of scheduling its next round")
	}
}

// The at-least-once guarantee made harmless (test RT-4). The same event delivered twice reaches
// the subscriber once - and the second round still marks it, because the outbox is not where the
// duplicate is corrected.
func TestASecondDeliveryDoesNotReachTheSubscriberAgain(t *testing.T) {
	envelope := envelopeAt("018f3a1c-0000-7000-8000-000000000001", now.Add(-time.Second))
	consumed := newConsumption()
	subscriber := &subscriberDouble{name: "automation"}

	for round := range 2 {
		pending := &pendingDouble{events: []event.Envelope{envelope}}
		if _, err := dispatcher(pending, consumed, subscriber).Run(t.Context(), dispatchJob()); err != nil {
			t.Fatalf("round %d: %v", round+1, err)
		}
	}

	if len(subscriber.delivered) != 1 {
		t.Errorf("the subscriber reacted %d times to one event, want once", len(subscriber.delivered))
	}
}

// A subscriber that is not interested is not asked, and nothing is recorded as consumed on its
// behalf - otherwise a subscriber that later grows an interest would never see the event.
func TestASubscriberIsOnlyAskedForWhatItWants(t *testing.T) {
	pending := &pendingDouble{events: []event.Envelope{
		envelopeAt("018f3a1c-0000-7000-8000-000000000001", now),
	}}
	uninterested := &subscriberDouble{name: "webhooks", wants: event.Type("de.hubtask.work.item.created.v1")}
	consumed := newConsumption()

	if _, err := dispatcher(pending, consumed, uninterested).Run(t.Context(), dispatchJob()); err != nil {
		t.Fatalf("dispatching: %v", err)
	}

	if len(uninterested.delivered) != 0 {
		t.Error("an event reached a subscriber that does not want that type")
	}
	if len(consumed.seen) != 0 {
		t.Errorf("consumption was recorded for a subscriber that was never asked: %v", consumed.seen)
	}
}

// A failure stops the batch where it happened. Delivering the events after it would hand consumers
// a later change before an earlier one, and nothing is marked - the whole round is retried.
func TestAFailedDeliveryStopsTheBatchAndMarksNothing(t *testing.T) {
	pending := &pendingDouble{events: []event.Envelope{
		envelopeAt("018f3a1c-0000-7000-8000-000000000001", now),
		envelopeAt("018f3a1c-0000-7000-8000-000000000002", now),
	}}
	failing := &subscriberDouble{name: "webhooks", err: errors.New("the target refused")}

	_, err := dispatcher(pending, newConsumption(), failing).Run(t.Context(), dispatchJob())
	if err == nil {
		t.Fatal("a failing subscriber was reported as a successful round")
	}
	if len(pending.dispatched) != 0 {
		t.Errorf("%d events were marked as delivered although the round failed", len(pending.dispatched))
	}
}

// The tenant comes from the job, and the transaction is opened for it. A dispatch job without one
// would read an empty outbox and report success for work it never did.
func TestADispatchJobWithoutATenantIsRefused(t *testing.T) {
	job := dispatchJob()
	job.TenantID = ""

	_, err := dispatcher(&pendingDouble{}, newConsumption()).Run(t.Context(), job)
	if err == nil || shared.AsError(err).DetailCode != "outbox.dispatch_without_tenant" {
		t.Fatalf("error %v, want the job to be refused", err)
	}
}

// The age at delivery is what SLO-4 measures. A clock that went backwards reports zero rather than
// a negative age, which would make the percentile lie in the reassuring direction.
func TestTheLagIsTheAgeOfTheEventAtDelivery(t *testing.T) {
	pending := &pendingDouble{events: []event.Envelope{
		envelopeAt("018f3a1c-0000-7000-8000-000000000001", now.Add(-3*time.Second)),
		envelopeAt("018f3a1c-0000-7000-8000-000000000002", now.Add(time.Second)),
	}}

	var reported []float64
	d := dispatcher(pending, newConsumption(), &subscriberDouble{name: "search"})
	d.Lag = func(_ context.Context, seconds float64) { reported = append(reported, seconds) }

	if _, err := d.Run(t.Context(), dispatchJob()); err != nil {
		t.Fatalf("dispatching: %v", err)
	}

	want := []float64{3, 0}
	if len(reported) != len(want) {
		t.Fatalf("%d lag readings, want %d", len(reported), len(want))
	}
	for i, seconds := range reported {
		if seconds != want[i] {
			t.Errorf("lag %d = %v seconds, want %v", i+1, seconds, want[i])
		}
	}
}

// The polling policy, which decides how quickly a change reaches its consumers and how much a
// quiet installation pays for the machinery.
func TestTheNextRoundFollowsFromWhatThisOneFound(t *testing.T) {
	d := dispatcher(&pendingDouble{}, newConsumption())

	cases := []struct {
		name      string
		delivered int
		remaining int
		want      time.Duration
	}{
		{"a backlog is chased without waiting", 10, 40, 0},
		{"a round that emptied the outbox comes back soon", 3, 0, time.Second},
		{"a quiet tenant is left alone", 0, 0, 15 * time.Second},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := d.nextRound(c.delivered, c.remaining); got != c.want {
				t.Errorf("nextRound(%d, %d) = %v, want %v", c.delivered, c.remaining, got, c.want)
			}
		})
	}
}

// replayingDouble is a subscriber that has been read against backup-restore.md §8.4 and wants the
// events a restore wrote - the search index, in the shape it will have.
type replayingDouble struct{ subscriberDouble }

func (r *replayingDouble) TakesReplays() {}

func TestAReplayedEventGoesOnlyToSubscribersThatAskedForOne(t *testing.T) {
	replayed := envelopeAt("018f3a1c-0000-7000-8000-000000000001", now.Add(-time.Second))
	replayed.Replay = true
	pending := &pendingDouble{events: []event.Envelope{replayed}}

	outward := &subscriberDouble{name: "webhooks"}
	inward := &replayingDouble{subscriberDouble{name: "search"}}

	dispatch := dispatcher(pending, newConsumption(), outward)
	dispatch.Subscribers = append(dispatch.Subscribers, inward)

	if _, err := dispatch.Run(context.Background(), dispatchJob()); err != nil {
		t.Fatalf("the round failed: %v", err)
	}

	// §8.4: a restore fires no automation and sends no webhook. The dispatcher is where that is
	// true, so that it is true for a subscriber written by somebody who has not read §8.4.
	if len(outward.delivered) != 0 {
		t.Errorf("a replayed event reached the outward-facing subscriber")
	}
	if len(inward.delivered) != 1 {
		t.Errorf("the search index was not given the replayed event")
	}
	// It is still marked: an event nobody wanted is delivered as far as it goes, and leaving it
	// pending would make the dispatcher read it again for ever.
	if len(pending.dispatched) != 1 {
		t.Errorf("the replayed event was left pending")
	}
}

func TestAnOrdinaryEventStillReachesEverySubscriber(t *testing.T) {
	pending := &pendingDouble{events: []event.Envelope{
		envelopeAt("018f3a1c-0000-7000-8000-000000000001", now.Add(-time.Second)),
	}}
	outward := &subscriberDouble{name: "webhooks"}

	if _, err := dispatcher(pending, newConsumption(), outward).Run(context.Background(), dispatchJob()); err != nil {
		t.Fatalf("the round failed: %v", err)
	}
	if len(outward.delivered) != 1 {
		t.Fatalf("an ordinary event was withheld")
	}
}

func TestTheCloudEventNamesAReplayAndSaysNothingOtherwise(t *testing.T) {
	ordinary := ToCloudEvent(envelopeAt("018f3a1c-0000-7000-8000-000000000001", now), "hubtask")
	if _, present := ordinary["replay"]; present {
		t.Errorf("an ordinary event carries a replay attribute")
	}

	replayed := envelopeAt("018f3a1c-0000-7000-8000-000000000001", now)
	replayed.Replay = true
	if ToCloudEvent(replayed, "hubtask")["replay"] != true {
		t.Errorf("a replayed event does not say so")
	}
}
