// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webhook

import (
	"context"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

var platformAt = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

// platformStore is the subscriptions and deliveries as the tables keep them; the record of an
// attempt is guarded on the attempt before, as the statement is.
type platformStore struct {
	adminrepo.PlatformSubscriptions
	subscription domain.PlatformSubscription
	deliveries   map[shared.ID]domain.PlatformDelivery
	outcomes     []string
	updates      int
}

func (s *platformStore) Find(_ context.Context, id shared.ID) (adminrepo.StoredPlatformSubscription, error) {
	if id != s.subscription.ID {
		return adminrepo.StoredPlatformSubscription{}, shared.ErrNotFound
	}
	return adminrepo.StoredPlatformSubscription{
		Subscription: s.subscription, Secret: adminrepo.PlatformSecret{Ciphertext: []byte(signingSecret), KeyID: "k1"},
	}, nil
}

func (s *platformStore) Update(context.Context, domain.PlatformSubscription, int, time.Time) (bool, error) {
	s.updates++
	return true, nil
}

func (s *platformStore) Outcome(_ context.Context, _ shared.ID, _ time.Time, lastError string) error {
	s.outcomes = append(s.outcomes, lastError)
	return nil
}

type platformDeliveries struct{ store *platformStore }

func (d platformDeliveries) Find(_ context.Context, id shared.ID) (domain.PlatformDelivery, error) {
	delivery, ok := d.store.deliveries[id]
	if !ok {
		return domain.PlatformDelivery{}, shared.ErrNotFound
	}
	return delivery, nil
}

func (d platformDeliveries) Record(_ context.Context, settled domain.PlatformDelivery) (bool, error) {
	held := d.store.deliveries[settled.ID]
	if held.Attempt != settled.Attempt-1 || !held.Waiting() {
		return false, nil
	}
	d.store.deliveries[settled.ID] = settled
	return true, nil
}

// these satisfy the rest of the port; the deliverer does not call them.
func (platformDeliveries) Insert(context.Context, domain.PlatformDelivery) (bool, error) {
	return false, nil
}
func (platformDeliveries) Page(context.Context, shared.ID, string, shared.ID, int) ([]domain.PlatformDelivery, error) {
	return nil, nil
}
func (platformDeliveries) Requeue(context.Context, shared.ID, time.Time, int) ([]shared.ID, error) {
	return nil, nil
}
func (platformDeliveries) DeadLetters(context.Context) (int64, error)         { return 0, nil }
func (platformDeliveries) Sweep(context.Context, time.Time, int) (int, error) { return 0, nil }

type platformEvents struct{ missing bool }

func (e platformEvents) FindEvent(_ context.Context, id shared.ID) (event.Envelope, error) {
	if e.missing {
		return event.Envelope{}, shared.ErrNotFound
	}
	return event.NewWorkspaceEvent(id, event.WorkspaceSuspended, event.WorkspaceState{
		ID: tenant, Status: "SUSPENDED", Version: 3,
	}, event.Actor{Kind: shared.ActorSystem}, platformAt, nil)
}

type platformHarness struct {
	deliverer PlatformDeliverer
	store     *platformStore
	call      *caller
	jobs      *jobs
}

func newPlatformHarness(status int) platformHarness {
	store := &platformStore{deliveries: map[shared.ID]domain.PlatformDelivery{}}
	store.subscription, _ = domain.NewPlatformSubscription(subscriptionID, "https://shop.example/hubtask", false, author, platformAt)
	queued, _ := domain.NewPlatformDelivery(deliveryID, subscriptionID, eventID, string(event.WorkspaceSuspended), platformAt)
	store.deliveries[deliveryID] = queued
	call, asked := &caller{status: status}, &jobs{}
	return platformHarness{
		store: store, call: call, jobs: asked,
		deliverer: PlatformDeliverer{
			Subscriptions: store, Deliveries: platformDeliveries{store}, Events: platformEvents{},
			Encryptor: sealer{}, Signer: security.NewWebhookSigner(), Client: call,
			UnitOfWork: unitOfWork{}, Jobs: asked, Clock: clock.Fixed(platformAt), Source: "https://hubtask.example",
			NextAttempt: func(attempt int) time.Duration { return time.Duration(attempt) * time.Minute },
		},
	}
}

func (h platformHarness) run(t *testing.T, id shared.ID) {
	t.Helper()
	if _, err := h.deliverer.Run(t.Context(), queue.Job{Kind: queue.KindPlatformDeliver,
		Payload: map[string]any{"delivery_id": id.String()}}); err != nil {
		t.Fatalf("delivering: %v", err)
	}
}

// The event arrives as the workspace's webhooks send theirs: the CloudEvent, signed with the
// subscription's secret, with the event's id, type and attempt (UC-INS-14 check 1).
func TestAnInstallationEventArrivesSignedAsAWebhook(t *testing.T) {
	h := newPlatformHarness(204)
	h.run(t, deliveryID)

	if len(h.call.requests) != 1 {
		t.Fatalf("%d calls", len(h.call.requests))
	}
	sent := h.call.requests[0]
	if !h.deliverer.Signer.Verify(sent.Header[security.SignatureHeader][0], sent.Body, secret.New(signingSecret)) {
		t.Error("the signature does not verify with the subscription's secret")
	}
	if sent.Header[EventIDHeader][0] != eventID.String() || sent.Header[EventTypeHeader][0] != string(event.WorkspaceSuspended) ||
		sent.Header[AttemptHeader][0] != "1" || sent.TargetClass != "webhook" {
		t.Errorf("headers %v, class %s", sent.Header, sent.TargetClass)
	}
	if got := h.store.deliveries[deliveryID]; got.Status != domain.DeliverySucceeded || got.Attempt != 1 {
		t.Errorf("settled %+v", got)
	}
	if len(h.store.outcomes) != 1 || h.store.outcomes[0] != "" {
		t.Errorf("outcomes %v", h.store.outcomes)
	}

	// A repeat of the settled job sends nothing.
	h.run(t, deliveryID)
	if len(h.call.requests) != 1 {
		t.Error("a settled delivery was sent again")
	}
}

// Three deliveries that each end as a dead letter leave the subscription as it was: never switched
// off, its dead letters waiting for a redelivery (UC-INS-14 check 6).
func TestThreeDeadLettersLeaveTheSubscriptionOn(t *testing.T) {
	h := newPlatformHarness(503)
	ids := []shared.ID{deliveryID, nextID, author}
	for _, id := range ids[1:] {
		queued, _ := domain.NewPlatformDelivery(id, subscriptionID, eventID, string(event.WorkspaceSuspended), platformAt)
		h.store.deliveries[id] = queued
	}
	for _, id := range ids {
		for attempt := 0; attempt < domain.MaxDeliveryAttempts; attempt++ {
			h.run(t, id)
		}
		if got := h.store.deliveries[id]; got.Status != domain.DeliveryDeadLetter || got.ErrorCode != "webhooks.target_unavailable" {
			t.Fatalf("after a round %+v", got)
		}
	}
	if h.store.updates != 0 || h.store.subscription.State != domain.PlatformActive {
		t.Errorf("the subscription was changed %d times; it stands %s", h.store.updates, h.store.subscription.State)
	}
	// Seven retries asked for per delivery, none after the dead letter.
	if len(h.jobs.requests) != 3*(domain.MaxDeliveryAttempts-1) {
		t.Errorf("%d retries asked for", len(h.jobs.requests))
	}
	for _, request := range h.jobs.requests {
		if request.Kind != queue.KindPlatformDeliver || !request.TenantID.IsZero() {
			t.Errorf("asked %+v", request)
		}
	}
}

func TestAnUndeliverableEventIsHeldNotSent(t *testing.T) {
	expired := newPlatformHarness(204)
	expired.deliverer.Events = platformEvents{missing: true}
	expired.run(t, deliveryID)
	if got := expired.store.deliveries[deliveryID]; got.Status != domain.DeliveryDeadLetter || got.ErrorCode != "webhooks.event_expired" ||
		len(expired.call.requests) != 0 {
		t.Errorf("an expired event: %+v, %d calls", got, len(expired.call.requests))
	}

	paused := newPlatformHarness(204)
	paused.store.subscription.State = domain.PlatformPaused
	paused.run(t, deliveryID)
	if got := paused.store.deliveries[deliveryID]; got.Status != domain.DeliveryDeadLetter || got.ErrorCode != "admin.subscription_paused" ||
		len(paused.call.requests) != 0 {
		t.Errorf("a paused subscription: %+v, %d calls", got, len(paused.call.requests))
	}
}

// A redelivered round climbs the ladder from its first rung again.
func TestARedeliveredRoundStartsTheLadderAgain(t *testing.T) {
	h := newPlatformHarness(503)
	held := h.store.deliveries[deliveryID]
	held.Attempt = domain.MaxDeliveryAttempts
	h.store.deliveries[deliveryID] = held
	h.run(t, deliveryID)
	if len(h.jobs.requests) != 1 || !h.jobs.requests[0].RunAt.Equal(platformAt.Add(time.Minute)) {
		t.Errorf("the first retry of a redelivery is asked for %+v, want a minute on", h.jobs.requests)
	}
}
