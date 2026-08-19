// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// bucketNameCodes are the codes the bucket's name rule reports with.
var bucketNameCodes = nameCodes{
	empty:     "buckets.name_empty",
	tooLong:   "buckets.name_too_long",
	malformed: "buckets.name_malformed",
}

// Bucket is a column of a collection's board: the "list" of the requirements
// (domain-model.md §3.5).
//
// It belongs to a collection and to nothing else, which is why it carries no parent beyond
// CollectionID and no type: a bucket under a hub would have no items to hold, and a bucket under
// an item would be a second hierarchy next to the one I-W2 already describes.
//
// What is deliberately absent: created_at, updated_at and created_by. The column has none of them
// (db/schema.sql), and a struct field with no column behind it is a value that reads back as zero
// - which is worse than not answering the question at all. Who moved a bucket and when is the
// activity history's answer (B-11), not this row's.
type Bucket struct {
	ID       shared.ID
	TenantID shared.ID
	// CollectionID is the collection whose board this bucket is a column of. Only a collection
	// has one; the application layer is what refuses a hub.
	CollectionID shared.ID
	Name         string
	// OrderKey ranks the bucket among the collection's other buckets: a fractional index, so that
	// two offline devices can insert a column without either one's order being discarded
	// (offline-sync.md §4.2).
	OrderKey string
	// WipLimit is how many items the people using this board agreed to have in the column at once.
	// Nil is "no limit", which is a different thing from zero - and zero is not a limit anybody
	// could satisfy, which is why the column refuses it.
	//
	// Nothing enforces it on a write. It is advisory by design: a limit that refused the drop
	// would make a board a form, and the requirement is that the column turns red rather than
	// that the work becomes impossible.
	WipLimit *int
	// IsDoneBucket marks the column that means "finished". Stored and reported; what reacts to it
	// is the client that renders the board and, later, an automation rule - the server completes
	// nothing on its own account here (domain-model.md §3.5).
	IsDoneBucket bool
	// ColorToken is optional, and the empty string is "not set" rather than a value - which is
	// what lets the adapter store NULL without a second flag per field.
	ColorToken string
	// DeletedAt is the soft delete. A bucket is deleted rather than trashed - it holds no content
	// of its own, so there is nothing for a retention period to protect - but the row stays, so
	// that a change log entry can still name it and a restore stays possible (offline-sync.md §7).
	DeletedAt *time.Time
	// Version is the optimistic lock. It starts at 1, which is what the column default says.
	Version int
}

// NewBucketInput is what a bucket is made of. The rank is computed by the ordering service rather
// than passed by a client, and arrives here already decided.
type NewBucketInput struct {
	ID           shared.ID
	TenantID     shared.ID
	CollectionID shared.ID
	Name         string
	OrderKey     string

	WipLimit     *int
	IsDoneBucket bool
	ColorToken   string
}

// NewBucket builds a bucket and checks its invariants (project-structure.md §3: constructors
// check, callers do not).
//
// Uniqueness of the name within the collection is not checked here and cannot be: this type sees
// one bucket. It is the unique index that decides it, translated by the adapter into
// `buckets.name_taken` - a check followed by an insert is two statements with a gap between them,
// and two requests arriving in that gap both pass the check (multi-tenancy.md §2.1).
func NewBucket(in NewBucketInput) (Bucket, error) {
	name, err := structureName(in.Name, bucketNameCodes)
	if err != nil {
		return Bucket{}, err
	}

	limit, err := wipLimit(in.WipLimit)
	if err != nil {
		return Bucket{}, err
	}

	token, err := colorToken(in.ColorToken, "buckets.color_token_malformed")
	if err != nil {
		return Bucket{}, err
	}

	// The identifiers and the rank come from ports, not from a client. Missing means the use case
	// was wired wrong, which is a defect rather than something the caller can fix (security.md §9).
	if in.ID.IsZero() || in.TenantID.IsZero() || in.CollectionID.IsZero() || in.OrderKey == "" {
		return Bucket{}, shared.ErrInternal.WithDetail("buckets.identity_incomplete")
	}

	return Bucket{
		ID:           in.ID,
		TenantID:     in.TenantID,
		CollectionID: in.CollectionID,
		Name:         name,
		OrderKey:     in.OrderKey,
		WipLimit:     limit,
		IsDoneBucket: in.IsDoneBucket,
		ColorToken:   token,
		Version:      1,
	}, nil
}

// wipLimit checks a work in progress limit. Zero clears it, which is what a caller means by
// sending nothing: the column's CHECK refuses zero, so no value is lost by reading it that way,
// and it saves every layer above from carrying a second flag beside the number.
func wipLimit(wanted *int) (*int, error) {
	switch {
	case wanted == nil, *wanted == 0:
		return nil, nil
	case *wanted < 0:
		return nil, shared.ErrValidation.
			WithDetail("buckets.wip_limit_invalid").
			WithParams(map[string]string{"value": strconv.Itoa(*wanted)}).
			WithFields(shared.FieldError{Path: "/wip_limit", Code: "buckets.wip_limit_invalid"})
	}
	limit := *wanted
	return &limit, nil
}

// IsDeleted reports whether the bucket has been removed from its collection's board.
func (b Bucket) IsDeleted() bool { return b.DeletedAt != nil }

