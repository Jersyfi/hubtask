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

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Page is what a paged read asks for and reports back about the walk itself.
//
// The cursor is opaque here, and deliberately so: it is produced and read by the adapter, which is
// the only layer that knows what the rows are sorted by and holds the key it is signed with
// (api-guidelines.md §4, security.md §8). The application layer clamps the size, passes the cursor
// through, and never looks inside it - so a change of sort key is a change to one adapter rather than
// to every use case that lists something.
type Page struct {
	// Cursor is the boundary to continue after, empty for the first page.
	Cursor string
	// Size is how many rows the caller wants. Already clamped by the use case; an adapter reads one
	// row beyond it to answer HasMore, and does not return that row.
	Size int
}

// PageInfo is the walk's own state, as the contract's PageInfo schema carries it.
type PageInfo struct {
	// NextCursor continues the walk, and is empty when HasMore is false.
	NextCursor string
	HasMore    bool
}

// ContainerQuery names one level of the container tree.
//
// A level rather than a filtered set: an absent ParentID means the hubs, not "any parent". The tree
// is two deep, so "one level" is the whole of what a plain list can usefully mean here - everything
// else is the query DSL (B-12).
type ContainerQuery struct {
	// ParentID is the hub whose collections are wanted, or the zero identifier for the hubs.
	ParentID shared.ID
	// Type narrows the level further, and composes with ParentID rather than replacing it. The two
	// impossible combinations therefore return an empty page: a collection always has a parent, a
	// hub never does.
	Type work.ContainerType
	// IncludeArchived keeps archived containers in the page. Trashed ones are never in it - the
	// trash is its own view (B-10).
	IncludeArchived bool
	Page            Page
}

// ContainerPage is one page of a level.
type ContainerPage struct {
	Containers []work.Container
	Info       PageInfo
}

// ItemQuery names one level of one collection.
//
// CollectionID is required even when ParentID decides the level on its own: it is what the index the
// query reads through begins with, and what keeps the level of tasks from being a scan of every task
// in the tenant.
type ItemQuery struct {
	CollectionID shared.ID
	// ParentID is the item whose children are wanted, or the zero identifier for the items directly
	// in the collection.
	ParentID        shared.ID
	IncludeArchived bool
	Page            Page
}

// ItemPage is one page of a level.
type ItemPage struct {
	Items []work.WorkItem
	Info  PageInfo
}

// Containers stores hubs and collections.
type Containers interface {
	// Find returns the container, or ErrNotFound if it does not exist *for this tenant*. The two
	// cases are deliberately one answer: telling a caller that an identifier exists elsewhere
	// would confirm the existence of another tenant's data (multi-tenancy.md §2).
	Find(ctx context.Context, id shared.ID) (work.Container, error)

	// List returns one page of one level, in the containers' manual order.
	//
	// It reports what the tenant has, and judges no further than the query asks: whether the actor
	// may see a given container is decided in the application layer, which is the only place
	// authorisation happens (ADR-0005). A repository that filtered by permission would be a second
	// place for that rule to be wrong.
	List(ctx context.Context, query ContainerQuery) (ContainerPage, error)

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

	// List returns one page of one level of one collection, in the items' manual order.
	//
	// Trashed items are never in it, and archived ones only when asked - unlike Find, which reports
	// both as they are stored. The difference is not an inconsistency: Find answers "what is this
	// item", where the lifecycle state is the answer, and List answers "what is in this level", where
	// a trashed item is not.
	List(ctx context.Context, query ItemQuery) (ItemPage, error)

	// ChildCompletion counts the children of one item and how many of them are done - the two numbers
	// the roll-up decides from (I-W5).
	//
	// A summary rather than the children, because the question is one boolean: is anything still open
	// down there. Trashed children are not counted, archived ones are; the reasoning is at
	// work.ChildCompletion, and it lives there rather than here because it is a rule about what
	// "children" means and not about how they are stored.
	ChildCompletion(ctx context.Context, parentID shared.ID) (work.ChildCompletion, error)

	// SetCompletion writes an item's completion, or reports a version conflict.
	//
	// The expected version is passed rather than read off the item, for the reason UpdateGroup's is: the
	// caller knows which version it decided against, and an update that re-read the row would overwrite
	// whoever moved it in between (api-guidelines.md §5).
	SetCompletion(ctx context.Context, item work.WorkItem, expectedVersion int) error

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
