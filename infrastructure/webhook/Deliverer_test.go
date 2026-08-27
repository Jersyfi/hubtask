// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package webhook

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

var (
	tenant         = shared.ID("01936f2a-7c1e-7000-8000-000000000a01")
	subscriptionID = shared.ID("01936f2a-7c1e-7000-8000-000000000a02")
	deliveryID     = shared.ID("01936f2a-7c1e-7000-8000-000000000a03")
	nextID         = shared.ID("01936f2a-7c1e-7000-8000-000000000a04")
	eventID        = shared.ID("01936f2a-7c1e-7000-8000-000000000a05")
	author         = shared.ID("01936f2a-7c1e-7000-8000-000000000a06")
	now            = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	signingSecret  = "the-signing-secret-of-this-subscription"
)

type stores struct {
	subscription repository.StoredSubscription
	deliveries   map[shared.ID]domain.WebhookDelivery
	outcomes     []repository.DeliveryOutcome
	missing      bool
}

func (s *stores) Insert(_ context.Context, delivery domain.WebhookDelivery) error {
	s.deliveries[delivery.ID] = delivery
	return nil
}

func (s *stores) Find(_ context.Context, id shared.ID) (domain.WebhookDelivery, error) {
	delivery, found := s.deliveries[id]
	if !found {
		return domain.WebhookDelivery{}, shared.ErrNotFound.WithDetail("webhooks.delivery_not_found")
	}
	return delivery, nil
}

func (s *stores) List(context.Context, repository.DeliveryQuery) ([]domain.WebhookDelivery, error) {
	return nil, nil
}

func (s *stores) RecordOutcome(_ context.Context, outcome repository.DeliveryOutcome) error {
	s.outcomes = append(s.outcomes, outcome)
	delivery := s.deliveries[outcome.ID]
	delivery.Status = outcome.Status
	delivery.ResponseStatus = outcome.ResponseStatus
	delivery.ErrorCode = outcome.ErrorCode
	s.deliveries[outcome.ID] = delivery
	return nil
}

func (s *stores) FindSubscription(_ context.Context, _ shared.ID) (repository.StoredSubscription, error) {
	if s.missing {
		return repository.StoredSubscription{}, shared.ErrNotFound.
			WithDetail("webhooks.subscription_not_found")
	}
	return s.subscription, nil
}

// subscriptions adapts the store to the repository interface, which the deliverer only reads from.
type subscriptions struct{ store *stores }

func (s subscriptions) Insert(context.Context, repository.StoredSubscription) error { return nil }
func (s subscriptions) Find(ctx context.Context, id shared.ID) (repository.StoredSubscription, error) {
	return s.store.FindSubscription(ctx, id)
}
func (s subscriptions) List(context.Context) ([]domain.WebhookSubscription, error) { return nil, nil }
func (s subscriptions) WantingEvent(context.Context, event.Type) ([]repository.StoredSubscription, error) {
	return nil, nil
}
func (s subscriptions) Update(context.Context, domain.WebhookSubscription, int) (bool, error) {
	return true, nil
}
func (s subscriptions) Rotate(context.Context, shared.ID, repository.SealedSecret, time.Time, int) (bool, error) {
	return true, nil
}
func (s subscriptions) Delete(context.Context, shared.ID) (bool, error) { return true, nil }

type events struct{ missing bool }

func (e events) FindEvent(_ context.Context, id shared.ID) (event.Envelope, error) {
	if e.missing {
		return event.Envelope{}, shared.ErrNotFound.WithDetail("events.event_not_found")
	}
	return event.NewEnvelope(id, event.ItemCreated, tenant,
		"item/01936f2a-7c1e-7000-8000-000000000a07",
		event.Actor{Kind: shared.ActorUser, ID: author}, now, event.Cause{},
		map[string]any{"title": "Buy milk"})
}

type outcomes struct {
	delivered []shared.ID
	failures  []string
	terminals int
}

func (o *outcomes) Delivered(_ context.Context, id shared.ID) error {
	o.delivered = append(o.delivered, id)
	return nil
}

func (o *outcomes) Failed(_ context.Context, _ shared.ID, code string, terminal bool) error {
	o.failures = append(o.failures, code)
	if terminal {
		o.terminals++
	}
	return nil
}

// caller records the request and answers what the test told it to.
type caller struct {
	requests []httpport.Request
	status   int
	err      error
}

func (c *caller) Do(_ context.Context, req httpport.Request) (httpport.Response, error) {
	c.requests = append(c.requests, req)
	if c.err != nil {
		return httpport.Response{}, c.err
	}
	return httpport.Response{Status: c.status}, nil
}

type jobs struct{ requests []queue.Request }

func (j *jobs) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	j.requests = append(j.requests, request)
	return nextID, nil
}

type sealer struct{}