// BucketAttributes is what an update may change.
//
// A pointer per field, so that "set it to nothing" and "do not touch it" stay two different
// requests all the way down from the merge patch that expressed them (offline-sync.md §4.2). The
// rank is not here: where a bucket sits is ReorderBucket's decision, with its own endpoint and its
// own neighbour lookup, and a patch that quietly reordered a board would be a second answer to
// that question.
type BucketAttributes struct {
	Name       *string
	ColorToken *string
	// WipLimit is a pointer to a number of which zero means "clear it", exactly as the empty
	// string does for the text fields. Zero is not a limit the column can hold, so nothing is lost
	// by reading it that way - and the alternative, a pointer to a pointer, is a type nobody reads
	// correctly twice.
	WipLimit     *int
	IsDoneBucket *bool
}

// IsEmpty reports whether the update asks for nothing at all.
func (a BucketAttributes) IsEmpty() bool {
	return a.Name == nil && a.ColorToken == nil && a.WipLimit == nil && a.IsDoneBucket == nil
}

// Updated applies a change and reports which fields moved.
//
// Nothing that did not move is in the result, and a request that changes nothing returns the
// bucket untouched with no changes at all - the contract Container.Renamed and WorkItem.Updated
// both keep, and for the same reason: the caller writes nothing, spends no version and announces
// nothing, which is what makes a repeat harmless rather than merely accepted.
func (b Bucket) Updated(attributes BucketAttributes) (Bucket, []FieldChange, error) {
	if err := b.EnsureEditable(); err != nil {
		return Bucket{}, nil, err
	}

	var changes []FieldChange

	if attributes.Name != nil {
		name, err := structureName(*attributes.Name, bucketNameCodes)
		if err != nil {
			return Bucket{}, nil, err
		}
		if name != b.Name {
			changes = append(changes, FieldChange{Field: FieldName, From: b.Name, To: name})
			b.Name = name
		}
	}

	if attributes.ColorToken != nil {
		token, err := colorToken(*attributes.ColorToken, "buckets.color_token_malformed")
		if err != nil {
			return Bucket{}, nil, err
		}
		if token != b.ColorToken {
			changes = append(changes,
				FieldChange{Field: FieldColorToken, From: b.ColorToken, To: token})
			b.ColorToken = token
		}
	}

	if attributes.WipLimit != nil {
		limit, err := wipLimit(attributes.WipLimit)
		if err != nil {
			return Bucket{}, nil, err
		}
		if formatLimit(limit) != formatLimit(b.WipLimit) {
			changes = append(changes, FieldChange{
				Field: FieldWipLimit, From: formatLimit(b.WipLimit), To: formatLimit(limit),
			})
			b.WipLimit = limit
		}
	}

	if attributes.IsDoneBucket != nil && *attributes.IsDoneBucket != b.IsDoneBucket {
		changes = append(changes, FieldChange{
			Field: FieldIsDoneBucket,
			From:  strconv.FormatBool(b.IsDoneBucket), To: strconv.FormatBool(*attributes.IsDoneBucket),
		})
		b.IsDoneBucket = *attributes.IsDoneBucket
	}

	if len(changes) == 0 {
		return b, nil, nil
	}
	return b, changes, nil
}

// formatLimit is how a work in progress limit travels in a change set: the number, or the empty
// string for "no limit". The empty string rather than "0", because every other field in a change
// set spells "not set" that way, and a recipient that reads 0 as a limit would render a column
// nobody may drop into.
func formatLimit(limit *int) string {
	if limit == nil {
		return ""
	}
	return strconv.Itoa(*limit)
}

// Reordered returns the bucket at a new rank among its collection's buckets, and reports the move.
//
// Idempotent: a bucket asked for the rank it already holds comes back untouched, so that nothing
// is written and nothing is announced.
func (b Bucket) Reordered(orderKey string) (Bucket, []FieldChange, error) {
	if err := b.EnsureEditable(); err != nil {
		return Bucket{}, nil, err
	}
	if orderKey == "" {
		// The rank comes from the ordering service. Empty means the use case was wired wrong.
		return Bucket{}, nil, shared.ErrInternal.WithDetail("buckets.identity_incomplete")
	}
	if orderKey == b.OrderKey {
		return b, nil, nil
	}

	changes := []FieldChange{{Field: FieldOrderKey, From: b.OrderKey, To: orderKey}}
	b.OrderKey = orderKey
	return b, changes, nil
}

// Deleted takes the bucket off the board.
//
// A soft delete, and idempotent: a bucket already deleted comes back untouched, which is what
// makes a retry after a lost response harmless. Where its items go is the use case's decision -
// this type sees one bucket and cannot answer for the collection's other columns.
func (b Bucket) Deleted(at time.Time) (Bucket, []FieldChange, error) {
	if b.IsDeleted() {
		return b, nil, nil
	}

	changes := []FieldChange{{Field: FieldDeletedAt, From: "", To: instant(at)}}
	b.DeletedAt = &at
	return b, changes, nil
}

// EnsureEditable refuses a bucket that is no longer on the board.
//
// A conflict rather than a validation failure: the request is well formed and the state is what
// makes it impossible, which is the distinction that tells a client whether anything it could do
// would help (api-guidelines.md §6). Whether the collection above it is archived is a question
// about the collection, and the use case asks Container.EnsureEditable for it.
func (b Bucket) EnsureEditable() error {
	if b.IsDeleted() {
		return shared.ErrConflict.
			WithDetail("buckets.deleted").
			WithParams(map[string]string{"bucket_id": b.ID.String()})
	}
	return nil
}
