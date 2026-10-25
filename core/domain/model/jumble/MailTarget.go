// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import "github.com/Jersyfi/hubtask/core/domain/model/shared"

// TargetKind is where a mail is meant to go (ADR-0094): the jumble, a collection or an entry.
type TargetKind string

const (
	// TargetJumble is the workspace's jumble: whatever arrives becomes a jumble entry.
	TargetJumble TargetKind = "JUMBLE"
	// TargetCollection is a collection: a mail becomes an entry in it.
	TargetCollection TargetKind = "COLLECTION"
	// TargetItem is an entry: a mail becomes a comment on it.
	TargetItem TargetKind = "ITEM"
)

// Valid reports whether the kind is one the column allows.
func (k TargetKind) Valid() bool {
	switch k {
	case TargetJumble, TargetCollection, TargetItem:
		return true
	default:
		return false
	}
}

// MailTarget names one place. The jumble has no identifier - there is one per workspace.
type MailTarget struct {
	Kind TargetKind
	ID   shared.ID
}

// IsZero reports whether no place is named.
func (t MailTarget) IsZero() bool { return t.Kind == "" }

// NewMailTarget validates a place: the jumble without an identifier, the others with one.
func NewMailTarget(kind TargetKind, id shared.ID) (MailTarget, error) {
	switch {
	case !kind.Valid():
		return MailTarget{}, shared.ErrValidation.WithDetail("mail.target_unknown").
			WithFields(shared.FieldError{Path: "/target_kind", Code: "mail.target_unknown"})
	case kind == TargetJumble:
		return MailTarget{Kind: kind}, nil
	case id.IsZero():
		return MailTarget{}, shared.ErrValidation.WithDetail("mail.target_required").
			WithFields(shared.FieldError{Path: "/target_id", Code: "mail.target_required"})
	default:
		return MailTarget{Kind: kind, ID: id}, nil
	}
}
