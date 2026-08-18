// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// itemTypes is the closed set, in the order of the constants in CapabilityProfile.go.
var itemTypes = [...]ItemType{ItemTask, ItemWorkPackage, ItemActivity}

// ItemTypes returns every defined type. An adapter uses it to prove it handles all of them.
func ItemTypes() []ItemType { return itemTypes[:] }

// Valid reports whether the type is one of the defined ones.
//
// Deliberately a closed set even though the profiles are data: the profile decides what a type
// may *do*, the enum decides which types the schema knows. A profile row naming a type the column
// cannot store would fail at the insert, and this says so earlier.
func (t ItemType) Valid() bool {
	for _, known := range itemTypes {
		if known == t {
			return true
		}
	}
	return false
}

// MaxItemTitleLength counts code points rather than bytes, for the reason given at
// MaxContainerNameLength: a limit in bytes draws a line a user cannot see (I-W7).
const MaxItemTitleLength = 500

// PathSeparator separates the identifiers of a materialised path.
const PathSeparator = "/"

// Completion is the done/open state, together with who closed it and when (domain-model.md §3.4).
//
// One value rather than three fields, because the three are only ever meaningful together: a
// completed item without a timestamp, or a timestamp on an open one, are states nothing should be
// able to express. CompleteWorkItem (B-07) is what moves it.
type Completion struct {
	IsCompleted bool
	CompletedAt *time.Time
	CompletedBy shared.ID
}

// WorkItem is the aggregate root: a task, a work package, or an activity (domain-model.md §3.4).
//
// One type for all three levels is the decision of ADR-0006. What a given item may do is not in
// this struct but in its CapabilityProfile, which is why a field being present here does not mean
// every item may carry it - the profile decides, and Require refuses what it does not allow.
//
// What is deliberately absent, on the same reasoning that kept `policies` off Container: a field
// nothing writes is a promise nothing keeps. `bucket_id` and the labels arrive with B-09, the
// members and the assignee with the assignment use cases in 0.3.0, the due date with 0.4.0, and
// the cover, the custom fields and the recurrence rule with the use cases that own them. Their
// columns exist and carry NULL until then.
type WorkItem struct {
	ID       shared.ID
	TenantID shared.ID
	// CollectionID is the collection the item belongs to, denormalised onto every level of the
	// subtree rather than resolved through the parent chain: it is what a board query filters on
	// and what row level security compares, and a join per row would be paid on every read
	// (domain-model.md §3.4).
	CollectionID shared.ID
	Type         ItemType
	// ParentID is the item above, and empty for a task - whose parent is the collection rather
	// than another item (I-W1).
	ParentID shared.ID
	// Path is the materialised path, `/<taskId>/<wpId>/…/`, including the item itself and closed
	// by a separator. Subtree queries are then a prefix comparison against an index rather than a
	// recursive walk, and the trailing separator is what stops one identifier's prefix from
	// matching another's (I-W2).
	Path string
	// Depth is the level below the collection, counting from 1. Derived from Path and stored, so
	// that a depth limit can be checked without parsing a string per row.
	Depth int
	Title string
	// Notes is Markdown, unrendered on the server. Capability NOTES, so an activity has none.
	Notes      string
	Completion Completion
	// OrderKey ranks the item among its siblings: a fractional index, so that two offline devices
	// can insert into the same list without either one's order being discarded (offline-sync.md §4.2).
	OrderKey     string
	ArchivedAt   *time.Time
	DeletedAt    *time.Time
	TrashBatchID shared.ID
	CreatedBy    shared.ID
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int
}

// NewWorkItemInput is what an item is made of. Placement - the path and the depth - is computed by
// the hierarchy service rather than passed by a client, and arrives here already decided.
type NewWorkItemInput struct {
	ID           shared.ID
	TenantID     shared.ID
	CollectionID shared.ID
	Type         ItemType
	ParentID     shared.ID

	Title string
	Notes string

	// Profile is the capability profile in force for this type, which is data rather than code
	// (ADR-0006) and therefore has to be handed in. It decides which of the optional fields above
	// this item may carry at all.
	Profile CapabilityProfile

	Path      string
	Depth     int
	OrderKey  string
	CreatedBy shared.ID
	Now       time.Time
}

