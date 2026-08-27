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
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Orphan is one object the reconciliation decided to remove: the row's identity and where its
// bytes live, which is everything the purge job needs without a read of its own.
type Orphan struct {
	ID         shared.ID
	StorageKey string
}

// Thresholds are the two clocks the orphan sweep marks against, as instants rather than as
// durations: the application layer owns the Clock port, and a repository that subtracted a grace
// from a time of its own would be a second clock nobody could fix in a test (arc42 §8.13).
//
// A struct rather than two more parameters, because both are timestamps in the past and a caller
// that swapped them would compile, run, and mark the wrong rows.
type Thresholds struct {
	// Unreferenced is how far back a READY object must have stopped being pointed at. An object
	// is unreferenced between its confirmation and the first thing that uses it, and again between
	// a detachment and the next attachment; neither is evidence that it is garbage.
	Unreferenced time.Time
	// Pending is how far back a staging must have been made for nobody's confirmation to count as
	// abandonment.
	Pending time.Time
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

	// Recount makes every live counter what the references say, and records against every row
	// that reaches zero since when nothing has pointed at it - keeping the first such moment
	// rather than the latest, and clearing it again when a reference appears.
	Recount(ctx context.Context, now time.Time) error

	// MarkOrphans marks what nothing references and has not for long enough: READY rows
	// unreferenced since before before.Unreferenced, and PENDING rows staged before
	// before.Pending that nobody ever confirmed.
	MarkOrphans(ctx context.Context, now time.Time, before Thresholds) (int, error)

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

// Attachments maintains which items carry which objects, and the tags that decide it after a
// merge.
//
// The same two-tables-behind-one-interface as ItemLabels, and for the same reason: `item_attachment`
// is the link every read goes through and `set_element` is the OR-set tag that survives an offline
// merge (offline-sync.md §4.2). Writing one without the other is the failure mode - a link with no
// tag merges as last writer wins and loses a concurrent change - so neither is separately callable.
type Attachments interface {
	// Add links the object to the item, records the addition's tag, and reports whether the link
	// is new - attaching what is attached succeeds and reports false, so the caller does not raise
	// the reference count twice. The tag is written either way, so that the decision merges.
	Add(ctx context.Context, itemID, mediaID shared.ID, tag shared.HLC) (bool, error)

	// Remove unlinks, records the removal's tag, and reports whether there was a link. The tag is
	// written either way: a device that detaches something this replica never saw attached has
	// still made a decision another replica has to merge against.
	Remove(ctx context.Context, itemID, mediaID shared.ID, tag shared.HLC) (bool, error)

	// MediaIDs returns the identifiers an item carries, stably ordered.
	MediaIDs(ctx context.Context, itemID shared.ID) ([]shared.ID, error)

	// Elements returns every tag of one item's attachment set: what a merge compares a client's
	// tags against.
	Elements(ctx context.Context, itemID shared.ID) ([]work.SetElement, error)
}
