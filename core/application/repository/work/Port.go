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
	// BucketID is the column the moved item lands in, and empty for none. Decided by the use case
	// rather than kept from the row: a move to another collection takes the item away from the
	// board it was on, and a reference to a column of the collection it left would be one nothing
	// renders (I-W6).
	BucketID  shared.ID
	UpdatedAt time.Time
	// ExpectedVersion locks the moved item's own row. Zero means the caller read no version.
	ExpectedVersion int
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

	// SetAttributes writes a container's own descriptive fields - what RenameContainer may change -
	// or reports a version conflict.
	//
	// The whole container is passed rather than the fields that moved, for the reason Items.SetAttributes
	// takes the whole item: the decision about what the row should say has already been taken, and an
	// adapter handed a list of changes would have to apply them a second time. A name already taken at
	// this level comes back as `containers.name_taken`, from the same index an insert fails on.
	SetAttributes(ctx context.Context, container work.Container, expectedVersion int) error

	// SetPolicies writes a collection's policies, or reports a version conflict. One key of the
	// document is written and the rest is left alone - the others have use cases of their own.
	SetPolicies(ctx context.Context, container work.Container, expectedVersion int) error

	// SetArchived writes the container's own archive stamp, set or cleared, or reports a version
	// conflict.
	//
	// One row, whatever sits below it. A collection inherits an archived hub's state through the read
	// rather than through a stamp of its own (I-C3), which is what lets unarchiving restore exactly
	// what the archiving covered.
	SetArchived(ctx context.Context, container work.Container, expectedVersion int) error

	// SetPlacement writes where a collection sits and how it ranks there, or reports a version
	// conflict. The whole of what a move changes: a container tree is two deep, so nothing below it
	// has to follow.
	SetPlacement(ctx context.Context, container work.Container, expectedVersion int) error

	// Neighbours returns the two ranks a position sits between at one container level: the container
	// to go before, and whatever sits below it.
	//
	// An empty parentID means the hub level. An empty beforeID means the end of the level. Both bounds
	// come back as the empty string when there is nothing there, which is what the ordering service
	// reads as "no bound". The moving container is excluded from its own level - reordering within a
	// level would otherwise measure a position against the rank it is leaving.
	Neighbours(ctx context.Context, parentID, beforeID, movingID shared.ID) (previous, next string, err error)
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

	// SetAttributes writes an item's own fields - what UpdateWorkItem may change - or reports a version
	// conflict.
	//
	// The whole item is passed rather than the fields that moved, because the decision about what the row
	// should say has already been taken: the use case read the item, applied the update and refused what
	// the capability profile does not allow. An adapter handed a list of changes would have to apply them
	// a second time, which is the second place for that rule to be wrong (ADR-0005).
	SetAttributes(ctx context.Context, item work.WorkItem, expectedVersion int) error
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

	// MoveSubtree rewrites where an item and everything below it sits, drops the references the
	// destination cannot resolve, and returns how many rows it touched together with what was lost.
	//
	// One method rather than several, because the statements behind it must not be separable: an
	// item whose parent moved and whose subtree's paths did not is a tree that no longer describes
	// itself (I-W2), and one that kept a label of the collection it left is a reference nothing
	// renders (I-W6). The count is the size of the subtree, which the event reports so that a
	// client knows how much of its own copy to rewrite; the losses are what the answer names, since
	// I-W6 asks for them to be reported rather than discovered.
	MoveSubtree(ctx context.Context, move Move) (int, []work.DroppedReference, error)

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

