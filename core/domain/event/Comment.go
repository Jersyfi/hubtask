// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package event

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// NewCommentCreated announces a new contribution to an entry's discussion (domain-model.md §4).
func NewCommentCreated(id shared.ID, comment work.Comment, collectionID shared.ID,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newCommentEvent(id, CommentCreated, comment, collectionID, actor, occurredAt, cause)
}

// NewCommentUpdated announces that a comment's text was rewritten. The payload is the comment
// after the edit - the displaced text is not preserved (offline-sync.md §4.2).
func NewCommentUpdated(id shared.ID, comment work.Comment, collectionID shared.ID,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return newCommentEvent(id, CommentUpdated, comment, collectionID, actor, occurredAt, cause)
}

// NewCommentDeleted announces a comment's tombstone.
func NewCommentDeleted(id shared.ID, comment work.Comment, collectionID shared.ID,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	if comment.DeletedAt == nil {
		// A deletion event about a living comment is the writer and the event disagreeing, which
		// is a defect rather than something a client sent (security.md §9).
		return Envelope{}, shared.ErrInternal.WithDetail("events.comment_not_deleted")
	}
	return newCommentEvent(id, CommentDeleted, comment, collectionID, actor, occurredAt, cause)
}

func newCommentEvent(id shared.ID, eventType Type, comment work.Comment, collectionID shared.ID,
	actor Actor, occurredAt time.Time, cause Cause,
) (Envelope, error) {
	return NewEnvelope(id, eventType, comment.TenantID,
		CommentSubject(comment.ID), actor, occurredAt, cause,
		CommentPayload(comment, collectionID))
}

// CommentPayload is the comment as every recipient reads it: the event outwards and the change
// log to synchronising clients describe the same state, and building it twice is how the two come
// to disagree. The body is null exactly when the comment is deleted - the tombstone's shape, the
// same one the contract's Comment schema declares.
func CommentPayload(comment work.Comment, collectionID shared.ID) map[string]any {
	payload := map[string]any{
		"id":                comment.ID.String(),
		"item_id":           comment.ItemID.String(),
		"collection_id":     collectionID.String(),
		"author_id":         comment.AuthorID.String(),
		"parent_comment_id": nil,
		"body":              nil,
		"created_at":        comment.CreatedAt.UTC(),
		"edited_at":         nil,
		"deleted_at":        nil,
		"version":           comment.Version,
	}
	if !comment.ParentCommentID.IsZero() {
		payload["parent_comment_id"] = comment.ParentCommentID.String()
	}
	if comment.DeletedAt == nil {
		payload["body"] = comment.Body
	}
	if comment.EditedAt != nil {
		payload["edited_at"] = comment.EditedAt.UTC()
	}
	if comment.DeletedAt != nil {
		payload["deleted_at"] = comment.DeletedAt.UTC()
	}
	return payload
}

// CommentSubject is what a comment event is about, kept next to the events so the two cannot
// drift.
func CommentSubject(id shared.ID) string { return "comment/" + id.String() }
