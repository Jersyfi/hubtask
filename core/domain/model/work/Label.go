// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// labelNameCodes are the codes the label's name rule reports with.
var labelNameCodes = nameCodes{
	empty:     "labels.name_empty",
	tooLong:   "labels.name_too_long",
	malformed: "labels.name_malformed",
}

// Label is a tag a collection defines and its items carry (domain-model.md §3.5).
//
// Defined on the collection rather than on the tenant, deliberately: a label is a vocabulary the
// people working in one collection agree on, and a tenant-wide list would make every collection
// pay for every other's vocabulary. What that costs is that a label does not survive a move to
// another collection - which is invariant I-W6, reported back rather than silently dropped.
//
// The colour is required, unlike a bucket's. A label is rendered as a chip and nothing else: with
// no colour there is nothing to render it as, and a client would have to invent one - which is how
// two clients come to render the same label differently.
type Label struct {
	ID           shared.ID
	TenantID     shared.ID
	CollectionID shared.ID
	Name         string
	// ColorToken is a theme token rather than a colour value, so that clients render it in their
	// own palette (domain-model.md §3.5).
	ColorToken string
	// Description is optional; the empty string is "not set" rather than a value.
	Description string
	// DeletedAt is the soft delete. The row stays so that a change log entry can still name it and
	// the items that carry the label keep a reference that resolves (offline-sync.md §7).
	DeletedAt *time.Time
	Version   int
}

// NewLabelInput is what a label is made of.
type NewLabelInput struct {
	ID           shared.ID
	TenantID     shared.ID
	CollectionID shared.ID
	Name         string
	ColorToken   string
	Description  string
}

// NewLabel builds a label and checks its invariants.
//
// Uniqueness within the collection is the unique index's decision rather than this type's, for the
// reason given at NewBucket: a check followed by an insert is two statements with a gap between
// them, and two requests arriving in that gap both pass the check.
func NewLabel(in NewLabelInput) (Label, error) {
	name, err := structureName(in.Name, labelNameCodes)
	if err != nil {
		return Label{}, err
	}

	token, err := labelColorToken(in.ColorToken)
	if err != nil {
		return Label{}, err
	}

	if in.ID.IsZero() || in.TenantID.IsZero() || in.CollectionID.IsZero() {
		return Label{}, shared.ErrInternal.WithDetail("labels.identity_incomplete")
	}

	return Label{
		ID:           in.ID,
		TenantID:     in.TenantID,
		CollectionID: in.CollectionID,
		Name:         name,
		ColorToken:   token,
		Description:  strings.TrimSpace(in.Description),
		Version:      1,
	}, nil
}

// labelColorToken is the bucket's rule with the empty case closed: a label has to have a colour.
func labelColorToken(raw string) (string, error) {
	token, err := colorToken(raw, "labels.color_token_malformed")
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", shared.ErrValidation.
			WithDetail("labels.color_token_empty").
			WithFields(shared.FieldError{Path: "/color_token", Code: "labels.color_token_empty"})
	}
	return token, nil
}

// IsDeleted reports whether the label has been removed from its collection's vocabulary.
func (l Label) IsDeleted() bool { return l.DeletedAt != nil }

// LabelAttributes is what an update may change. A pointer per field, so that "set it to nothing"
// and "do not touch it" stay two different requests.
//
// The colour has no "set it to nothing": a label without one cannot be rendered, so an empty
// colour is refused rather than stored.
type LabelAttributes struct {
	Name        *string
	ColorToken  *string
	Description *string
}

// IsEmpty reports whether the update asks for nothing at all.
func (a LabelAttributes) IsEmpty() bool {
	return a.Name == nil && a.ColorToken == nil && a.Description == nil
}

// Updated applies a change and reports which fields moved. Nothing that did not move is in the
// result, and a request that changes nothing returns the label untouched.
func (l Label) Updated(attributes LabelAttributes) (Label, []FieldChange, error) {
	if err := l.EnsureEditable(); err != nil {
		return Label{}, nil, err
	}

	var changes []FieldChange

	if attributes.Name != nil {
		name, err := structureName(*attributes.Name, labelNameCodes)
		if err != nil {
			return Label{}, nil, err
		}
		if name != l.Name {
			changes = append(changes, FieldChange{Field: FieldName, From: l.Name, To: name})
			l.Name = name
		}
	}

	if attributes.ColorToken != nil {
		token, err := labelColorToken(*attributes.ColorToken)
		if err != nil {
			return Label{}, nil, err
		}
		if token != l.ColorToken {
			changes = append(changes,
				FieldChange{Field: FieldColorToken, From: l.ColorToken, To: token})
			l.ColorToken = token
		}
	}

	if attributes.Description != nil {
		description := strings.TrimSpace(*attributes.Description)
		if description != l.Description {
			if hasControlCharacter(description) {
				return Label{}, nil, shared.ErrValidation.
					WithDetail("labels.description_malformed").
					WithFields(shared.FieldError{
						Path: "/description", Code: "labels.description_malformed",
					})
			}
			changes = append(changes,
				FieldChange{Field: FieldDescription, From: l.Description, To: description})
			l.Description = description
		}
	}

	if len(changes) == 0 {
		return l, nil, nil
	}
	return l, changes, nil
}

// Deleted takes the label out of the collection's vocabulary.
//
// A soft delete, and idempotent. What happens to the items carrying it is the use case's decision:
// this type sees one label and knows nothing about who wears it.
func (l Label) Deleted(at time.Time) (Label, []FieldChange, error) {
	if l.IsDeleted() {
		return l, nil, nil
	}

	changes := []FieldChange{{Field: FieldDeletedAt, From: "", To: instant(at)}}
	l.DeletedAt = &at
	return l, changes, nil
}

// EnsureEditable refuses a label that is no longer in the collection's vocabulary. A conflict
// rather than a validation failure, for the reason Bucket.EnsureEditable gives.
func (l Label) EnsureEditable() error {
	if l.IsDeleted() {
		return shared.ErrConflict.
			WithDetail("labels.deleted").
			WithParams(map[string]string{"label_id": l.ID.String()})
	}
	return nil
}
