// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package integration holds the repository ports of what reaches into this system from outside
// it: today the calendar feeds, later the webhook subscriptions beside them.
package integration

import (
	"context"
	"time"

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
