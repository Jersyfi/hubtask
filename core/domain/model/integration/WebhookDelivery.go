// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// DeliveryStatus is what became of one attempt.
type DeliveryStatus string

const (
	// DeliveryPending is an attempt that has not been made yet, or one waiting for its retry.
	DeliveryPending DeliveryStatus = "PENDING"
	// DeliverySucceeded is a target that answered 2xx.
	DeliverySucceeded DeliveryStatus = "SUCCEEDED"
	// DeliveryFailed is an attempt that did not land and has another coming.
	DeliveryFailed DeliveryStatus = "FAILED"
	// DeliveryDeadLetter is where the attempts stop. It is not a deletion: this is the row an
	// operator reads and the one a replay acts on (automation.md §3.1).
	DeliveryDeadLetter DeliveryStatus = "DEAD_LETTER"
)

// WebhookDelivery is one attempt at one event against one subscription.
//
// A row per attempt rather than a row per event with a counter, and the difference is what the
// listing is for: "this event was tried eight times over two days and these are the answers" is a
// question an operator asks, and a counter cannot answer it.
type WebhookDelivery struct {
	ID             shared.ID
	TenantID       shared.ID
	SubscriptionID shared.ID
	// EventID is the event's own identifier, which travels as X-Hubtask-Event-Id. A replay carries
	// the same one, so a subscriber that deduplicates on it recognises the repeat for what it is.
	EventID shared.ID
	// Attempt counts from one and carries on across a replay rather than resetting. The log is an
	// account of how many times this event was sent, and a replay that reset it would make the
	// account false.
	Attempt int
	Status  DeliveryStatus
	// ResponseStatus is what the target answered, and zero where it answered nothing at all -
	// a connection refused, a timeout, a name that does not resolve.
	ResponseStatus int
	// ErrorCode is a message code of ours. Never the target's response body: that body is somebody
	// else's system's output and belongs in no column of ours (rule 10).
	ErrorCode string
	// NextAttemptAt is when the retry is due, and zero when there will not be one.
	NextAttemptAt time.Time
	CreatedAt     time.Time
}

// NewWebhookDelivery records an attempt about to be made.
func NewWebhookDelivery(
	id, tenantID, subscriptionID, eventID shared.ID, attempt int, now time.Time,
) (WebhookDelivery, error) {
	if id.IsZero() || tenantID.IsZero() || subscriptionID.IsZero() || eventID.IsZero() {
		return WebhookDelivery{}, shared.ErrInternal.WithDetail("webhooks.delivery_incomplete")
	}
	if attempt < 1 {
		return WebhookDelivery{}, shared.ErrInternal.WithDetail("webhooks.delivery_attempt_invalid")
	}

	return WebhookDelivery{
		ID: id, TenantID: tenantID, SubscriptionID: subscriptionID, EventID: eventID,
		Attempt: attempt, Status: DeliveryPending, CreatedAt: now.UTC(),
	}, nil
}

// Succeeded is a target that answered. No next attempt, and no error left behind from a previous
// one: what this row records is this attempt.
func (d WebhookDelivery) Succeeded(status int) WebhookDelivery {
	d.Status = DeliverySucceeded
	d.ResponseStatus = status
	d.ErrorCode = ""
	d.NextAttemptAt = time.Time{}
	return d
}

// Failed records an attempt that did not land, and decides whether another one follows.
//
// The decision is here rather than at the caller because it is the same decision every caller
// would have to make: the attempt number against the maximum, and nothing else. A caller that got
// it wrong would either retry forever or dead-letter early, and both look like the system working.
func (d WebhookDelivery) Failed(status int, code string, nextAttemptAt time.Time) WebhookDelivery {
	d.ResponseStatus = status
	d.ErrorCode = code

	if d.Attempt >= MaxDeliveryAttempts {
		d.Status = DeliveryDeadLetter
		d.NextAttemptAt = time.Time{}
		return d
	}
	d.Status = DeliveryFailed
	d.NextAttemptAt = nextAttemptAt.UTC()
	return d
}

// IsDeadLettered reports where the attempts stopped, which is what a replay acts on.
func (d WebhookDelivery) IsDeadLettered() bool { return d.Status == DeliveryDeadLetter }

// Retried is the next attempt after one that failed and has another coming.
//
// A separate constructor from Replayed, and the guard is the difference between them: a retry
// follows automatically from an attempt that failed, and a replay is a person acting on one that
// stopped. Sharing one method would mean one of the two guards had to go, and both are load
// bearing - a retry of a dead letter would restart a ladder that was deliberately ended, and a
// replay of a failed attempt would put two attempts of one event in flight.
func (d WebhookDelivery) Retried(id shared.ID, now time.Time) (WebhookDelivery, error) {
	if d.Status != DeliveryFailed {
		return WebhookDelivery{}, shared.ErrConflict.
			WithDetail("webhooks.delivery_not_retryable").
			WithParams(map[string]string{"status": string(d.Status)})
	}
	return NewWebhookDelivery(id, d.TenantID, d.SubscriptionID, d.EventID, d.Attempt+1, now)
}

// Replayed is the same event sent again by hand, after whatever made it fail has been fixed.
//
// The identifier is new because this is a new attempt with its own outcome; the event identifier
// is the one it always had, so a subscriber deduplicating on X-Hubtask-Event-Id recognises the
// repeat. The attempt number carries on rather than resetting - see Attempt.
func (d WebhookDelivery) Replayed(id shared.ID, now time.Time) (WebhookDelivery, error) {
	if !d.IsDeadLettered() {
		// Replaying something that is still being retried would put two attempts of one event in
		// flight, and the second would arrive before the first had given up.
		return WebhookDelivery{}, shared.ErrConflict.
			WithDetail("webhooks.delivery_not_replayable").
			WithParams(map[string]string{"status": string(d.Status)})
	}
	return NewWebhookDelivery(id, d.TenantID, d.SubscriptionID, d.EventID, d.Attempt+1, now)
}
