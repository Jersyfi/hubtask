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
	"github.com/Jersyfi/hubtask/core/domain/model/view"
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
	// RestrictTo narrows the level to a named set of entries, or to all of them when it is empty.
	//
	// It carries the read half of C-04: an actor who holds no role on the collection may still
	// hold a membership on entries inside it, and then the level is those entries rather than a
	// refusal. Applied in the query rather than after it, so a page is a page - filtering the rows
	// out afterwards would return short pages and a cursor that skips.
	//
	// It is a narrowing and never a widening: everything else the query excludes stays excluded.
	RestrictTo []shared.ID
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

// Copy is one entry a duplication writes: the row it wants, and which definition each of its
// custom field values belongs to.
//
// The references travel beside the entry rather than on it, because they are not part of what the
// domain models an entry as: `custom_field_refs` says which definition a value was written under,
// which is what keeps a deleted-and-recreated key from resurrecting what the old one held (C-07,
// migration 0018). The map is keyed exactly as the document is, and a document key without one is
// a defect the adapter refuses rather than a value written under nothing.
//
// Everything else the copy decides - the new identifier, the path, the rank, the references the
// destination could resolve - is decided in the application layer and arrives here settled
// (ADR-0005).
type Copy struct {
	Item work.WorkItem
	// FieldDefinitions maps each custom field key of Item to the definition it is written under in
	// the destination collection.
	FieldDefinitions map[string]shared.ID
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

	// TrashSubtree moves a hub or a collection and everything under it into the trash under one
	// batch, and reports what went with it.
	//
	// One method rather than several, for the reason MoveSubtree is one: the statements behind it
	// must not be separable. A hub in the trash whose collections are still live is a tree that
	// does not describe itself, and a collection in the trash whose items are not is a deletion
	// that a restore could not reverse (I-C2). A row already in the trash from an earlier deletion
	// keeps that deletion and is not in the answer.
	TrashSubtree(ctx context.Context, trash ContainerTrash) (Cascade, error)

	// RestoreBatch takes one deletion back out of the trash, whole, and reports what came back.
	//
	// Keyed on the batch and not on the subtree, which is what makes a restore exactly reverse the
	// deletion it undoes: a younger, separate deletion inside the same subtree stays in the trash.
	RestoreBatch(ctx context.Context, restore ContainerTrash) (Cascade, error)

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

// AnchorKind says what a query is anchored to. The three shapes need three different scope
// predicates, and which one applies is something the use case has already established: it read the
// container or the item in order to ask the permission question about it.
type AnchorKind string

const (
	// AnchorTenant is everything the transaction can see, which is one tenant's entries and only
	// those (ADR-0010). The unanchored read, and the only one: a search is the one question that
	// is asked of a workspace rather than of a place in it, and the answer is narrowed to what the
	// actor may see rather than refused (C-08, view.Search).
	AnchorTenant AnchorKind = "TENANT"
	// AnchorHub is a whole hub: everything in every collection under it.
	AnchorHub AnchorKind = "HUB"
	// AnchorCollection is one collection.
	AnchorCollection AnchorKind = "COLLECTION"
	// AnchorItem is one entry, and with descendants its subtree.
	AnchorItem AnchorKind = "ITEM"
)

// Anchor is the resolved scope of a query.
//
// Resolved rather than repeated: view.Scope says what the client asked for, and this says what the
// use case found when it looked - which collection, which path, whether the container was a hub.
// An adapter that worked it out for itself would read the same rows a second time, and would be a
// second place where "what a hub contains" is decided.
type Anchor struct {
	Kind AnchorKind
	// CollectionID is the collection to look in, for a collection or an item anchor.
	CollectionID shared.ID
	// HubID is the hub, for a hub anchor.
	HubID shared.ID
	// ItemID is the entry, for an item anchor.
	ItemID shared.ID
	// PathPrefix is that entry's own path, which every row of its subtree begins with (I-W2).
	PathPrefix string
	// IncludeDescendants searches the subtree rather than one level below the anchor.
	IncludeDescendants bool
}

// ItemSearch is one query as the adapter receives it: the validated request and its resolved
// anchor.
type ItemSearch struct {
	Anchor Anchor
	Spec   view.Spec
	// Language is the tag the query's own words are read under - the searcher's, resolved by the
	// use case, and empty for a caller whose language nothing said.
	//
	// The searcher's rather than the entry's, and the difference is the whole reason it is here.
	// An entry's document is built under the configuration its own language names (ADR-0034), and
	// a query parsed per row would be exact and unindexable: an index scan needs the query to be
	// one value for the whole scan. So the words are parsed under the searcher's configuration and
	// under `simple`, which is two constants and two index scans.
	Language string
	// RestrictTo narrows the answer to a named set of entries, or to all of them when it is
	// empty - the read half of C-04, exactly as ItemQuery carries it for the plain level list.
	RestrictTo []shared.ID
}

// TextSearch is one full-text search as the adapter receives it: the validated request, its
// resolved scope, and the narrowing the use case decided (C-08).
type TextSearch struct {
	Anchor  Anchor
	Request view.Search
	// RestrictTo narrows the answer to a named set of entries, or to all of them when it is
	// empty - the read half of C-04, exactly as ItemSearch carries it for the query language.
	RestrictTo []shared.ID
}

// ItemHit is one entry a search found: the entry, where it sits, and how well it matched.
//
// The hub travels with it for the reason it travels with a trash entry: the search spans hubs, so
// the permission has to be asked about each row's place in the tree, and a membership held at a
// hub applies downwards (domain-model.md §3.2). Reading it back per row afterwards would be one
// query per collection in the page.
type ItemHit struct {
	Item  work.WorkItem
	HubID shared.ID
	// Rank is what `ts_rank_cd` answered, and it is the order rather than a number anybody reads:
	// it is comparable inside one result set and means nothing outside it. It is on the hit
	// because the cursor is a keyset over it - a walk that lost it could not continue.
	Rank float32
}

// ItemHitPage is one page of a search, in rank order.
type ItemHitPage struct {
	Hits []ItemHit
	Info PageInfo
}

// ItemGroup is one column of a grouped result: the entries that share a value of the grouping
// field, with a walk of their own.
type ItemGroup struct {
	// Key is the shared value, rendered as text - an identifier, an item type, a boolean.
	Key string
	// Absent marks the one group whose entries have no value at all: the entries on no board
	// column, for instance. It is a group rather than an omission, because a board draws it.
	Absent bool
	Items  []work.WorkItem
	Info   PageInfo
	// Total is how many entries the group holds altogether, with an exact count asked for.
	Total int
}

// ItemQueryResult is what a query answers: rows, or groups of rows, and a total when one was
// asked for.
//
// One type for both shapes rather than two methods, because they are one question with one filter
// and one permission check - and because a client that switched between them would otherwise be
// switching between two code paths that have to stay in step.
type ItemQueryResult struct {
	Items  []work.WorkItem
	Info   PageInfo
	Groups []ItemGroup
	// Total is the size of the whole result, and is meaningful only when the query asked for an
	// exact count.
	Total int
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

	// SetAssignee writes the one person an entry is on, set or cleared, or reports a version
	// conflict.
	//
	// Its own method rather than a field of SetAttributes, for the reason SetCompletion is its own:
	// an assignment is one decision about one field, and writing it through the statement that also
	// writes the title would make handing an entry to somebody spend the version of a rename nobody
	// asked for. Whether that account may be assigned is decided before this runs (ADR-0005).
	SetAssignee(ctx context.Context, item work.WorkItem, expectedVersion int) error

	// CountOpenByAssignee counts the open entries each of the given accounts carries, tenant-wide
	// - LEAST_LOADED's material (domain-model.md §3.6). An account with no open entry is absent
	// from the answer, and the caller reads an absent key as zero.
	CountOpenByAssignee(ctx context.Context, accounts []shared.ID) (map[shared.ID]int, error)

	// SetCustomField writes one key of the entry's custom field document, or reports a version
	// conflict. Its own method for the reason SetCover is: one decision about one column, never
	// spending a rename's version.
	//
	// One key rather than the whole document, and that is a data-safety rule as much as a merge
	// one. The stored document may hold values whose definitions were deleted - visible to no
	// read, but kept (C-07) - and a write that replaced it with what a read answered would erase
	// them. The item carries the wanted state; what is written is the one key, taken from it: a
	// present key is set, an absent one removed. The version predicate is what makes two devices
	// writing two different keys resolve rather than overwrite (offline-sync.md §4.2).
	//
	// The definition's identity travels with the value: a read shows a value only while exactly
	// that definition lives, which is what keeps a deleted-and-recreated key from resurrecting
	// what the old one held (C-07).
	SetCustomField(
		ctx context.Context, item work.WorkItem, key string, definitionID shared.ID,
		expectedVersion int,
	) error

	// SetCover writes the cover, set or cleared, or reports a version conflict. Its own method
	// for the reason SetAssignee is: one decision about one field, never spending a rename's
	// version.
	SetCover(ctx context.Context, item work.WorkItem, expectedVersion int) error

	// SetDueDate writes the due trio, set or cleared whole, or reports a version conflict. Its
	// own method for the reason SetAssignee is: one decision about one date, never spending a
	// rename's version. The three columns travel together because none of them means anything
	// alone (D-01, i18n-l10n.md §4).
	SetDueDate(ctx context.Context, item work.WorkItem, expectedVersion int) error

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

	// Subtree returns everything below one entry, the entry itself excluded, parents before their
	// children - so that one pass over the rows always meets an entry after the one it hangs from,
	// which is what lets a copy carry its mapping from old identifier to new one forwards.
	//
	// Trashed rows are not in it: they are on their way out, and a copy carrying them would put
	// back what somebody deleted. Archived ones are, because being put away is a place rather than
	// a deletion.
	//
	// The limit is a bound rather than a page, and it is the caller's: a duplication is one
	// transaction, so a subtree bigger than the caller is willing to write in one is refused rather
	// than copied halfway. One row beyond the limit comes back when there is one, which is how the
	// caller tells "as large as allowed" from "larger than allowed".
	Subtree(ctx context.Context, item work.WorkItem, limit int) ([]work.WorkItem, error)

	// InsertCopy writes a new entry that carries another one's description: its title and notes,
	// the column and the person it was on, its cover and its custom field values.
	//
	// Its own method rather than more fields on Insert, because the two write different things. A
	// create writes what its use case owns and leaves the rest for the use case that owns it; a
	// copy writes what another row already carries, at once, because there is nothing to decide
	// about it a second time and writing it through five more statements would spend five versions
	// on an entry a moment old.
	//
	// What it does not carry is the completion - which names a person and a moment, and would be a
	// false record on an entry that person never touched - and the deletion stamps. The archive
	// stamp travels, so that an entry put away below the copied one is not silently brought back.
	InsertCopy(ctx context.Context, duplicate Copy) error

	// SetArchived writes an item's archive stamp, set or cleared, or reports a version conflict.
	//
	// One row, whatever sits below it. Archiving is a decision about one entry: a work package
	// under an archived task stays writable, because an item's children are entries in their own
	// right rather than a level of the structure - which is what makes this different from I-C3.
	SetArchived(ctx context.Context, item work.WorkItem, expectedVersion int) error

	// TrashSubtree moves an item and everything under it into the trash under one batch, and
	// reports how many rows went, the item included.
	//
	// A descendant already in the trash from an earlier deletion keeps that deletion and is not
	// counted: adopting it would restart its retention period and make a restore of this batch
	// bring back something nobody deleted this time (I-C2).
	TrashSubtree(ctx context.Context, trash ItemTrash) (int, error)

	// RestoreBatch takes one deletion back out of the trash, whole, and reports how many rows came
	// back. It serves a container's deletion as well as an item's: the items of a trashed
	// collection carry the container's batch.
	RestoreBatch(ctx context.Context, restore ItemTrash) (int, error)

	// Query answers the query language: an arbitrary filter over one anchored scope, sorted,
	// paged, and optionally grouped (B-12, api-guidelines.md §3).
	//
	// The only method here whose statement is not written in advance, and the reason ADR-0026
	// exists. What arrives has been through the grammar in core/domain/model/view, so every field
	// and every operator in it is one this installation declared; the adapter still emits nothing
	// but constants of its own and binds every value as a parameter.
	//
	// It judges no further than the query asks, as List does: whether the actor may see the scope
	// is decided in the application layer, once, before this runs (ADR-0005).
	Query(ctx context.Context, search ItemSearch) (ItemQueryResult, error)

	// Search answers the full text search: one page of entries in the order the database ranked
	// them, with the hub each one sits under (C-08).
	//
	// Ranked rather than sorted, which is what makes the cursor a keyset over the rank. The order
	// is `ts_rank_cd` descending and the identifier descending after it, so that entries matching
	// equally well come newest first - a UUIDv7 carries its own time (ADR-0034).
	Search(ctx context.Context, search TextSearch) (ItemHitPage, error)
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

	// ListFor returns the labels a page of entries carries, keyed by entry: what `expand=labels`
	// needs. One query for the whole page, because one per entry is a round trip per row - which
	// is the cost that makes a relation nobody asked for expensive, and why it is asked for rather
	// than always included.
	//
	// An entry that carries none is absent from the map rather than present with an empty slice.
	// The caller writes the empty list, because the caller is the one that knows the page.
	ListFor(ctx context.Context, itemIDs []shared.ID) (map[shared.ID][]shared.ID, error)

	// Elements returns every tag of one item's label set: what a merge compares a client's tags
	// against.
	Elements(ctx context.Context, itemID shared.ID) ([]work.SetElement, error)
}

// ItemMembers stores which accounts an item carries, and the tags that decide it after a merge.
//
// The same two-tables-behind-one-interface as ItemLabels, and for the same reason: `item_member` is
// the membership every read goes through and `set_element` is the OR-set tag that survives an
// offline merge (offline-sync.md §4.2, §10). Writing one without the other is the failure mode -
// membership with no tag merges as last writer wins and loses a concurrent change, a tag with no
// membership is invisible to every read - so neither is separately callable.
//
// Its own interface rather than a second set of methods on ItemLabels. The two sets share their
// merge machinery and nothing else: a label belongs to a collection's vocabulary and an account to
// the tenant, and the questions asked before writing one are not the questions asked before writing
// the other.
type ItemMembers interface {
	// List returns the accounts an item carries.
	List(ctx context.Context, itemID shared.ID) ([]shared.ID, error)

	// Add puts an account on an item and records the addition's tag. Adding one the item already
	// carries is the state the caller asked for and succeeds, writing a fresh tag.
	Add(ctx context.Context, itemID, accountID shared.ID, tag shared.HLC) error

	// Remove takes an account off an item, records the removal's tag, and reports whether the item
	// carried it at all. The tag is written either way: a device that removes somebody this replica
	// never saw added has still made a decision that another replica has to merge.
	Remove(ctx context.Context, itemID, accountID shared.ID, tag shared.HLC) (bool, error)

	// Elements returns every tag of one item's member set: what a merge compares a client's tags
	// against.
	Elements(ctx context.Context, itemID shared.ID) ([]work.SetElement, error)
}

// CommentPage is one page of one entry's discussion, oldest first.
type CommentPage struct {
	Comments []work.Comment
	Info     PageInfo
}

// Comments stores the discussion beside the entries (domain-model.md §3.5).
//
// There is deliberately no hard delete here: a comment leaves as a tombstone through SetDeleted,
// and what removes the row is the deletion of the item it belongs to, through the foreign key -
// exactly as the history goes.
type Comments interface {
	// Find returns the comment, tombstone or not, or ErrNotFound if it does not exist *for this
	// tenant*. Whether a deleted comment may be changed is the domain's question, not a query's.
	Find(ctx context.Context, id shared.ID) (work.Comment, error)

	// List returns one page of one entry's comments, oldest first - a conversation reads top
	// down. Tombstones are in it; the reader serves them without their body.
	List(ctx context.Context, itemID shared.ID, page Page) (CommentPage, error)

	// Insert writes a new comment.
	Insert(ctx context.Context, comment work.Comment) error

	// SetBody writes the rewritten text and the edit stamp, or reports a version conflict. A
	// tombstone is never matched: an edit racing a deletion loses to it.
	SetBody(ctx context.Context, comment work.Comment, expectedVersion int) error

	// SetDeleted writes the tombstone, or reports a version conflict. Also never matches one
	// that is already a tombstone - the caller decides idempotence on what it read.
	SetDeleted(ctx context.Context, comment work.Comment, expectedVersion int) error
}

// AutoAssignPolicies stores the assignment policy per scope (domain-model.md §3.6).
//
// One policy per scope, and the upsert is the whole write vocabulary: the policy is the
// `autoAssign` key of a container's policies document, which arrives complete (PUT semantics), so
// the row it maps to is written complete or removed. The rotation's state is the one exception,
// with a write of its own, because it is advanced by assignments rather than by configuration.
type AutoAssignPolicies interface {
	// FindForScope reads the policy at one scope, or ErrNotFound when none is configured there.
	FindForScope(ctx context.Context, scope work.AutoAssignScope, scopeID shared.ID) (work.AutoAssignPolicy, error)

	// Lock reads the same row and holds it for the rest of the transaction. The rotation reads
	// its cursor through this: two assignments arriving together must queue on the row, because a
	// cursor read hopefully is one turn handed to both of them.
	Lock(ctx context.Context, scope work.AutoAssignScope, scopeID shared.ID) (work.AutoAssignPolicy, error)

	// Upsert writes the whole definition, creating or replacing the scope's row. A replacement
	// resets the rotation: the state belonged to the pool that was configured, and a new pool
	// starts at its head.
	Upsert(ctx context.Context, policy work.AutoAssignPolicy) error

	// Delete removes the scope's policy. Removing one that is not there succeeds - that is the
	// state the caller asked for.
	Delete(ctx context.Context, scope work.AutoAssignScope, scopeID shared.ID) error

	// SaveState persists the advanced rotation, under the lock Lock took.
	SaveState(ctx context.Context, policy work.AutoAssignPolicy) error
}

// ItemTrash is everything one item's deletion or restore needs decided before it runs.
//
// The item arrives already stamped by the domain, as it does everywhere else in this port: the
// decision about what the row should say has been taken, and the adapter writes it.
type ItemTrash struct {
	// Item is the row the caller read, with the stamp the transition put on it - set on the way
	// in, cleared on the way out.
	Item work.WorkItem
	// Prefix is the item's own path. Every row of its subtree begins with it, which is what makes
	// a subtree deletion one statement rather than a walk (I-W2).
	Prefix string
	// BatchID is the deletion all of it belongs to. It is passed rather than read off the item,
	// because on the way out the item's own batch has already been cleared by the transition and
	// the rest of the batch still has to be found.
	BatchID shared.ID
	// ExpectedVersion locks the row the caller read. Zero means the caller read no version.
	ExpectedVersion int
}

// ContainerTrash is the same for a hub or a collection, whose deletion takes a subtree of two
// kinds with it (I-C2).
type ContainerTrash struct {
	Container       work.Container
	BatchID         shared.ID
	ExpectedVersion int
}

// Cascade is what a container's deletion or restore took with it.
//
// The collections are returned as identifiers rather than counted, because each is announced to
// offline clients in its own right: a device that subscribes to one collection rather than to the
// hub above it would otherwise never learn that its collection is gone (offline-sync.md §3.1). The
// items are counted, because they are announced through the collection they are in.
type Cascade struct {
	Collections []shared.ID
	Items       int
}

// TrashPage is one page of the trash, newest deletion first.
type TrashPage struct {
	Entries []work.TrashEntry
	Info    PageInfo
}

// Trash reads what is in the trash and finally removes it.
//
// Its own interface rather than methods on Containers and Items, because what it answers spans
// both: "what did I delete" is one question with one order and one cursor, and splitting it in two
// would leave the use case merging two pages and inventing a cursor that covers neither.
type Trash interface {
	// List returns one page of the trash: one entry per deletion, not one per deleted row. The
	// entry is the batch's root - the thing somebody deleted - and what came along with it is
	// reached through the batch.
	List(ctx context.Context, page Page) (TrashPage, error)

	// SubtreeIDs returns every identifier in one item's subtree, the item included.
	//
	// Read before a hard delete rather than derived from the cascade the schema would apply: each
	// row that goes needs a tombstone and a journal entry of its own (offline-sync.md §7,
	// data-retention.md §5), and rows removed by a foreign key nobody counted are exactly the
	// orphans the completeness rule forbids (ADR-0020 §6).
	SubtreeIDs(ctx context.Context, prefix string) ([]shared.ID, error)

	// PurgeItems removes items for good, by identifier, and reports how many rows went.
	PurgeItems(ctx context.Context, ids []shared.ID) (int, error)

	// PurgeContainers removes hubs and collections for good, by identifier.
	//
	// Collections before the hubs that hold them: `container.parent_id` is ON DELETE RESTRICT, so a
	// hub whose collections are still there refuses to go. That is the database insisting on the
	// order rather than a rule a caller could forget - but the caller still has to call it twice.
	PurgeContainers(ctx context.Context, ids []shared.ID) (int, error)
}

// CustomFields stores the definitions a workspace or a collection adds to its entries (C-07).
//
// Its own interface rather than methods on Items, because the two are not the same thing: this is
// the vocabulary, and `work_item.custom_fields` is what an entry says in it. A repository that
// held both would let a read path reach a statement written for the other.
type CustomFields interface {
	// Insert writes a new definition. A key already taken in the scope comes back as ErrConflict:
	// the unique index decides it, because a check followed by an insert is two statements with a
	// gap between them.
	Insert(ctx context.Context, definition work.CustomFieldDefinition) error

	// Find returns one definition by identifier, deleted or not. Whether a deleted one may still
	// be used is the domain's question.
	Find(ctx context.Context, id shared.ID) (work.CustomFieldDefinition, error)

	// FindInScope returns the live definition one collection sees under a key: the collection's
	// own, or the workspace-wide one. The collection's own wins, so that a team can narrow a
	// workspace-wide default rather than having to avoid its key.
	//
	// A zero collection asks about the workspace-wide scope alone.
	FindInScope(ctx context.Context, collectionID shared.ID, key string) (work.CustomFieldDefinition, error)

	// ListInScope returns every live definition in force for one collection: its own and the
	// workspace-wide ones above it, ordered by scope and then by key. A zero collection answers
	// the workspace-wide ones alone.
	//
	// Unpaged, deliberately. A workspace's vocabulary is small and bounded by what a person can
	// fill in on one form; a client renders the whole of it or none of it, and a cursor over a
	// list nobody scrolls would be machinery for its own sake.
	ListInScope(ctx context.Context, collectionID shared.ID) ([]work.CustomFieldDefinition, error)

	// SetAttributes writes what an edit may change, or reports a version conflict.
	SetAttributes(ctx context.Context, definition work.CustomFieldDefinition, expectedVersion int) error

	// SetDeleted marks the definition out of use, or reports a version conflict. A soft delete:
	// the values stay in the entries and stop being visible.
	SetDeleted(ctx context.Context, definition work.CustomFieldDefinition, expectedVersion int) error
}
