// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Comment is one contribution to the discussion beside an entry (domain-model.md §3.5).
//
// Its own entity rather than a field of the item, because the two obey different merge rules:
// comments append and never merge (offline-sync.md §4.2, "appending lists"), so each is its own
// row with its own change log entries, and only an edit needs a rule at all - last writer wins
// over the body, with the displaced text not preserved. That mechanism belongs to §5 and to
// 0.8.5, and pretending to have it early would promise a recovery this milestone cannot keep.
type Comment struct {
	ID       shared.ID
	TenantID shared.ID
	ItemID   shared.ID
	AuthorID shared.ID
	// ParentCommentID is the comment this one replies to, empty for a top-level comment. One
	// level only (§3.5): a reply cannot itself be replied to, which keeps a thread a list rather
	// than a tree a client has to render recursively.
	ParentCommentID shared.ID
	// Body is the text, and empty exactly when the comment is deleted: a soft deletion keeps the
	// row so the thread stays readable, and clears the text because a deletion that only hid the
	// words would be retained personal content with no purpose (data-protection.md).
	Body      string
	CreatedAt time.Time
	// EditedAt is when the body was last rewritten, nil for never. Its own stamp rather than a
	// generic updated_at, because it is a fact the thread shows: "edited" beside a comment is
	// this field, and a deletion must not set it.
	EditedAt  *time.Time
	DeletedAt *time.Time
	Version   int
}

// MaxCommentBodyLength counts code points rather than bytes, for the reason the title limit does
// (I-W7): a limit in bytes would measure the alphabet rather than the text.
const MaxCommentBodyLength = 20000

// NewCommentInput is what a creation needs decided.
type NewCommentInput struct {
	ID       shared.ID
	TenantID shared.ID
	ItemID   shared.ID
	AuthorID shared.ID
	// Parent is the comment being replied to, already read by the caller - nil for a top-level
	// comment. The whole comment rather than its identifier, because the rules are about what it
	// is: on the same entry, not deleted, and not itself a reply.
	Parent *Comment
	Body   string
	Now    time.Time
}

// NewComment validates and builds a comment.
func NewComment(input NewCommentInput) (Comment, error) {
	body, err := validCommentBody(input.Body)
	if err != nil {
		return Comment{}, err
	}

	var parentID shared.ID
	if input.Parent != nil {
		parent := *input.Parent
		if parent.ItemID != input.ItemID {
			// A reply lives where what it replies to lives. A thread spanning two entries would
			// be two half-conversations, each missing the other.
			return Comment{}, shared.ErrValidation.
				WithDetail("comments.parent_not_on_item").
				WithFields(shared.FieldError{
					Path: "/parent_comment_id", Code: "comments.parent_not_on_item",
				})
		}
		if !parent.ParentCommentID.IsZero() {
			// One level of threading (§3.5). Refused rather than silently reattached to the top
			// of the thread: the caller said which comment they were answering, and storing a
			// different one would misquote them.
			return Comment{}, shared.ErrValidation.
				WithDetail("comments.reply_to_reply").
				WithFields(shared.FieldError{
					Path: "/parent_comment_id", Code: "comments.reply_to_reply",
				})
		}
		if parent.DeletedAt != nil {
			// The tombstone keeps the thread readable; it does not keep it open. A reply to a
			// deleted comment would answer words that are no longer there.
			return Comment{}, shared.ErrValidation.
				WithDetail("comments.parent_deleted").
				WithFields(shared.FieldError{
					Path: "/parent_comment_id", Code: "comments.parent_deleted",
				})
		}
		parentID = parent.ID
	}

	return Comment{
		ID:              input.ID,
		TenantID:        input.TenantID,
		ItemID:          input.ItemID,
		AuthorID:        input.AuthorID,
		ParentCommentID: parentID,
		Body:            body,
		CreatedAt:       input.Now,
		Version:         1,
	}, nil
}

// Edited returns the comment with its body rewritten.
//
// The displaced text is not preserved: an edit is last writer wins over the body
// (offline-sync.md §4.2), and the mechanism that files a displaced version belongs to §5 and to
// milestone 0.8.5. Editing a deleted comment is refused - the text is gone, and an edit that
// resurrected it would be an undelete nobody declared.
func (c Comment) Edited(body string, at time.Time) (Comment, error) {
	if c.DeletedAt != nil {
		return Comment{}, shared.ErrConflict.
			WithDetail("comments.comment_deleted").
			WithFields(shared.FieldError{Path: "/body", Code: "comments.comment_deleted"})
	}
	valid, err := validCommentBody(body)
	if err != nil {
		return Comment{}, err
	}

	c.Body = valid
	c.EditedAt = &at
	return c, nil
}

// Removed returns the comment as its tombstone: the text gone, the identity, the author and the
// timestamps kept, so that a reply does not dangle (C-03's acceptance). EditedAt survives - that
// the words had been rewritten is part of the thread's history, what they were is not.
//
// Idempotent: a comment already deleted comes back untouched, so that nothing is written, no
// version is spent and nothing is announced.
func (c Comment) Removed(at time.Time) Comment {
	if c.DeletedAt != nil {
		return c
	}
	c.Body = ""
	c.DeletedAt = &at
	return c
}

// validCommentBody applies the two body rules: not empty once trimmed, and at most
// MaxCommentBodyLength code points. Newlines are fine - a comment is prose, not a title - and the
// text is stored as sent apart from Unicode NFC normalisation, which the adapter applies for the
// reason container names get it: two spellings of the same word must compare equal.
func validCommentBody(body string) (string, error) {
	if strings.TrimSpace(body) == "" {
		return "", shared.ErrValidation.
			WithDetail("comments.body_required").
			WithFields(shared.FieldError{Path: "/body", Code: "comments.body_required"})
	}
	if utf8.RuneCountInString(body) > MaxCommentBodyLength {
		return "", shared.ErrValidation.
			WithDetail("comments.body_too_long").
			WithParams(map[string]string{"maximum": strconv.Itoa(MaxCommentBodyLength)}).
			WithFields(shared.FieldError{Path: "/body", Code: "comments.body_too_long"})
	}
	return body, nil
}

// EnsureCommentable refuses what cannot carry a discussion at all: a type whose profile does not
// carry COMMENTS - an activity, per the matrix in §2 - and a trashed or archived entry, in that
// order for the reason EnsureAssignable asks the capability first.
func (i WorkItem) EnsureCommentable(profile CapabilityProfile) error {
	if err := profile.Require(CapabilityComments, "/item_id"); err != nil {
		return err
	}
	return i.EnsureEditable()
}
