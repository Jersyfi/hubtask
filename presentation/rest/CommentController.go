// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The discussion beside the entries (C-03): a sub-resource of the item, because a comment is not
// a field of its row - it appends, it never merges, and it pages on its own.

const (
	addCommentUseCase    = "AddComment"
	listCommentsUseCase  = "ListComments"
	editCommentUseCase   = "EditComment"
	deleteCommentUseCase = "DeleteComment"
)

// ListComments answers GET /items/{itemId}/comments.
func (c *RestController) ListComments(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
	params openapi.ListCommentsParams,
) {
	out, ok := c.read(w, r, listCommentsUseCase, usecase.Input{
		"item_id": itemID.String(),
		"cursor":  optionalStringField(params.Cursor),
		"size":    optionalIntField(params.Size),
	})
	if !ok {
		return
	}

	page := openapi.CommentPage{Data: []openapi.Comment{}, Page: pageResponse(out)}
	for _, row := range rowsOf(out) {
		page.Data = append(page.Data, commentResponse(row))
	}
	writeJSON(w, r, http.StatusOK, page)
}

// AddComment answers POST /items/{itemId}/comments.
func (c *RestController) AddComment(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId,
	_ openapi.AddCommentParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.AddCommentJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{"item_id": itemID.String(), "body": body.Body}
	if body.ParentCommentId != nil {
		in["parent_comment_id"] = body.ParentCommentId.String()
	}

	out, err := c.UseCases.Invoke(r.Context(), addCommentUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusCreated, commentResponse(out))
}

// EditComment answers PATCH /items/{itemId}/comments/{commentId}.
func (c *RestController) EditComment(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, commentID openapi.CommentId,
	params openapi.EditCommentParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.EditCommentJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	in := usecase.Input{
		"item_id": itemID.String(), "comment_id": commentID.String(), "body": body.Body,
	}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	out, err := c.UseCases.Invoke(r.Context(), editCommentUseCase, actorOf(r), in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, commentResponse(out))
}

// DeleteComment answers DELETE /items/{itemId}/comments/{commentId}. 204: the caller asked for
// the comment to be gone, and what remains - the tombstone - is read through the thread.
func (c *RestController) DeleteComment(
	w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, commentID openapi.CommentId,
	params openapi.DeleteCommentParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	in := usecase.Input{"item_id": itemID.String(), "comment_id": commentID.String()}
	if version, ok := versionFromIfMatch(params.IfMatch); ok {
		in["expected_version"] = version
	}

	if _, err := c.UseCases.Invoke(r.Context(), deleteCommentUseCase, actorOf(r), in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// commentResponse maps the catalogue's output onto the generated schema. The mapping lives here
// because the generated types are the contract's shape rather than the domain's
// (project-structure.md §3).
func commentResponse(out usecase.Output) openapi.Comment {
	comment := openapi.Comment{
		Id:        uuidValue(out.String("id")),
		ItemId:    uuidValue(out.String("item_id")),
		AuthorId:  uuidValue(out.String("author_id")),
		CreatedAt: timeValue(out["created_at"]),
		Version:   out.Int("version"),
	}
	// The body is null exactly when the comment is deleted - the tombstone's shape, straight from
	// the contract's Comment schema.
	if body, alive := out["body"].(string); alive {
		comment.Body = &body
	}
	if parent := out.String("parent_comment_id"); parent != "" {
		parentID := uuidValue(parent)
		comment.ParentCommentId = &parentID
	}
	if at, ok := out["edited_at"].(time.Time); ok {
		comment.EditedAt = &at
	}
	if at, ok := out["deleted_at"].(time.Time); ok {
		comment.DeletedAt = &at
	}
	return comment
}
