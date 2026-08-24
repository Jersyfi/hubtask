// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package notification declares what the notification path needs from storage: the records it
// writes, and the preferences it honours.
//
// As everywhere in this layer, no method takes a tenant: the unit of work carries it and row level
// security compares it (ADR-0010, rule 3).
package notification

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Notifications stores the records.
type Notifications interface {
	// Insert writes a record, and reports whether this call was the one that wrote it.
	//
	// False is not an error. The outbox delivers at-least-once (ADR-0007), so a consumer may see
	// the same event twice, and the unique index over (event, recipient, channel) is what makes
	// the second write a no-op. The boolean is what stops the caller enqueuing a second delivery
	// for a message somebody is already going to receive.
	Insert(ctx context.Context, record domain.Notification) (bool, error)

	// Find returns the record, or ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.Notification, error)

	// Save writes back what happened to a record: its state, why, when it went, and how often it
	// has been tried. Nothing about what it is about - a delivery may change the outcome of a
	// notification and never its recipient.
	//
	// Zero rows matched comes back as ErrNotFound: a record the retention sweep removed while its
	// delivery was in flight is gone rather than silently un-updated.
	Save(ctx context.Context, record domain.Notification) error

	// DeleteExpired removes up to batch records written before the cutoff, oldest first, and
	// reports how many went. One batch per call, because the sweep's whole shape is one batch per
	// transaction (data-retention.md §5).
	DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error)

	// CountExpired reports how many records are due, counted no higher than ceiling.
	//
	// Bounded, so that a tenant with a million expired rows costs a page of an index scan rather
	// than a count of the table. What a pass needs is "is there more after this batch", and a
	// ceiling of batch+1 answers exactly that.
	CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error)
}

// Preferences stores what people have said about being told.
type Preferences interface {
	// Find returns one preference, or ErrNotFound where the account has said nothing.
	//
	// ErrNotFound rather than the default, deliberately. The default is a domain decision
	// (notification.DefaultPreference) and a repository that applied it would be a second place
	// the default is written down - which is how "on" and "off" come to disagree.
	Find(ctx context.Context, accountID shared.ID, category domain.Category, channel domain.Channel) (
		domain.Preference, error)

	// Save writes a preference, replacing whatever the account said before. An upsert: somebody
	// switching a category off twice is stating the same thing, not making a second row.
	Save(ctx context.Context, preference domain.Preference) error
}
