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
// nothing writes is a promise nothing keeps. The members and the assignee arrive with the
// assignment use cases in 0.3.0, the due date with 0.4.0, and the cover, the custom fields and the
// recurrence rule with the use cases that own them. Their columns exist and carry NULL until then.
//
// The labels are absent too, and for a different reason: they are a set rather than a field. They
// live in their own table with their own merge tags, because two devices adding two different
// labels at once is the case last writer wins over an array gets wrong (offline-sync.md §4.2) - so
// an item does not carry them, and AddLabel is what moves them.
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
	// BucketID is the column of the collection's board this item sits in, and empty for one that
	// sits in none. Capability BUCKET, which only a task carries: a board is the collection's, so
	// only the items directly in it have a place on it (domain-model.md §2).
	//
	// A scalar rather than a set, so it merges as last writer wins per field - two devices dragging
	// the same card to two columns is a genuine conflict with one answer, unlike two devices adding
	// two labels.
	BucketID shared.ID
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
	// BucketID is the column the item starts in. Checked against the profile like every other
	// optional field, so that a client cannot put a work package on a board.
	BucketID shared.ID

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

	if !in.BucketID.IsZero() {
		if err := in.Profile.Require(CapabilityBucket, "/bucket_id"); err != nil {
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
		BucketID:   in.BucketID,
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

// Field names, as the API spells them. They travel into the event's change set and into the change
// log, where a client matches them against the members it sent - so they are written once here
// rather than as a literal at each of the three places that has to agree.
const (
	FieldTitle    = "title"
	FieldNotes    = "notes"
	FieldBucketID = "bucket_id"
)

// ItemAttributes is what an update may change: the fields 0.2.0 owns.
//
// A nil pointer means "leave it alone", which is what an absent member of a JSON Merge Patch says.
// The distinction is the whole of the type's job: `notes: null` clears the notes and an omitted
// `notes` keeps them, and a struct of plain strings could not tell a caller that meant one from a
// caller that meant the other - it would clear the notes of every client that only wanted to
// rename something.
//
// Icon and colour are deliberately absent. The backlog names them, but a WorkItem has neither -
// domain-model.md §3.4 gives the item no such field and §3.3 puts both on Container, which is
// B-06's subject. Adding them here would be a model change rather than an implementation of one.
type ItemAttributes struct {
	Title *string
	Notes *string
	// BucketID is the column the item moves to, and a pointer to the zero identifier takes it off
	// the board altogether - which is the same "empty is not set" the text fields keep.
	BucketID *shared.ID
}

// IsEmpty reports whether the update asks for nothing at all.
func (a ItemAttributes) IsEmpty() bool {
	return a.Title == nil && a.Notes == nil && a.BucketID == nil
}

// FieldChange is one field that moved, with the value on each side.
//
// Reported out of the domain rather than recomputed by each recipient. Three of them describe the
// same change - the event's change set, one change log entry per field for the offline merge, and
// the audit entry - and three derivations would be three chances to disagree about what changed.
type FieldChange struct {
	Field    string
	From, To string
}

// Updated applies an update and reports which fields moved.
//
// Nothing that did not move is in the result, and a request that changes nothing returns the item
// untouched with no changes at all. That is what makes a repeat harmless rather than merely
// accepted: the caller writes nothing, spends no version and announces nothing, which is the same
// contract Completed keeps for the same reason.
//
// The capability is asked before the lifecycle, exactly as EnsureCompletable does and for the same
// reason: "an activity has no notes" is true of the type whatever state one particular activity is
// in, and answering with the state first would send a client off to unarchive an item whose notes
// would still be refused afterwards.
func (i WorkItem) Updated(
	attributes ItemAttributes, profile CapabilityProfile, at time.Time,
) (WorkItem, []FieldChange, error) {
	// Only a non-empty value needs the capability, which is the rule NewWorkItem already applies at
	// creation. An activity has no notes to begin with, so clearing them asks for the state it is
	// already in; refusing that would be a second answer to a question that has one.
	if attributes.Notes != nil && strings.TrimSpace(*attributes.Notes) != "" {
		if err := profile.Require(CapabilityNotes, "/notes"); err != nil {
			return WorkItem{}, nil, err
		}
	}
	// The same rule for the board: only a non-empty value needs the capability, because taking an
	// item off a board it was never on asks for the state it is already in.
	if attributes.BucketID != nil && !attributes.BucketID.IsZero() {
		if err := profile.Require(CapabilityBucket, "/bucket_id"); err != nil {
			return WorkItem{}, nil, err
		}
	}
	if err := i.EnsureEditable(); err != nil {
		return WorkItem{}, nil, err
	}

	var changes []FieldChange

	if attributes.Title != nil {
		title, err := itemTitle(*attributes.Title)
		if err != nil {
			return WorkItem{}, nil, err
		}
		if title != i.Title {
			changes = append(changes, FieldChange{Field: FieldTitle, From: i.Title, To: title})
			i.Title = title
		}
	}

	if attributes.Notes != nil {
		notes := strings.TrimSpace(*attributes.Notes)
		if notes != i.Notes {
			changes = append(changes, FieldChange{Field: FieldNotes, From: i.Notes, To: notes})
			i.Notes = notes
		}
	}

	if attributes.BucketID != nil && *attributes.BucketID != i.BucketID {
		changes = append(changes, FieldChange{
			Field: FieldBucketID, From: i.BucketID.String(), To: attributes.BucketID.String(),
		})
		i.BucketID = *attributes.BucketID
	}

	if len(changes) == 0 {
		return i, nil, nil
	}
	i.UpdatedAt = at
	return i, changes, nil
}

// EnsureEditable refuses an item whose state makes it read-only (I-W4).
//
// A conflict rather than a validation failure: the request is well formed and the state is what
// makes it impossible, which is the distinction that tells a client whether restoring the item
// first would help (api-guidelines.md §6).
func (i WorkItem) EnsureEditable() error {
	if i.IsTrashed() {
		return shared.ErrConflict.
			WithDetail("items.trashed").
			WithParams(map[string]string{"item_id": i.ID.String()})
	}
	if i.IsArchived() {
		return shared.ErrConflict.
			WithDetail("items.archived").
			WithParams(map[string]string{"item_id": i.ID.String()})
	}
	return nil
}

// EnsureCompletable refuses what cannot have its completion changed at all.
//
// Two reasons, and they are different kinds of answer. A type whose profile does not carry COMPLETION
// cannot be completed by anybody - that is the capability matrix, and it comes back as
// ErrCapabilityNotSupported naming the type and the capability (ADR-0006). A trashed or archived item
// cannot be *edited*, which is invariant I-W4 and a conflict: the request is well formed and the state
// is what makes it impossible, which is the distinction that tells a client whether restoring the item
// first would help (api-guidelines.md §6).
//
// The capability is checked before the lifecycle deliberately. "An activity has no completion" is true
// of the type whatever state one particular activity is in, and reporting the state first would send a
// client off to restore an item that still could not be completed afterwards.
func (i WorkItem) EnsureCompletable(profile CapabilityProfile) error {
	if err := profile.Require(CapabilityCompletion, "/completion"); err != nil {
		return err
	}
	return i.EnsureEditable()
}

// Completed returns the item marked done, by whom and when.
//
// Idempotent: an item that is already done comes back untouched, keeping the original timestamp and the
// original person. That is not politeness towards a client retrying - it is what invariant I-W5 asks of
// the roll-up, which may reach the same parent twice from two children, and it is what keeps
// `completed_by` the truth about who finished the work rather than about who last pressed the button.
//
// The caller decides what to do with an unchanged item: nothing is written, no version is spent and no
// event is announced, which is what makes a repeat harmless rather than merely accepted.
func (i WorkItem) Completed(by shared.ID, at time.Time) WorkItem {
	if i.Completion.IsCompleted {
		return i
	}

	completedAt := at.UTC()
	i.Completion = Completion{IsCompleted: true, CompletedAt: &completedAt, CompletedBy: by}
	i.UpdatedAt = at
	return i
}

// Reopened returns the item marked open again, and clears who completed it and when.
//
// Cleared rather than kept: the two fields answer "when was this finished, and by whom", and an open
// item has no answer. Keeping the old values would make `completed_at` a record of the last time it
// happened to be closed, which is what the activity history is for (B-11).
//
// Idempotent for the reason Completed is: reopening propagates upwards, and an item already open comes
// back untouched so that nothing is written.
func (i WorkItem) Reopened(at time.Time) WorkItem {
	if !i.Completion.IsCompleted {
		return i
	}

	i.Completion = Completion{}
	i.UpdatedAt = at
	return i
}

// ChildPath is where an item directly underneath this one would sit. Kept next to Path so that
// the two cannot drift: whoever builds a path and whoever reads it share this line.
func (i WorkItem) ChildPath(child shared.ID) string { return i.Path + child.String() + PathSeparator }

// Contains reports whether other sits inside this item's subtree, this item itself included.
//
// A prefix test on the materialised path, which is what the trailing separator on every path exists for: a
// path is `/a/b/` rather than `/a/b`, so `/ab/` cannot be read as being inside `/a/`. Without the separator
// this test would be quietly wrong for exactly the identifiers that share a prefix (I-W2).
//
// It is the whole of the cycle check. An item may not move under something inside its own subtree, and
// "inside its own subtree" is a string comparison rather than a walk - which is what makes the check cheap
// enough to run on every move rather than a thing that gets skipped for performance.
func (i WorkItem) Contains(other WorkItem) bool {
	return strings.HasPrefix(other.Path, i.Path)
}

// SubtreePathUnder is where this item's path lands when it moves under a parent whose path is parentPath.
// The empty parentPath is the top level of a collection.
//
// Kept beside Path and ChildPath so that the three cannot drift: whoever moves a subtree and whoever built
// its paths in the first place read the same lines.
func (i WorkItem) SubtreePathUnder(parentPath string) string {
	if parentPath == "" {
		parentPath = PathSeparator
	}
	return parentPath + i.ID.String() + PathSeparator
}

// RootPath is the path of an item directly under the collection: the top of a subtree.
func RootPath(id shared.ID) string { return PathSeparator + id.String() + PathSeparator }

// PathIDs reads a materialised path back into the chain it encodes: the entries from the top of the
// collection down to and including this one.
//
// The inverse of how a path is built, and it exists for the one question a stored path cannot answer
// otherwise: which entries sit above this one. A legal hold placed on a task has to reach the
// activities under it (data-retention.md §4.1), and walking the parent chain to find out would be a
// query per level for something the row already carries.
//
// A malformed path yields nothing rather than an error. Every path in the database was written by
// checkPlacement, so an unreadable one is a defect elsewhere - and the caller of this is a deletion,
// which must not be stopped by a chain it can only fail to widen.
func PathIDs(path string) []shared.ID {
	trimmed := strings.Trim(path, PathSeparator)
	if trimmed == "" {
		return nil
	}

	segments := strings.Split(trimmed, PathSeparator)
	ids := make([]shared.ID, 0, len(segments))
	for _, segment := range segments {
		id, err := shared.ParseID(segment)
		if err != nil {
			return nil
		}
		ids = append(ids, id)
	}
	return ids
}
