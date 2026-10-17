// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PlatformDelivery is one installation event sent to one platform subscription, across its
// attempts. One row with a counter rather than a webhook's row per attempt: what an operator acts
// on is the event that did not arrive, and a redelivery counts on from where it stopped.
type PlatformDelivery struct {
	ID             shared.ID
	SubscriptionID shared.ID
	EventID        shared.ID
	EventType      string
	// Attempt is how many attempts were made, a redelivery's counting on.
	Attempt        int
	Status         DeliveryStatus
	ResponseStatus int
	// ErrorCode is a message code, never the target's own words (rule 10).
	ErrorCode     string
	NextAttemptAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewPlatformDelivery is an event queued for a subscription, no attempt made.
func NewPlatformDelivery(id, subscriptionID, eventID shared.ID, eventType string, now time.Time) (PlatformDelivery, error) {
	if id.IsZero() || subscriptionID.IsZero() || eventID.IsZero() || eventType == "" {
		return PlatformDelivery{}, shared.ErrInternal.WithDetail("webhooks.delivery_incomplete")
	}
	return PlatformDelivery{
		ID: id, SubscriptionID: subscriptionID, EventID: eventID, EventType: eventType,
		Status: DeliveryPending, CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
	}, nil
}

// Waiting reports whether an attempt is owed: queued, or failed with another coming.
func (d PlatformDelivery) Waiting() bool {
	return d.Status == DeliveryPending || d.Status == DeliveryFailed
}

// Arrived is an attempt the target answered 2xx.
func (d PlatformDelivery) Arrived(status int, now time.Time) PlatformDelivery {
	d.Attempt++
	d.Status, d.ResponseStatus, d.ErrorCode, d.NextAttemptAt, d.UpdatedAt = DeliverySucceeded, status, "", time.Time{}, now.UTC()
	return d
}

// Missed is an attempt that did not land: another follows at `next`, or after the webhook's
// number of attempts it stops as a dead letter, which waits for a redelivery. A subscription is
// never switched off for it.
func (d PlatformDelivery) Missed(status int, code string, next, now time.Time) PlatformDelivery {
	d.Attempt++
	d.ResponseStatus, d.ErrorCode, d.UpdatedAt = status, code, now.UTC()
	// The attempts of one round: a redelivery's attempts count on, so a round ends every
	// MaxDeliveryAttempts attempts rather than once for ever.
	if d.Attempt%MaxDeliveryAttempts == 0 {
		d.Status, d.NextAttemptAt = DeliveryDeadLetter, time.Time{}
		return d
	}
	d.Status, d.NextAttemptAt = DeliveryFailed, next.UTC()
	return d
}

// Expired is a delivery whose event was swept before it arrived: nothing can be sent any more.
func (d PlatformDelivery) Expired(now time.Time) PlatformDelivery {
	d.Status, d.ErrorCode, d.NextAttemptAt, d.UpdatedAt = DeliveryDeadLetter, "webhooks.event_expired", time.Time{}, now.UTC()
	return d
}