// NewWorkItem builds an item and checks its invariants.
//
// The capability gate runs here rather than in the use case, so that every path into an item -
// this one today, an import or a template instantiation later - passes the same check. A use case
// that wrote its own version of it would be the second answer to a question that must have one.
func NewWorkItem(in NewWorkItemInput) (WorkItem, error) {
	if !in.Type.Valid() {
		return WorkItem{}, shared.ErrValidation.
			WithDetail("items.type_unknown").
			WithParams(map[string]string{"value": string(in.Type)}).
			WithFields(shared.FieldError{Path: "/type", Code: "items.type_unknown"})
	}
	if in.Profile.Type != in.Type {
		// The caller resolved the profile of a different type. Nothing a client sent could have
		// caused it, so it is a defect rather than input (security.md §9).
		return WorkItem{}, shared.ErrInternal.WithDetail("items.profile_mismatched")
	}

	title, err := itemTitle(in.Title)
	if err != nil {
		return WorkItem{}, err
	}

	notes := strings.TrimSpace(in.Notes)
	if notes != "" {
		if err := in.Profile.Require(CapabilityNotes, "/notes"); err != nil {
			return WorkItem{}, err
		}
	}

	if err := checkPlacement(in.ID, in.ParentID, in.Path, in.Depth); err != nil {
		return WorkItem{}, err
	}

	if in.ID.IsZero() || in.TenantID.IsZero() || in.CollectionID.IsZero() ||
		in.CreatedBy.IsZero() || in.OrderKey == "" {
		return WorkItem{}, shared.ErrInternal.WithDetail("items.identity_incomplete")
	}

	return WorkItem{
		ID:           in.ID,
		TenantID:     in.TenantID,
		CollectionID: in.CollectionID,
		Type:         in.Type,
		ParentID:     in.ParentID,
		Path:         in.Path,
		Depth:        in.Depth,
		Title:        title,
		Notes:        notes,
		// An item starts open. There is no way to create a completed one, and that is deliberate:
		// completion is an event with a time and an actor, and inventing one at creation would
		// put a lie in the history (I-W5, B-07).
		Completion: Completion{},
		OrderKey:   in.OrderKey,
		CreatedBy:  in.CreatedBy,
		CreatedAt:  in.Now,
		UpdatedAt:  in.Now,
		Version:    1,
	}, nil
}

// checkPlacement holds the parent, the path and the depth to one another (I-W2).
//
// All three are derived by the hierarchy service, which is also what refused the placements a
// client is not allowed to ask for - so an inconsistency reaching this point is a defect in the
// derivation rather than something a caller sent, and it is reported as one.
//
// It is checked all the same, because a path that does not end in the item's own identifier makes
// every subtree query below it wrong, and nothing downstream would notice: the row inserts, the
// response looks right, and the children simply fail to appear.
func checkPlacement(id, parentID shared.ID, path string, depth int) error {
	malformed := shared.ErrInternal.
		WithDetail("items.path_malformed").
		WithParams(map[string]string{"path": path})

	if !strings.HasPrefix(path, PathSeparator) || !strings.HasSuffix(path, PathSeparator) ||
		path == PathSeparator {
		return malformed
	}
	segments := strings.Split(strings.Trim(path, PathSeparator), PathSeparator)

	switch {
	case segments[len(segments)-1] != id.String():
		return malformed
	case depth != len(segments):
		return shared.ErrInternal.
			WithDetail("items.depth_inconsistent").
			WithParams(map[string]string{"path": path})

	// A root item sits directly under the collection, so its path is one segment long and it
	// names no parent item. Which types may be roots is the hierarchy service's decision; that
	// the two answers agree is this one's.
	case parentID.IsZero() && depth != 1:
		return shared.ErrInternal.WithDetail("items.parent_item_required")
	case !parentID.IsZero() && (depth < 2 || segments[len(segments)-2] != parentID.String()):
		return malformed
	}
	return nil
}

// itemTitle trims and checks the title. One line, like a container's name and for the same
// reason: it survives every layer and then breaks the one that renders it - an export, a log
// line, a calendar summary. Anything with newlines in it belongs in the notes.
func itemTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)

	switch {
	case title == "":
		return "", shared.ErrValidation.
			WithDetail("items.title_empty").
			WithFields(shared.FieldError{Path: "/title", Code: "items.title_empty"})

	case utf8.RuneCountInString(title) > MaxItemTitleLength:
		return "", shared.ErrValidation.
			WithDetail("items.title_too_long").
			WithParams(map[string]string{"maximum": "500"}).
			WithFields(shared.FieldError{Path: "/title", Code: "items.title_too_long"})

	case hasControlCharacter(title):
		return "", shared.ErrValidation.
			WithDetail("items.title_malformed").
			WithFields(shared.FieldError{Path: "/title", Code: "items.title_malformed"})
	}
	return title, nil
}

// IsArchived reports whether the item is archived: kept, and read-only (I-W4).
func (i WorkItem) IsArchived() bool { return i.ArchivedAt != nil }

// IsTrashed reports whether the item is in the trash - a deletion waiting out its retention
// period, which is a different thing from being archived.
func (i WorkItem) IsTrashed() bool { return i.DeletedAt != nil }

// ChildPath is where an item directly underneath this one would sit. Kept next to Path so that
// the two cannot drift: whoever builds a path and whoever reads it share this line.
func (i WorkItem) ChildPath(child shared.ID) string { return i.Path + child.String() + PathSeparator }

// RootPath is the path of an item directly under the collection: the top of a subtree.
func RootPath(id shared.ID) string { return PathSeparator + id.String() + PathSeparator }
