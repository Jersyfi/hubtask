// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package webhook sends one event to one subscriber (G-03, automation.md §3.1).
//
// It is an outbound adapter and nothing else: what to send and to whom was decided by the fan-out,
// and what a failure means is decided by the aggregate. What lives here is the part that needs the
// network - the body, the signature, the call through the guard, and the translation of an HTTP
// answer into "try again" or "stop".
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	"github.com/Jersyfi/hubtask/infrastructure/eventbus"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The headers automation.md §3.1 names, beside the signature.
const (
	EventIDHeader   = "X-Hubtask-Event-Id"
	EventTypeHeader = "X-Hubtask-Event-Type"
	AttemptHeader   = "X-Hubtask-Delivery-Attempt"
)

// targetClass is the metric label for these calls. What the call is, never who it goes to: a label
// per target host would grow a series per customer integration (rule 10).
const targetClass = "webhook"

// Events is the slice of the outbox this adapter reads: one event, by its identifier.
//
// A delivery renders the event as it was rather than carrying a copy in the job payload, so a
// retry two days later sends what the first attempt would have - and a job row does not become a
// second place a workspace's content lives.
type Events interface {
	FindEvent(ctx context.Context, id shared.ID) (event.Envelope, error)
}

// Outcomes is what an attempt reports back, and what decides the failure run and the auto-disable.
//
// Declared here rather than imported, because an outbound adapter may not call a use case
// (project-structure.md §2, and the gate that says so). The composition root puts the two
// together; what this package knows is that something answers these two questions.
type Outcomes interface {
	Delivered(ctx context.Context, subscriptionID shared.ID) error
	Failed(ctx context.Context, subscriptionID shared.ID, code string, terminal bool) error
}

// Queue is where the next attempt is asked for. The same one line the fan-out needs, declared
// again for the same reason.
type Queue interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// Deliverer sends one delivery. It is a queue handler, so the retry ladder is the queue's and the
// backoff policy is the resilience adapter's - eight attempts reaching a day is a schedule this
// system already knows how to keep.
type Deliverer struct {
	Subscriptions repository.WebhookSubscriptions
	Deliveries    repository.WebhookDeliveries
	Events        Events
	Outcomes      Outcomes
	// Encryptor opens the sealed signing secret. This is the only place a webhook secret is ever
	// in memory in clear, and it is in memory for the length of one HMAC.
	Encryptor crypto.Encryptor
	Signer    security.WebhookSigner
	// Client is the guarded one, always. A webhook target is an egress channel exactly as a backup
	// target is: a private range or the cloud metadata address is refused here unless the
	// installation has deliberately released private networks (rule 6, T-07).
	Client httpport.Port
	// UnitOfWork is this handler's own, because it is Detached: the call to somebody else's server
	// happens between two short transactions rather than inside one long one. Holding a database
	// connection for as long as a subscriber's server feels like taking is exactly what
	// observability-reliability.md §8 forbids.
	UnitOfWork persistence.UnitOfWork
	// Jobs is where the next attempt is asked for. A job per attempt rather than a repeating job,
	// so that each attempt has its own row and the log can answer "this event was tried eight
	// times over two days and these are the answers".
	Jobs  Queue
	Clock clock.Clock
	IDs   clock.IDGenerator
	// Source is what the CloudEvent names as its origin. The installation's own identifier, so a
	// subscriber receiving from two installations can tell them apart.
	Source string
	// NextAttempt is the backoff, given the attempts made so far. Injected for the runner's
	// reason: this layer decides when to retry, not how far apart.
	NextAttempt func(attempt int) time.Duration
}

var (
	_ queue.Handler  = Deliverer{}
	_ queue.Detached = Deliverer{}
)

// OwnsItsTransactions declares the exception this handler needs (queue.Detached).
//
// What it gives up is the atomicity everybody else gets for free, and that is acceptable here for
// the reason the interface asks about: a delivery is safe to run twice. At-least-once is the
// contract ADR-0007 states, a subscriber deduplicates on X-Hubtask-Event-Id, and a repeat that
// finds its attempt row already settled stops without sending anything.
func (Deliverer) OwnsItsTransactions() {}

