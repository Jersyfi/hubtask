// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package media declares what the media use cases need from storage of the *records* - the rows
// beside the bytes. The bytes themselves go through core/port/storage; splitting the two is what
// keeps every byte operation outside a database transaction
// (observability-reliability.md §8).
//
// As everywhere in this layer, no method takes a tenant: the unit of work carries it, and row
// level security compares it (ADR-0010).
package media

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Orphan is one object the reconciliation decided to remove: the row's identity and where its
// bytes live, which is everything the purge job needs without a read of its own.
type Orphan struct {
	ID         shared.ID
	StorageKey string
}

// ItemRef is one item a media object serves, with the collection its authorisation path starts
// from.
type ItemRef struct {
	ItemID       shared.ID
	CollectionID shared.ID
}

// ObjectPage is one page of media objects, oldest upload first.
type ObjectPage struct {
	Objects []media.Object
	Info    repository.PageInfo
}

// Objects stores the media records.
type Objects interface {
	// Insert stages a new object.
	Insert(ctx context.Context, object media.Object) error

	// Find returns the object, marked or not, or ErrNotFound. Whether a marked object may still
	// be served is the domain's question.
	Find(ctx context.Context, id shared.ID) (media.Object, error)

	// Seal writes the confirmation: PENDING becomes READY with the judged type and the measured
	// size. Zero rows matched - already READY, marked, gone - comes back as ErrConflict, and the
	// caller reads the row again to answer from what is there.
	Seal(ctx context.Context, object media.Object) error

	// AdjustRefCount moves the counter by delta, floored at zero. The fast half of reference
	// counting; the reconciliation's recount is what keeps it honest.
	AdjustRefCount(ctx context.Context, id shared.ID, delta int) error

	// MarkDeleted marks an unreferenced, live object for the reconciliation to remove, and
	// reports whether it matched - a referenced object never does.
	MarkDeleted(ctx context.Context, id shared.ID, at time.Time) (bool, error)

	// Recount makes every live counter what the references say.
	Recount(ctx context.Context) error

	// MarkOrphans marks what nothing references: READY rows at zero, and PENDING rows staged
	// before pendingBefore that nobody ever confirmed.
	MarkOrphans(ctx context.Context, now, pendingBefore time.Time) (int, error)

	// TakeOrphans returns up to batch marked rows whose grace ended before markedBefore.
	TakeOrphans(ctx context.Context, markedBefore time.Time, batch int) ([]Orphan, error)

	// RemoveRows deletes the records for good and reports how many went. The journal entries are
	// the caller's to write first, in the same transaction (ADR-0020 §6).
	RemoveRows(ctx context.Context, ids []shared.ID) (int, error)

	// ReferencingItems returns the items the object serves - covers and attachments - bounded,
	// for the authorisation question.
	ReferencingItems(ctx context.Context, mediaID shared.ID) ([]ItemRef, error)

	// ListForItem returns one page of an item's attachments as media objects.
	ListForItem(ctx context.Context, itemID shared.ID, page repository.Page) (ObjectPage, error)
}

// Attachments maintains which items carry which objects.
type Attachments interface {
	// Add links the object to the item and reports whether the link is new - attaching what is
	// attached succeeds and reports false, so the caller does not raise the count twice.
	Add(ctx context.Context, itemID, mediaID shared.ID) (bool, error)

	// Remove unlinks and reports whether there was a link.
	Remove(ctx context.Context, itemID, mediaID shared.ID) (bool, error)

	// MediaIDs returns the identifiers an item carries, stably ordered.
	MediaIDs(ctx context.Context, itemID shared.ID) ([]shared.ID, error)
}