func (sealer) Seal(_ context.Context, plaintext secret.Secret, _ crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{KeyID: "k1", Ciphertext: []byte(plaintext.Reveal())}, nil
}
func (sealer) ActiveKeyID() string { return "k1" }
func (sealer) Open(_ context.Context, sealed crypto.Sealed, _ crypto.Purpose) (secret.Secret, error) {
	return secret.New(string(sealed.Ciphertext)), nil
}

type unitOfWork struct{}

func (unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}
func (u unitOfWork) WithinReadOnly(ctx context.Context, s persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, s, fn)
}

type ids struct{ next shared.ID }

func (i ids) NewID() shared.ID { return i.next }

type harness struct {
	deliverer Deliverer
	store     *stores
	calls     *caller
	results   *outcomes
	queued    *jobs
}

func newHarness(t *testing.T, attempt int) *harness {
	t.Helper()

	delivery, err := domain.NewWebhookDelivery(deliveryID, tenant, subscriptionID, eventID, attempt, now)
	if err != nil {
		t.Fatalf("building the delivery: %v", err)
	}
	subscription, err := domain.NewWebhookSubscription(domain.NewWebhookSubscriptionInput{
		ID: subscriptionID, TenantID: tenant, CreatedBy: author,
		TargetURL: "https://example.org/hooks", EventTypes: []string{string(event.ItemCreated)},
		Now: now,
	})
	if err != nil {
		t.Fatalf("building the subscription: %v", err)
	}

	h := &harness{
		store: &stores{
			subscription: repository.StoredSubscription{
				Subscription: subscription,
				Secret:       repository.SealedSecret{KeyID: "k1", Ciphertext: []byte(signingSecret)},
			},
			deliveries: map[shared.ID]domain.WebhookDelivery{delivery.ID: delivery},
		},
		calls: &caller{status: 200}, results: &outcomes{}, queued: &jobs{},
	}
	h.deliverer = Deliverer{
		Subscriptions: subscriptions{store: h.store}, Deliveries: h.store,
		Events: events{}, Outcomes: h.results,
		Encryptor: sealer{}, Signer: security.NewWebhookSigner(),
		Client: h.calls, UnitOfWork: unitOfWork{}, Jobs: h.queued,
		Clock: clock.Fixed(now), IDs: ids{next: nextID},
		Source:      "urn:hubtask:test",
		NextAttempt: func(attempt int) time.Duration { return time.Duration(attempt) * time.Minute },
	}
	return h
}

func job() queue.Job {
	return queue.Job{
		Kind: queue.KindWebhookDeliver, TenantID: tenant,
		Payload: map[string]any{
			"subscription_id": subscriptionID.String(),
			"delivery_id":     deliveryID.String(),
			"event_id":        eventID.String(),
		},
	}
}

// The acceptance criterion, in one test: a subscribed URL receives a signed CloudEvent whose
// signature verifies against the documented formula, and a tampered body does not.
func TestASubscriberReceivesASignedCloudEventThatVerifies(t *testing.T) {
	h := newHarness(t, 1)

	if _, err := h.deliverer.Run(t.Context(), job()); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(h.calls.requests) != 1 {
		t.Fatalf("made %d calls, want one", len(h.calls.requests))
	}

	request := h.calls.requests[0]
	signature := request.Header[security.SignatureHeader][0]
	if !security.NewWebhookSigner().Verify(signature, request.Body, secret.New(signingSecret)) {
		t.Error("the delivery's signature does not verify against its body")
	}

	tampered := append([]byte{}, request.Body...)
	tampered[len(tampered)-2] ^= 0x20
	if security.NewWebhookSigner().Verify(signature, tampered, secret.New(signingSecret)) {
		t.Error("a tampered body verified")
	}

	// The three headers automation.md §3.1 names, beside the signature.
	if request.Header[EventIDHeader][0] != eventID.String() {
		t.Errorf("the event id header is %v", request.Header[EventIDHeader])
	}
	if request.Header[EventTypeHeader][0] != string(event.ItemCreated) {
		t.Errorf("the event type header is %v", request.Header[EventTypeHeader])
	}
	if request.Header[AttemptHeader][0] != "1" {
		t.Errorf("the attempt header is %v", request.Header[AttemptHeader])
	}

	// The body is the CloudEvent, identical to the internal one - "no feature available only
	// internally or only externally".
	if !contains(request.Body, `"specversion":"1.0"`) || !contains(request.Body, `"tenantid"`) {
		t.Errorf("the body is not a CloudEvent: %s", request.Body)
	}
	if len(h.results.delivered) != 1 {
		t.Error("a 2xx did not end the failure run")
	}
}

// Attempt eight lands in the dead letter, and nothing further is queued.
func TestTheEighthAttemptLandsInTheDeadLetterAndQueuesNothing(t *testing.T) {
	h := newHarness(t, domain.MaxDeliveryAttempts)
	h.calls.status = 500

	if _, err := h.deliverer.Run(t.Context(), job()); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	if len(h.store.outcomes) != 1 || h.store.outcomes[0].Status != domain.DeliveryDeadLetter {
		t.Fatalf("outcomes = %+v, want the dead letter", h.store.outcomes)
	}
	if len(h.queued.requests) != 0 {
		t.Errorf("the dead letter queued %d further attempts", len(h.queued.requests))
	}
	// Only a delivery that has stopped counts against the subscription.
	if h.results.terminals != 1 {
		t.Errorf("the dead letter counted %d times against the subscription", h.results.terminals)
	}
}

