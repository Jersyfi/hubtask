// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package integration holds the repository ports of what reaches into this system from outside
// it: today the calendar feeds, later the webhook subscriptions beside them.
package integration

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"

	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CalendarFeeds stores and finds the subscriptions.
//
// The presented token is passed whole rather than pre-hashed, exactly as the access tokens' port
// takes it: hashing needs the server-side pepper, and the pepper has no business in the
// application layer - it is a secret of the persistence adapter, which is also the only place
// that can produce the value stored (security.md §8).
type CalendarFeeds interface {
	// Insert writes a new feed, with the hash of the token that was minted beside it.
	Insert(ctx context.Context, feed domain.CalendarFeed, token domain.FeedToken) error

	// FindByToken returns the feed a presented token names, or an error wrapping
	// shared.ErrNotFound when the hash matches nothing.
	//
	// It reports what is stored and judges none of it: whether the token is revoked and whether
	// there is still a view behind it are the use case's questions, so the rules stay where they
	// can be tested without a database (ADR-0001).
	FindByToken(ctx context.Context, token domain.FeedToken) (domain.CalendarFeed, error)

	// Find returns one feed by its identifier, revoked or not.
	Find(ctx context.Context, id shared.ID) (domain.CalendarFeed, error)

	// ListForAccount returns one account's own feeds, newest first. Not paged: a person has a
	// handful of subscriptions, and a cursor over a handful is machinery nobody reads.
	ListForAccount(ctx context.Context, accountID shared.ID) ([]domain.CalendarFeed, error)

	// Revoke stamps the feed and reports whether it changed anything. False means it was already
	// revoked, which is not an error - the second call is somebody making sure.
	Revoke(ctx context.Context, id shared.ID, at time.Time) (bool, error)
}

// SealedSecret is a webhook's signing secret as it is stored: a ciphertext and the key it opens
// under. The pair travels together because a sealed value is a pair - an installation that has
// rotated its keyring holds several master keys (E-02).
//
// Declared here rather than reusing crypto.Sealed in the repository signature so that the port
// says what a row holds: two secrets, the current one and possibly a previous one with the moment
// it stops verifying.
type SealedSecret struct {
	Ciphertext []byte
	KeyID      string
}

func (s SealedSecret) IsZero() bool { return s.KeyID == "" && len(s.Ciphertext) == 0 }

// StoredSubscription is one row: the aggregate, plus the sealed values the domain deliberately
// knows nothing about.
type StoredSubscription struct {
	Subscription domain.WebhookSubscription
	Secret       SealedSecret
	// Previous is the secret a rotation retired. Zero when there has been no rotation, or when the
	// grace has been retired deliberately. Whether it still counts is the aggregate's question -
	// PreviousSecretUntil - and not this struct's.
	Previous SealedSecret
}

// WebhookSubscriptions stores and finds the standing requests external systems make (G-03).
type WebhookSubscriptions interface {
	// Insert writes a new subscription with its sealed secret.
	Insert(ctx context.Context, stored StoredSubscription) error

	// Find returns one subscription, or an error wrapping shared.ErrNotFound. One of another
	// tenant is not found rather than forbidden.
	Find(ctx context.Context, id shared.ID) (StoredSubscription, error)

	// List returns the workspace's subscriptions, newest first. Not paged: a workspace has a
	// handful of integrations.
	List(ctx context.Context) ([]domain.WebhookSubscription, error)

	// WantingEvent returns the active subscriptions that named this event type.
	//
	// Filtered in the database rather than in the process: the alternative is reading every
	// subscription of the tenant on every event, which is the shape that stops working exactly
	// when a workspace has enough integrations to care.
	WantingEvent(ctx context.Context, eventType event.Type) ([]StoredSubscription, error)

	// Update writes the mutable half against an expected version, and reports whether it matched.
	// False means somebody else moved the row on, which the caller answers with a conflict rather
	// than by overwriting them.
	Update(ctx context.Context, subscription domain.WebhookSubscription, expectedVersion int) (bool, error)

	// Rotate makes the new secret current and the current one previous, in one statement. Two
	// statements could leave a subscription with a new secret and no grace, which is the failure
	// the grace exists to prevent.
	Rotate(ctx context.Context, id shared.ID, secret SealedSecret, previousUntil time.Time, expectedVersion int) (bool, error)

	// Delete removes the subscription. Its deliveries go with it by cascade.
	Delete(ctx context.Context, id shared.ID) (bool, error)
}

// WebhookDeliveries is the log of what was sent and what became of it.
type WebhookDeliveries interface {
	// Insert records an attempt about to be made.
	Insert(ctx context.Context, delivery domain.WebhookDelivery) error

	// Find returns one delivery, or an error wrapping shared.ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.WebhookDelivery, error)

	// List returns one subscription's attempts, newest first, optionally narrowed to one status.
	// Paged, unlike the subscriptions: a busy integration produces a delivery per event.
	List(ctx context.Context, query DeliveryQuery) ([]domain.WebhookDelivery, error)

	// RecordOutcome writes what became of an attempt.
	RecordOutcome(ctx context.Context, outcome DeliveryOutcome) error
}

// DeliveryQuery is what a listing asks for.
type DeliveryQuery struct {
	SubscriptionID shared.ID
	// Status narrows to one outcome. Empty means every one.
	Status domain.DeliveryStatus
	// Before is the cursor: identifiers are time-ordered, so "older than this one" is the page.
	Before   shared.ID
	PageSize int
}

// DeliveryOutcome is what one attempt came to.
type DeliveryOutcome struct {
	ID     shared.ID
	Status domain.DeliveryStatus
	// ResponseStatus is what the target answered, and zero where it answered nothing at all.
	ResponseStatus int
	// ErrorCode is a message code of ours. Never the target's response body: that body is somebody
	// else's system's output (rule 10).
	ErrorCode string
	// NextAttemptAt is when the retry is due, and zero when there will not be one.
	NextAttemptAt time.Time
}