// Run makes one attempt.
//
// It returns no error for a target that refused, and that is the shape the whole discipline rests
// on: a failed delivery is this system working correctly, and a job that failed would be retried
// by the queue on the queue's schedule rather than on the delivery's, and would eventually reach
// the queue's dead letter rather than the delivery log an operator reads. The retry is asked for
// explicitly instead.
func (d Deliverer) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	subscriptionID, deliveryID, err := identifiers(job)
	if err != nil {
		return queue.Result{}, err
	}
	scope := persistence.Scope{TenantID: job.TenantID}

	// One short transaction to read what the attempt needs, and then out of it before dialling.
	var (
		stored   repository.StoredSubscription
		delivery domain.WebhookDelivery
		envelope event.Envelope
		skip     bool
		expired  bool
	)
	err = d.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		stored, err = d.Subscriptions.Find(ctx, subscriptionID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				// Unsubscribed between the fan-out and the attempt. Not an error: the caller asked
				// for the deliveries to stop, and they have.
				skip = true
				return nil
			}
			return err
		}
		delivery, err = d.Deliveries.Find(ctx, deliveryID)
		if err != nil {
			return err
		}
		if delivery.Status != domain.DeliveryPending {
			// Already settled - a repeat of a job whose attempt is done. Doing it again would send
			// the event twice against one attempt row.
			skip = true
			return nil
		}

		envelope, err = d.Events.FindEvent(ctx, delivery.EventID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				// The event was swept before its delivery ran. The outbox's retention outliving
				// the retry ladder is a misconfiguration, and this is what it looks like here.
				expired = true
				return nil
			}
			return err
		}
		return nil
	})
	if err != nil || skip {
		return queue.Result{}, err
	}
	if expired {
		return queue.Result{}, d.write(ctx, scope, func(ctx context.Context) error {
			return d.settle(ctx, stored.Subscription, delivery, 0, "webhooks.event_expired", true)
		})
	}

	body, err := json.Marshal(eventbus.ToCloudEvent(envelope, d.Source))
	if err != nil {
		return queue.Result{}, shared.ErrInternal.
			WithDetail("webhooks.body_unrenderable").
			WithCause(err)
	}

	signing, err := d.Encryptor.Open(ctx, crypto.Sealed{
		KeyID: stored.Secret.KeyID, Ciphertext: stored.Secret.Ciphertext,
	}, crypto.Purpose(domain.SecretPurposeFor(subscriptionID)))
	if err != nil {
		return queue.Result{}, err
	}

	// The call itself, outside every transaction.
	now := d.Clock.Now()
	response, callErr := d.Client.Do(ctx, httpport.Request{
		Method: http.MethodPost,
		URL:    stored.Subscription.TargetURL,
		Header: map[string][]string{
			"Content-Type":           {"application/cloudevents+json; charset=utf-8"},
			security.SignatureHeader: {d.Signer.Sign(signing, now, body)},
			EventIDHeader:            {envelope.ID.String()},
			EventTypeHeader:          {envelope.Type.String()},
			AttemptHeader:            {itoa(delivery.Attempt)},
		},
		Body:        body,
		TargetClass: targetClass,
	})

	// And a second short transaction for what became of it.
	return queue.Result{}, d.write(ctx, scope, func(ctx context.Context) error {
		if callErr != nil {
			// Never reached the target: refused, timed out, or refused by the guard. The code is
			// ours and the cause is the client's to log; the target's own words are recorded
			// nowhere (rule 10).
			return d.retryOrStop(ctx, stored.Subscription, delivery, 0, guardOrTransport(callErr))
		}
		if response.Status >= 200 && response.Status < 300 {
			if err := d.record(ctx, delivery.Succeeded(response.Status)); err != nil {
				return err
			}
			return d.Outcomes.Delivered(ctx, subscriptionID)
		}
		return d.retryOrStop(ctx, stored.Subscription, delivery,
			response.Status, statusCode(response.Status))
	})
}

// write runs one short transaction for the bookkeeping either side of the call.
func (d Deliverer) write(
	ctx context.Context, scope persistence.Scope, fn func(context.Context) error,
) error {
	return d.UnitOfWork.Within(ctx, scope, fn)
}

