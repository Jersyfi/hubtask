// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The discussion beside the entries: a sub-resource of the item, because a comment is not
// a field of its row - it appends, it never merges, and it pages on its own.

// commentResponse maps the catalogue's output onto the generated schema. The mapping lives here
// because the generated types are the contract's shape rather than the domain's
// (project-structure.md §3).
func commentResponse(out usecase.Output) openapi.Comment {
	comment := openapi.Comment{
		Id:         uuidValue(out.String("id")),
		ItemId:     uuidValue(out.String("item_id")),
		AuthorId:   uuidValue(out.String("author_id")),
		CreatedAt:  timeValue(out["created_at"]),
		Version:    out.Int("version"),
		SystemCode: optionalTextField(out["system_code"]),
	}
	if kind := out.String("kind"); kind != "" {
		kind := openapi.CommentKind(kind)
		comment.Kind = &kind
	}
	if params, ok := out["system_params"].(map[string]any); ok {
		strings := make(map[string]string, len(params))
		for key, value := range params {
			text, _ := value.(string)
			strings[key] = text
		}
		comment.SystemParams = &strings
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
