// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
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

// PlatformEvents is the slice of the installation's events a delivery reads: one, by its id.
type PlatformEvents interface {
	FindEvent(ctx context.Context, id shared.ID) (event.Envelope, error)
}

// PlatformSignals carries each settled attempt into the metrics, as the workspace's deliveries do.
type PlatformSignals interface {
	PlatformDelivery(ctx context.Context, result, statusClass string)
}

// PlatformDeliverer sends one installation event to one platform subscription (UC-INS-14): the
// workspace webhook's signature, headers and retry ladder, and none of its switching off. A
// delivery that ends without arriving stays as a dead letter the operator redelivers, and the
// subscription stays as it was - the operator is told by A-21, not by the subscription going quiet.
type PlatformDeliverer struct {
	Subscriptions adminrepo.PlatformSubscriptions
	Deliveries    adminrepo.PlatformDeliveries
	Events        PlatformEvents
	// Encryptor opens the sealed secret for the length of one HMAC.
	Encryptor crypto.Encryptor
	Signer    security.WebhookSigner
	// Client is the guarded one (rule 6): a private range or the metadata address is refused
	// unless the installation released private networks.
	Client httpport.Port
	// UnitOfWork is this handler's own: the call to the platform happens between two short
	// transactions, never inside one.
	UnitOfWork  persistence.UnitOfWork
	Jobs        Queue
	Clock       clock.Clock
	Source      string
	NextAttempt func(attempt int) time.Duration
	Signals     PlatformSignals
}

var (
	_ queue.Handler  = PlatformDeliverer{}
	_ queue.Detached = PlatformDeliverer{}
)

// OwnsItsTransactions declares the exception: a delivery is safe to run twice, since a platform
// deduplicates on X-Hubtask-Event-Id and an attempt is recorded only against the attempt it read.
func (PlatformDeliverer) OwnsItsTransactions() {}

// Run makes one attempt. A target that refused is no error: the retry is asked for explicitly, on
// the delivery's ladder rather than the queue's.
func (d PlatformDeliverer) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	raw, _ := job.Payload["delivery_id"].(string)
	deliveryID, err := shared.ParseID(raw)
	if err != nil {
		return queue.Result{}, shared.ErrInternal.WithDetail("webhooks.delivery_job_malformed")
	}

	var (
		delivery domain.PlatformDelivery
		stored   adminrepo.StoredPlatformSubscription
		envelope event.Envelope
		skip     bool
		expired  bool
	)
	err = d.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		delivery, err = d.Deliveries.Find(ctx, deliveryID)
		if errors.Is(err, shared.ErrNotFound) {
			skip = true // unsubscribed, or swept: nothing is owed any more
			return nil
		}
		if err != nil {
			return err
		}
		if !delivery.Waiting() {
			skip = true // a repeat of a job whose attempt is settled
			return nil
		}
		stored, err = d.Subscriptions.Find(ctx, delivery.SubscriptionID)
		if errors.Is(err, shared.ErrNotFound) {
			skip = true
			return nil
		}
		if err != nil {
			return err
		}
		envelope, err = d.Events.FindEvent(ctx, delivery.EventID)
		if errors.Is(err, shared.ErrNotFound) {
			expired = true
			return nil
		}
		return err
	})
	if err != nil || skip {
		return queue.Result{}, err
	}
	now := d.Clock.Now()
	if expired {
		return queue.Result{}, d.settle(ctx, delivery.Expired(now), "dead", 0)
	}
	if stored.Subscription.State == domain.PlatformPaused {
		// Paused after it was queued: not sent, and kept for a redelivery once it is resumed.
		return queue.Result{}, d.settle(ctx, delivery.Withheld("admin.subscription_paused", now), "dead", 0)
	}

	body, err := json.Marshal(eventbus.ToCloudEvent(envelope, d.Source))
	if err != nil {
		return queue.Result{}, shared.ErrInternal.WithDetail("webhooks.body_unrenderable").WithCause(err)
	}
	signing, err := d.Encryptor.Open(ctx, crypto.Sealed{
		KeyID: stored.Secret.KeyID, Ciphertext: stored.Secret.Ciphertext,
	}, crypto.Purpose(domain.PlatformSecretPurposeFor(stored.Subscription.ID)))
	if err != nil {
		return queue.Result{}, err
	}

	response, callErr := d.Client.Do(ctx, httpport.Request{
		Method: http.MethodPost,
		URL:    stored.Subscription.TargetURL,
		Header: map[string][]string{
			"Content-Type":           {"application/cloudevents+json; charset=utf-8"},
			security.SignatureHeader: {d.Signer.Sign(signing, now, body)},
			EventIDHeader:            {envelope.ID.String()},
			EventTypeHeader:          {envelope.Type.String()},
			AttemptHeader:            {itoa(delivery.Attempt + 1)},
		},
		Body:        body,
		TargetClass: targetClass,
	})

	now = d.Clock.Now()
	if callErr == nil && response.Status >= 200 && response.Status < 300 {
		return queue.Result{}, d.settle(ctx, delivery.Arrived(response.Status, now), "ok", response.Status)
	}
	status, code := 0, guardOrTransport(callErr)
	if callErr == nil {
		status, code = response.Status, statusCode(response.Status)
	}
	// The ladder of this round: a redelivery's first retry waits as a first retry does.
	missed := delivery.Missed(status, code, now.Add(d.NextAttempt(delivery.Attempt%domain.MaxDeliveryAttempts+1)), now)
	result := "retry"
	if missed.Status == domain.DeliveryDeadLetter {
		result = "dead"
	}
	return queue.Result{}, d.settle(ctx, missed, result, status)
}

// settle records the attempt, the subscription's last outcome and, where another attempt follows,
// asks for it - in one short transaction. A repeated job whose attempt is already recorded writes
// nothing more.
func (d PlatformDeliverer) settle(ctx context.Context, settled domain.PlatformDelivery, result string, status int) error {
	err := d.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		written, err := d.Deliveries.Record(ctx, settled)
		if err != nil || !written {
			return err
		}
		arrived := time.Time{}
		if settled.Status == domain.DeliverySucceeded {
			arrived = settled.UpdatedAt
		}
		if err := d.Subscriptions.Outcome(ctx, settled.SubscriptionID, arrived, settled.ErrorCode); err != nil {
			return err
		}
		if settled.Status != domain.DeliveryFailed {
			return nil
		}
		_, err = d.Jobs.Enqueue(ctx, queue.Request{
			Kind: queue.KindPlatformDeliver, RunAt: settled.NextAttemptAt,
			Payload: map[string]any{"delivery_id": settled.ID.String()},
		})
		return err
	})
	if err == nil && d.Signals != nil {
		class := "none"
		if status >= 100 {
			class = itoa(status/100) + "xx"
		}
		d.Signals.PlatformDelivery(ctx, result, class)
	}
	return err
}
