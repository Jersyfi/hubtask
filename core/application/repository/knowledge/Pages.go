// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package knowledge is the storage port of the knowledge module's pages (M3). Every method runs
// inside the caller's transaction, under the tenant it names (rule 3); another workspace's page is
// not found.
package knowledge

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Ancestor is a page above another, as a breadcrumb names it.
type Ancestor struct {
	ID    shared.ID
	Title string
}

// Hit is a page a search found, with the passage that matched.
type Hit struct {
	ID        shared.ID
	HubID     shared.ID
	ParentID  shared.ID
	Title     string
	Icon      string
	Excerpt   string
	Rank      float64
	UpdatedAt time.Time
}

// Summary is a page as a reference shows it.
type Summary struct {
	ID            shared.ID
	HubID         shared.ID
	Title         string
	Icon          string
	Archived      bool
	VerifiedUntil *time.Time
	// Readers is who of the hub's readers may read the page; every door showing a summary asks it.
	Readers knowledge.Readers
}

// Backlink is a text that points at an object: a page (HubID set) or an entry's notes or one of
// its comments (EntryID set, SourceKind ENTRY or COMMENT).
type Backlink struct {
	SourceKind string
	SourceID   shared.ID
	HubID      shared.ID
	EntryID    shared.ID
	Strength   string
}

// VersionRow is a version as a history lists it: without its body.
type VersionRow struct {
	Seq       int
	Title     string
	AuthorID  shared.ID
	Name      string
	Restored  int
	Size      int
	CreatedAt time.Time
}

// PageSearcher is the search port for pages (F-9), as TextSearcher is for entries: its own port so
// an external index can answer a search without becoming the store of pages. It answers candidates
// in the spaces it is handed - which spaces the reader may read stays with the use case (rule 2) -
// and for the one workspace the transaction is bound to.
type PageSearcher interface {
	// Search answers the pages of the spaces named that a text finds, best first.
	Search(ctx context.Context, text string, hubIDs []shared.ID, limit int) ([]Hit, error)
}

// Pages stores pages, their versions and their comments.
type Pages interface {
	Insert(ctx context.Context, page knowledge.Page) error
	// Find answers a page, trashed ones included.
	Find(ctx context.Context, id shared.ID) (knowledge.Page, error)
	// FindForUpdate answers a page and locks its row for the transaction.
	FindForUpdate(ctx context.Context, id shared.ID) (knowledge.Page, error)
	// Update writes the page if it still stands at the version expected; ErrVersionConflict if not.
	Update(ctx context.Context, page knowledge.Page, expected int) error
	// List answers a space's tree without bodies, at most limit.
	List(ctx context.Context, hubID shared.ID, includeArchived, templatesOnly bool, limit int) ([]knowledge.Page, error)
	// Trashed answers a space's trashed pages, newest first.
	Trashed(ctx context.Context, hubID shared.ID, limit int) ([]knowledge.Page, error)
	// Count answers how many pages a space holds, trashed ones included.
	Count(ctx context.Context, hubID shared.ID) (int, error)
	// Ancestors answers the pages above one, nearest first.
	Ancestors(ctx context.Context, id shared.ID) ([]Ancestor, error)
	// Subtree answers a page and the pages under it, the page first, bodies included.
	Subtree(ctx context.Context, id shared.ID, includeTrashed bool, limit int) ([]knowledge.Page, error)
	// LastOrderKey answers the last order key under a parent (empty for the top), or "" for none.
	LastOrderKey(ctx context.Context, hubID, parentID shared.ID) (string, error)
	// Trash marks a page and the pages under it trashed at a moment; Restore brings back those
	// trashed at that moment. Each answers how many pages it touched.
	Trash(ctx context.Context, id, by shared.ID, at time.Time) (int, error)
	Restore(ctx context.Context, id, by shared.ID, trashedAt, at time.Time) (int, error)
	PageSearcher
	// Summaries answers the pages among the identifiers that are not trashed.
	Summaries(ctx context.Context, ids []shared.ID) ([]Summary, error)
	// Backlinks answers the texts pointing at an object, at most limit.
	Backlinks(ctx context.Context, targetKind string, targetID shared.ID, limit int) ([]Backlink, error)

	// LastVersion answers a page's newest version; found false when it has none.
	LastVersion(ctx context.Context, pageID shared.ID) (knowledge.Version, bool, error)
	InsertVersion(ctx context.Context, version knowledge.Version) error
	// FoldVersion rewrites a version's state in place: a write of the same editing session.
	FoldVersion(ctx context.Context, version knowledge.Version) error
	NameVersion(ctx context.Context, pageID shared.ID, seq int, name string) error
	Versions(ctx context.Context, pageID shared.ID, before, limit int) ([]VersionRow, error)
	Version(ctx context.Context, pageID shared.ID, seq int) (knowledge.Version, error)

	InsertComment(ctx context.Context, comment knowledge.Comment) error
	FindComment(ctx context.Context, id shared.ID) (knowledge.Comment, error)
	Comments(ctx context.Context, pageID shared.ID, limit int) ([]knowledge.Comment, error)
	UpdateComment(ctx context.Context, comment knowledge.Comment, expected int) error

	// Readers answers, for each page among the identifiers, who of its hub's readers may read it:
	// the restrictions on its way up, combined. A page that is not there answers unrestricted.
	Readers(ctx context.Context, ids []shared.ID) (map[shared.ID]knowledge.Readers, error)
	// RestrictedTo answers the people a page's own restriction names.
	RestrictedTo(ctx context.Context, pageID shared.ID) ([]shared.ID, error)
	// Restrict writes a page's own restriction - the people, or nil to lift it - if the page still
	// stands at the version expected; ErrVersionConflict if not.
	Restrict(ctx context.Context, page knowledge.Page, accounts []shared.ID, expected int) error

	// InsertShare writes a share, false when the person already had it; DeleteShare takes one
	// away, false when there was none.
	InsertShare(ctx context.Context, share knowledge.Share) (bool, error)
	DeleteShare(ctx context.Context, pageID, accountID shared.ID) (bool, error)
	// Shares answers the people a page is shared with, at most limit.
	Shares(ctx context.Context, pageID shared.ID, limit int) ([]knowledge.Share, error)
	// SharedWith answers whether a page is shared with a person.
	SharedWith(ctx context.Context, pageID, accountID shared.ID) (bool, error)
	// PagesSharedWith answers the living pages shared with a person, newest share first.
	PagesSharedWith(ctx context.Context, accountID shared.ID, limit int) ([]Summary, error)

	// PersonalSpace answers the hub that is a person's own notes, found false when there is none
	// or it has gone to the trash.
	PersonalSpace(ctx context.Context, accountID shared.ID) (shared.ID, bool, error)
	// ClaimPersonalSpace names a hub a person's own notes and answers the one that is: the first
	// claim wins while its hub stands.
	ClaimPersonalSpace(ctx context.Context, accountID, hubID shared.ID, at time.Time) (shared.ID, error)
	// DailyNote answers the living daily note of a space for a day, found false when there is none.
	DailyNote(ctx context.Context, hubID shared.ID, day time.Time) (shared.ID, bool, error)
}