// retryOrStop lets the delivery decide whether another attempt follows, and asks for it.
//
// A job per attempt rather than a repeating job: each attempt is its own row, which is what lets
// the log answer "this event was tried eight times over two days and these are the answers". A
// counter on one row cannot answer that, and neither can a job that keeps its original payload.
func (d Deliverer) retryOrStop(
	ctx context.Context, subscription domain.WebhookSubscription,
	delivery domain.WebhookDelivery, status int, code string,
) error {
	wait := d.NextAttempt(delivery.Attempt)
	settled := delivery.Failed(status, code, d.Clock.Now().Add(wait))
	if err := d.settleAs(ctx, subscription, settled, code); err != nil {
		return err
	}
	if settled.IsDeadLettered() {
		// Eight attempts over two days. The row stays as the dead letter, which is where an
		// operator looks and what a replay acts on.
		return nil
	}

	next, err := settled.Retried(d.IDs.NewID(), d.Clock.Now())
	if err != nil {
		return err
	}
	next.TenantID = settled.TenantID
	if err := d.Deliveries.Insert(ctx, next); err != nil {
		return err
	}

	_, err = d.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindWebhookDeliver, TenantID: settled.TenantID,
		RunAt: d.Clock.Now().Add(wait),
		Payload: map[string]any{
			"subscription_id": subscription.ID.String(),
			"delivery_id":     next.ID.String(),
			"event_id":        next.EventID.String(),
		},
	})
	return err
}

func (d Deliverer) settle(
	ctx context.Context, subscription domain.WebhookSubscription,
	delivery domain.WebhookDelivery, status int, code string, terminal bool,
) error {
	settled := delivery.Failed(status, code, time.Time{})
	if terminal {
		settled.Status = domain.DeliveryDeadLetter
		settled.NextAttemptAt = time.Time{}
	}
	return d.settleAs(ctx, subscription, settled, code)
}

func (d Deliverer) settleAs(
	ctx context.Context, subscription domain.WebhookSubscription,
	settled domain.WebhookDelivery, code string,
) error {
	if err := d.record(ctx, settled); err != nil {
		return err
	}
	// Only a delivery that has stopped counts against the subscription. Counting attempts would
	// disable one after three retries of a single event, which is a target that was briefly
	// unreachable rather than one that is gone.
	return d.Outcomes.Failed(ctx, subscription.ID, code, settled.IsDeadLettered())
}

func (d Deliverer) record(ctx context.Context, settled domain.WebhookDelivery) error {
	return d.Deliveries.RecordOutcome(ctx, repository.DeliveryOutcome{
		ID: settled.ID, Status: settled.Status,
		ResponseStatus: settled.ResponseStatus, ErrorCode: settled.ErrorCode,
		NextAttemptAt: settled.NextAttemptAt,
	})
}

// identifiers reads the job payload. A payload this handler cannot read is a programming error
// rather than a delivery to retry.
func identifiers(job queue.Job) (subscriptionID, deliveryID shared.ID, err error) {
	subscription, _ := job.Payload["subscription_id"].(string)
	delivery, _ := job.Payload["delivery_id"].(string)

	subscriptionID, subErr := shared.ParseID(subscription)
	deliveryID, delErr := shared.ParseID(delivery)
	if subErr != nil || delErr != nil {
		return "", "", shared.ErrInternal.
			WithDetail("webhooks.delivery_job_malformed").
			WithCause(fmt.Errorf("subscription %q, delivery %q", subscription, delivery))
	}
	return subscriptionID, deliveryID, nil
}

// statusCode turns an answer into one of a small closed set of message codes. Small and closed
// because it becomes a column and a message: a code per status would be a vocabulary nobody
// translates, and the status itself is recorded beside it anyway.
func statusCode(status int) string {
	switch {
	case status == http.StatusTooManyRequests:
		return "webhooks.target_rate_limited"
	case status >= 500:
		return "webhooks.target_unavailable"
	default:
		return "webhooks.target_refused"
	}
}

// guardOrTransport separates "this installation would not dial that" from "the dial did not
// work". The first is a configuration answer an operator can act on; the second is the target's
// problem and will very likely fix itself.
func guardOrTransport(err error) string {
	if errors.Is(err, shared.ErrForbidden) {
		return "webhooks.target_blocked"
	}
	return "webhooks.target_unreachable"
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
