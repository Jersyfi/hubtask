// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package mail is the store of a workspace's mail intake (ADR-0094): its settings, the receipts of
// what it received, and the lookups that route a message.
package mail

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Settings reads and writes a workspace's intake settings.
type Settings interface {
	// Find answers the settings; a workspace that never set any answers DefaultSettings, off.
	Find(ctx context.Context) (domain.Settings, error)
	// Save replaces them. A name another workspace holds is a conflict, `mail.name_taken`.
	Save(ctx context.Context, settings domain.Settings, by shared.ID, at time.Time) error
	// WorkspaceNamed is the tenant whose enabled intake has this name on the operator's domain,
	// across tenants: it runs before any is known. Not found for none.
	WorkspaceNamed(ctx context.Context, name string) (shared.ID, error)
}

// Receipts is the intake's log, and its guard against a message delivered twice.
type Receipts interface {
	// Seen reports whether the workspace already received the message.
	Seen(ctx context.Context, messageID string) (bool, error)
	// Record writes the receipt; false for a message that already has one.
	Record(ctx context.Context, receipt domain.Receipt) (bool, error)
	// CountFrom is how many messages the sender sent since the instant.
	CountFrom(ctx context.Context, sender string, since time.Time) (int, error)
	// Recent answers the newest receipts first.
	Recent(ctx context.Context, limit int) ([]domain.Receipt, error)
	// DeleteExpired and CountExpired are the retention run's: one batch of the receipts older than
	// the cutoff, and how many there are, counted no higher than the ceiling.
	DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error)
	CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error)
}

// Places resolves what a message names: a collection by its handle, an entry by its ticket key.
// Both are not found for nothing live under that name.
type Places interface {
	CollectionByHandle(ctx context.Context, handle string) (shared.ID, error)
	ItemByKey(ctx context.Context, key domain.Key) (shared.ID, error)
	// KeyOf is an entry's ticket key; not found for an entry in the trash.
	KeyOf(ctx context.Context, itemID shared.ID) (domain.Key, error)
	// SetHandle gives a collection a handle; one another collection of the workspace holds - or
	// held, while it is in the trash - is a conflict, `mail.handle_taken`.
	SetHandle(ctx context.Context, collectionID shared.ID, handle string) error
}
