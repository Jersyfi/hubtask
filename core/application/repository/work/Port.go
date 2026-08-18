// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package work declares what the work management use cases need from storage.
//
// Every method runs inside a unit of work, and what it can see is decided by the tenant that unit
// was opened with - never by a condition in the query (ADR-0010). A repository here therefore
// never takes a tenant parameter: a parameter can be forgotten, and row level security cannot.
package work

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Level names one level of one collection: the items with the same parent inside it. An absent ParentID is
// the level directly under the collection.
type Level struct {
	CollectionID shared.ID
	ParentID     shared.ID
}

// Move is everything a subtree move needs decided before it runs.
//
// The paths are passed rather than derived here, because deriving them is the domain's job: Placement and
// WorkItem.SubtreePathUnder produce them, and an adapter that built a path from parts would be a second
// place where the shape of a path is known (I-W2).
type Move struct {
	Item work.WorkItem
	// TargetParentID is the item it now sits under, or the zero identifier for the top level.
	TargetParentID shared.ID
	// CollectionID is where the whole subtree now lives. Unchanged for a move within one collection, and
	// written all the same: one statement either way is simpler than a statement with a branch.
	CollectionID shared.ID
	// OldPrefix and NewPrefix are the item's path before and after. Every descendant's path is its own
	// with the first swapped for the second.
	OldPrefix string
	NewPrefix string
	// DepthDelta is how far the whole subtree shifts.
	DepthDelta int
	OrderKey   string
	UpdatedAt  time.Time
	// ExpectedVersion locks the moved item's own row. Zero means the caller read no version.
	ExpectedVersion int
}

// Containers stores hubs and collections.
type Containers interface {
	// Find returns the container, or ErrNotFound if it does not exist *for this tenant*. The two
	// cases are deliberately one answer: telling a caller that an identifier exists elsewhere
	// would confirm the existence of another tenant's data (multi-tenancy.md §2).
	Find(ctx context.Context, id shared.ID) (work.Container, error)

	// LastOrderKey returns the highest rank among the containers directly under parentID, or the
	// empty string when there are none. An empty parentID means the hubs, which sit under nothing.
	//
	// Trashed containers count. Their rank is still occupied - a restore has to land where it
	// was, and reusing the key would put two containers in the same place.
	LastOrderKey(ctx context.Context, parentID shared.ID) (string, error)

	// Insert writes a new container.
	//
	// A name that is already taken at this level comes back as a conflict with the detail code
	// `containers.name_taken`, translated from the unique index rather than checked beforehand: a
	// check followed by an insert is two statements with a gap between them, and two requests
	// arriving in that gap both pass the check (multi-tenancy.md §2.1).
	Insert(ctx context.Context, container work.Container) error
}

// Items stores work items: tasks, work packages, and activities.
//
// One repository for all three levels, because they are one aggregate (ADR-0006). A repository
// per level would be three sets of the same five queries, and the cross-tenant test would have to
// be written three times to prove the same thing.
type Items interface {
	// Find returns the item, or ErrNotFound if it does not exist *for this tenant*. Trashed and
	// archived items come back as they are stored: whether one may take children is the domain's
	// question, and hiding a trashed item here would turn "it is in the trash" into "it does not
	// exist" (I-W4).
	Find(ctx context.Context, id shared.ID) (work.WorkItem, error)

	// Neighbours returns the two ranks a position sits between at one level: the item to go before, and
	// whatever sits below it.
	//
	// An empty beforeID means the end of the level. Both bounds come back as the empty string when there
	// is nothing there, which is what the ordering service reads as "no bound". The moving item is
	// excluded from its own level - reordering within a level would otherwise measure a position against
	// the rank the item is leaving.
	Neighbours(ctx context.Context, level Level, beforeID, movingID shared.ID) (previous, next string, err error)

	// SetOrderKey writes a new rank for one item, or reports a version conflict. The whole of what a
	// reorder changes.
	SetOrderKey(ctx context.Context, item work.WorkItem, expectedVersion int) error

	// MoveSubtree rewrites where an item and everything below it sits, and returns how many rows it
	// touched.
	//
	// One method rather than two, because the two statements behind it must not be separable: an item
	// whose parent moved and whose subtree's paths did not is a tree that no longer describes itself
	// (I-W2). The count is the size of the subtree, which the event reports so that a client knows how
	// much of its own copy to rewrite.
	MoveSubtree(ctx context.Context, move Move) (int, error)

	// LastOrderKey returns the highest rank among the siblings of a new item, or the empty string
	// when there are none. The siblings are the items with the same parent inside the same
	// collection; an empty parentID means those directly under the collection.
	//
	// Trashed items count, for the reason they count for containers: their rank is still
	// occupied, a restore has to land where it was, and reusing the key would put two items in
	// the same place.
	LastOrderKey(ctx context.Context, collectionID, parentID shared.ID) (string, error)

	// Insert writes a new item.
	Insert(ctx context.Context, item work.WorkItem) error
}