// An earlier attempt asks for the next one, as its own row and its own job.
func TestAnEarlierFailureQueuesTheNextAttempt(t *testing.T) {
	h := newHarness(t, 1)
	h.calls.status = 503

	if _, err := h.deliverer.Run(t.Context(), job()); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	if h.store.outcomes[0].Status != domain.DeliveryFailed {
		t.Errorf("status = %s, want a retry to follow", h.store.outcomes[0].Status)
	}
	if len(h.queued.requests) != 1 {
		t.Fatalf("queued %d attempts, want one", len(h.queued.requests))
	}
	queued := h.queued.requests[0]
	if queued.Payload["delivery_id"] != nextID.String() {
		t.Error("the next job does not name the new attempt row")
	}
	if !queued.RunAt.After(now) {
		t.Errorf("the retry is due at %v, which is not after %v", queued.RunAt, now)
	}
	// The new row is a fresh attempt of the same event: a subscriber deduplicating on the event id
	// sees the repeat for what it is.
	next := h.store.deliveries[nextID]
	if next.EventID != eventID || next.Attempt != 2 {
		t.Errorf("the next attempt is %+v", next)
	}
	// Not yet a failed delivery: counting attempts would disable a subscription after three
	// retries of one event.
	if h.results.terminals != 0 {
		t.Error("a retryable attempt counted against the subscription")
	}
}

// BK-9's sibling: a target the guard refuses is a delivery that failed for a reason an operator can
// act on, and the code says which of the two kinds of failure it was.
func TestAGuardRefusalIsRecordedAsItsOwnKindOfFailure(t *testing.T) {
	h := newHarness(t, 1)
	h.calls.err = shared.ErrForbidden.WithDetail("httpclient.private_address_blocked")

	if _, err := h.deliverer.Run(t.Context(), job()); err != nil {
		t.Fatalf("delivering: %v", err)
	}

	if len(h.results.failures) != 1 || h.results.failures[0] != "webhooks.target_blocked" {
		t.Errorf("failures = %v, want the guard's own code", h.results.failures)
	}
	// No response status: the call never reached anybody, and a zero is what says so.
	if h.store.outcomes[0].ResponseStatus != 0 {
		t.Errorf("a blocked call recorded status %d", h.store.outcomes[0].ResponseStatus)
	}

	h.calls.err = errors.New("dial tcp: i/o timeout")
	h2 := newHarness(t, 1)
	h2.calls.err = h.calls.err
	if _, err := h2.deliverer.Run(t.Context(), job()); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if h2.results.failures[0] != "webhooks.target_unreachable" {
		t.Errorf("a transport failure is recorded as %q", h2.results.failures[0])
	}
}

// Two shapes that must send nothing: a subscription deleted between the fan-out and the attempt,
// and an attempt row somebody has already settled.
func TestNothingIsSentForAnUnsubscribedOrSettledDelivery(t *testing.T) {
	gone := newHarness(t, 1)
	gone.store.missing = true
	if _, err := gone.deliverer.Run(t.Context(), job()); err != nil {
		t.Fatalf("an unsubscribed delivery was an error: %v", err)
	}
	if len(gone.calls.requests) != 0 {
		t.Error("a deleted subscription was still called")
	}

	settled := newHarness(t, 1)
	delivery := settled.store.deliveries[deliveryID]
	settled.store.deliveries[deliveryID] = delivery.Succeeded(200)
	if _, err := settled.deliverer.Run(t.Context(), job()); err != nil {
		t.Fatalf("a settled delivery was an error: %v", err)
	}
	if len(settled.calls.requests) != 0 {
		t.Error("a settled attempt was sent again")
	}
}

// An event swept before its delivery ran: the outbox's retention outliving the retry ladder is a
// misconfiguration, and it must not become an endless retry.
func TestAnExpiredEventStopsRatherThanRetryingForever(t *testing.T) {
	h := newHarness(t, 1)
	h.deliverer.Events = events{missing: true}

	if _, err := h.deliverer.Run(t.Context(), job()); err != nil {
		t.Fatalf("delivering an expired event: %v", err)
	}
	if len(h.calls.requests) != 0 {
		t.Error("an expired event was still sent")
	}
	if h.store.outcomes[0].Status != domain.DeliveryDeadLetter {
		t.Errorf("status = %s, want the dead letter", h.store.outcomes[0].Status)
	}
	if len(h.queued.requests) != 0 {
		t.Error("an expired event queued a retry")
	}
}

func contains(body []byte, needle string) bool {
	return len(body) > 0 && len(needle) > 0 && indexOf(string(body), needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