// Buckets stores the columns of a collection's board.
//
// Its own repository rather than methods on Containers: a bucket is its own row with its own
// version and its own uniqueness, and the collection it belongs to is a foreign key rather than an
// aggregate boundary. Which is also why deleting one is not a cascade - MoveItems is what a
// deleted column owes its items, and it is a decision the use case takes rather than the schema.
type Buckets interface {
	// Find returns the bucket, or ErrNotFound if it does not exist *for this tenant*. Deleted
	// buckets come back as they are stored: what that state means is the domain's question, and
	// hiding a deleted bucket here would turn "it has been deleted" into "it never existed".
	Find(ctx context.Context, id shared.ID) (work.Bucket, error)

	// List returns a collection's board, left to right, deleted columns left out.
	//
	// Not paged, unlike the container and item lists: a board has as many columns as fit on a
	// screen, and the contract returns a plain array (api-guidelines.md §2).
	List(ctx context.Context, collectionID shared.ID) ([]work.Bucket, error)

	// LastOrderKey returns the highest rank among the collection's buckets, or the empty string
	// when it has none. Deleted buckets count - their rank is still occupied, and reusing the key
	// would put two columns in the same place.
	LastOrderKey(ctx context.Context, collectionID shared.ID) (string, error)

	// Insert writes a new bucket. A name already taken in this collection comes back as a conflict
	// with the detail code `buckets.name_taken`, translated from the unique index rather than
	// checked beforehand: a check followed by an insert is two statements with a gap between them,
	// and two requests arriving in that gap both pass the check (multi-tenancy.md §2.1).
	Insert(ctx context.Context, bucket work.Bucket) error

	// SetAttributes writes a bucket's own fields, or reports a version conflict.
	SetAttributes(ctx context.Context, bucket work.Bucket, expectedVersion int) error

	// SetOrderKey writes a new rank for one bucket, or reports a version conflict. The whole of
	// what a reorder changes: a board is one level, so nothing below a column follows it.
	SetOrderKey(ctx context.Context, bucket work.Bucket, expectedVersion int) error

	// SetDeleted writes the bucket's deletion stamp, or reports a version conflict. A soft delete:
	// the row stays, so that a change log entry can still name it (offline-sync.md §7).
	SetDeleted(ctx context.Context, bucket work.Bucket, expectedVersion int) error

	// Neighbours returns the two ranks a position sits between on one board: the bucket to go
	// before, and whatever sits below it. An empty beforeID means the end of the board, and both
	// bounds come back as the empty string when there is nothing there. The moving bucket is
	// excluded from its own board - a reorder would otherwise measure a position against the rank
	// it is leaving.
	Neighbours(ctx context.Context, collectionID, beforeID, movingID shared.ID) (previous, next string, err error)

	// FirstOther returns the collection's leftmost remaining bucket, ignoring one: where a deleted
	// column's items go. ErrNotFound when the column being deleted was the last one, which is not
	// a failure - the items then carry no bucket, which is the state the collection was in before
	// anybody made a board.
	FirstOther(ctx context.Context, collectionID, excludedID shared.ID) (work.Bucket, error)

	// MoveItems moves every item out of one bucket and into another, and returns how many it
	// touched. A zero target takes them out of any bucket at all.
	//
	// One method rather than a read followed by a write per item: what a deleted column owes its
	// items is one statement, and a loop over them would be a transaction whose length depended on
	// how busy the column was.
	MoveItems(ctx context.Context, sourceID, targetID shared.ID, at time.Time) (int, error)
}

// Labels stores a collection's vocabulary.
type Labels interface {
	// Find returns the label, or ErrNotFound if it does not exist *for this tenant*. Deleted
	// labels come back as they are stored, for the reason Buckets.Find returns deleted buckets.
	Find(ctx context.Context, id shared.ID) (work.Label, error)

	// List returns a collection's labels by name, deleted ones left out.
	List(ctx context.Context, collectionID shared.ID) ([]work.Label, error)

	// Insert writes a new label. A name already taken in this collection comes back as
	// `labels.name_taken`, from the unique index.
	Insert(ctx context.Context, label work.Label) error

	// SetAttributes writes a label's own fields, or reports a version conflict.
	SetAttributes(ctx context.Context, label work.Label, expectedVersion int) error

	// SetDeleted writes the label's deletion stamp, or reports a version conflict. The rows saying
	// which items carry it stay: the read side filters on the label's own stamp, which is what
	// makes a deletion undoable without having to remember who wore it.
	SetDeleted(ctx context.Context, label work.Label, expectedVersion int) error
}

// ItemLabels stores which items carry which labels, and the tags that decide it after a merge.
//
// Two tables behind one interface, deliberately. `item_label` is the membership every read goes
// through, and `set_element` is the OR-set tag that survives an offline merge (offline-sync.md
// §4.2, §10). Writing one without the other is the failure mode: membership with no tag merges as
// last writer wins and loses a concurrent change, a tag with no membership is invisible to every
// read. So neither is separately callable.
type ItemLabels interface {
	// List returns the labels an item carries, deleted labels left out.
	List(ctx context.Context, itemID shared.ID) ([]shared.ID, error)

	// Add puts a label on an item and records the addition's tag. Adding one the item already
	// carries is the state the caller asked for and succeeds, writing a fresh tag.
	Add(ctx context.Context, itemID, labelID shared.ID, tag shared.HLC) error

	// Remove takes a label off an item, records the removal's tag, and reports whether the item
	// carried it at all. The tag is written either way: a device that removes something this
	// replica never saw added has still made a decision that another replica has to merge.
	Remove(ctx context.Context, itemID, labelID shared.ID, tag shared.HLC) (bool, error)

	// Elements returns every tag of one item's label set: what a merge compares a client's tags
	// against.
	Elements(ctx context.Context, itemID shared.ID) ([]work.SetElement, error)
}
