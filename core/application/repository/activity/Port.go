// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package activity declares how an item's history is written and how it is read back.
//
// Two interfaces rather than one, for the reason the outbox has two: the writing side is an insert
// inside somebody else's transaction, called by every use case that changes an item, and the
// reading side is a page served to whoever opened the item - possibly by a read replica
// (multi-tenancy.md §7). A single interface would put both in every use case's dependency list and
// make each of them look able to do the other.
package activity

import (
	"context"

	domain "github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Journal appends to an item's history.
type Journal interface {
	// Record writes one entry inside the caller's transaction. It fails the transaction rather
	// than swallowing the error: a change that reached the tables without its history is a gap
	// nothing later can notice, because the history is the only record of itself.
	Record(ctx context.Context, entry domain.Entry) error
}

// History reads it back.
type History interface {
	// List returns one page of one item's history, newest first.
	List(ctx context.Context, itemID shared.ID, page Page) (EntryPage, error)
}

// Page is what a caller asks for: where to continue, and how much.
//
// Its own type rather than the work repository's, even though the two are field for field the same.
// They are the same *shape*, not the same thing - the cursor is opaque and adapter-owned
// (api-guidelines.md §4), and a history cursor handed to a list of items would decode to a boundary
// in the wrong sort order.
type Page struct {
	// Cursor is the boundary to continue after, empty for the newest entry.
	Cursor string
	// Size is how many entries the caller wants, already clamped by the use case. The adapter reads
	// one beyond it to answer HasMore, and does not return that one.
	Size int
}

// PageInfo is the walk's own state, as the contract's PageInfo schema carries it.
type PageInfo struct {
	// NextCursor continues the walk, and is empty when HasMore is false.
	NextCursor string
	HasMore    bool
}

// EntryPage is one page of a history.
type EntryPage struct {
	Entries []domain.Entry
	Info    PageInfo
}
