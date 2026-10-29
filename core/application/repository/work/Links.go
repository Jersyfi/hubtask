// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// LinkTypes stores a workspace's own link types (F-3). The six shipped ones have no row; the
// domain merges (work.NewLinkTypes).
type LinkTypes interface {
	// List returns the workspace's rows, archived ones included.
	List(ctx context.Context) ([]work.LinkType, error)
	// Find returns the row of a key, or ErrNotFound.
	Find(ctx context.Context, key string) (work.LinkType, error)
	// Insert writes a new row; a taken key comes back as `link_types.key_taken`.
	Insert(ctx context.Context, linkType work.LinkType) error
	// Update writes a row's names, position and archive under the optimistic lock.
	Update(ctx context.Context, linkType work.LinkType, expectedVersion int) error
	// Delete removes a type no link names; one in use comes back as `link_types.in_use`.
	Delete(ctx context.Context, key string) error
	// InUse reports whether any link names the type.
	InUse(ctx context.Context, key string) (bool, error)
}

// EntryLinks stores the links between entries (F-3, UC-PLN-01).
type EntryLinks interface {
	// Lock holds the workspace's links for the rest of the transaction, so that the cardinality
	// and the cycle rule are decided on a graph nobody else changes meanwhile.
	Lock(ctx context.Context) error
	// Insert writes a link; one that already stands comes back as `links.exists`, an end that
	// is not an entry of the workspace as ErrNotFound.
	Insert(ctx context.Context, link work.EntryLink) error
	// Find returns a link, or ErrNotFound.
	Find(ctx context.Context, id shared.ID) (work.EntryLink, error)
	// Delete removes a link, or reports ErrNotFound.
	Delete(ctx context.Context, id shared.ID) error
	// Of returns an entry's links from both ends, at most max.
	Of(ctx context.Context, entryID shared.ID, max int) ([]work.EntryLink, error)
	// Touching returns the links with an end among the entries, at most max.
	Touching(ctx context.Context, entryIDs []shared.ID, max int) ([]work.EntryLink, error)
	// CountFrom counts the links of a type an entry starts.
	CountFrom(ctx context.Context, sourceID shared.ID, linkType string) (int, error)
	// PathExists reports whether `to` is reachable from `from` over the forward types' links read
	// source to target and the backward types' read target to source.
	PathExists(ctx context.Context, from, to shared.ID, forward, backward []string) (bool, error)
}

// EntrySummary is what an entry shows where something points at it: enough to draw a mention or
// a card, and the collection and hub its reader's permission is judged on. A trashed entry has no
// summary.
type EntrySummary struct {
	ID           shared.ID
	CollectionID shared.ID
	HubID        shared.ID
	// Handle is the collection's, for the entry's key (`sup-128`); empty where it has none.
	Handle    string
	Level     work.ItemType
	EntryType string
	Title     string
	Number    int64
	Completed bool
	StartAt   *time.Time
	DueAt     *time.Time
	Archived  bool
}

// ContainerSummary is what a hub or a collection shows where something points at it.
type ContainerSummary struct {
	ID       shared.ID
	Kind     work.ContainerType
	ParentID shared.ID
	Name     string
	Handle   string
	Archived bool
}

// Backlink is one text that points at an object: an entry's notes or one of its comments.
type Backlink struct {
	// SourceKind is ENTRY for the notes, COMMENT for a comment.
	SourceKind string
	SourceID   shared.ID
	// EntryID is the entry the text belongs to.
	EntryID  shared.ID
	Anchor   string
	Strength reference.Strength
}

// References reads what the resolver and the backlinks need (P-19).
type References interface {
	// Entries returns the summaries of the entries among the identifiers that exist and are not
	// trashed, in no particular order.
	Entries(ctx context.Context, ids []shared.ID) ([]EntrySummary, error)
	// Containers returns the summaries of the hubs and collections among the identifiers.
	Containers(ctx context.Context, ids []shared.ID) ([]ContainerSummary, error)
	// Backlinks returns the texts that point at the object, at most max.
	Backlinks(ctx context.Context, target reference.Reference, max int) ([]Backlink, error)
}
