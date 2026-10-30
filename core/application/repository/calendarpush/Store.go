// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package calendarpush is the store of the calendar pushes (migration 0278): a person's feeds
// written into their calendar at a provider, what each wrote, and the connections waiting for the
// provider's answer - read and written inside the unit of work's transaction (rule 3).
package calendarpush

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// Pending is a connection waiting for the provider's answer.
type Pending struct {
	ID        shared.ID
	AccountID shared.ID
	FeedID    shared.ID
	Provider  integration.PushProvider
	Verifier  crypto.Sealed
	ExpiresAt time.Time
}

// Store keeps the pushes.
type Store interface {
	// List answers a person's pushes in the order they were connected.
	List(ctx context.Context, accountID shared.ID) ([]integration.CalendarPush, error)
	Count(ctx context.Context, accountID shared.ID) (int, error)
	// Find answers a push with its sealed refresh token; not found for none.
	Find(ctx context.Context, id shared.ID) (integration.CalendarPush, crypto.Sealed, error)
	// Connect stores a push, or connects the feed's push at that provider again with a new grant;
	// it answers the push's identifier.
	Connect(ctx context.Context, push integration.CalendarPush, sealed crypto.Sealed) (shared.ID, error)
	// Record stores how a run went.
	Record(ctx context.Context, push integration.CalendarPush) error
	// Reseal replaces the refresh token a provider rotated.
	Reseal(ctx context.Context, id shared.ID, sealed crypto.Sealed) error
	// Disconnect removes a push and what it remembers writing; not found for none.
	Disconnect(ctx context.Context, id shared.ID) error

	Events(ctx context.Context, pushID shared.ID) ([]integration.PushedEvent, error)
	PutEvent(ctx context.Context, pushID shared.ID, event integration.PushedEvent, at time.Time) error
	DropEvent(ctx context.Context, pushID, entryID shared.ID) error

	Await(ctx context.Context, pending Pending, stateHash []byte) error
	// Take answers and removes the connection the state names; not found for none.
	Take(ctx context.Context, stateHash []byte) (Pending, error)
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}
