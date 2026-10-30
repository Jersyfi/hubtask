// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package filestore is the store of a person's file store connections (migration 0279): their
// sign-ins to a Nextcloud or to Microsoft's OneDrive and SharePoint, each with its grant sealed,
// and the connections waiting for the store's answer - read and written inside the unit of work's
// transaction (rule 3).
package filestore

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/filelink"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// Pending is a connection waiting for the store's answer: a Microsoft sign-in's PKCE verifier,
// or a Nextcloud login flow's poll token and the address it is polled at.
type Pending struct {
	ID           shared.ID
	AccountID    shared.ID
	Kind         filelink.ConnectionKind
	Server       string
	PollEndpoint string
	Secret       crypto.Sealed
	ExpiresAt    time.Time
}

// Store keeps the connections.
type Store interface {
	// List answers a person's connections in the order they were made.
	List(ctx context.Context, accountID shared.ID) ([]filelink.Connection, error)
	Count(ctx context.Context, accountID shared.ID) (int, error)
	// Find answers a connection with its sealed grant; not found for none.
	Find(ctx context.Context, id shared.ID) (filelink.Connection, crypto.Sealed, error)
	// Connect stores a connection, or replaces the grant of the person's connection to the same
	// store as the same login; it answers the connection's identifier.
	Connect(ctx context.Context, connection filelink.Connection, sealed crypto.Sealed) (shared.ID, error)
	// Reseal replaces a grant the store rotated or a new sign-in brought.
	Reseal(ctx context.Context, id shared.ID, sealed crypto.Sealed) error
	Touch(ctx context.Context, id shared.ID, at time.Time) error
	// Disconnect removes a connection; not found for none.
	Disconnect(ctx context.Context, id shared.ID) error

	Await(ctx context.Context, pending Pending, stateHash []byte) error
	// Pending answers the connection the state names, leaving it in place; not found for none.
	Pending(ctx context.Context, stateHash []byte) (Pending, error)
	Forget(ctx context.Context, id shared.ID) error
	DeleteExpired(ctx context.Context, now time.Time) (int, error)
}
