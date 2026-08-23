// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// CommentRepository stores the discussion beside the entries (C-03).
type CommentRepository struct {
	cursors security.CursorCodec
}

func NewCommentRepository(cursors security.CursorCodec) CommentRepository {
	return CommentRepository{cursors: cursors}
}

var _ repository.Comments = CommentRepository{}

// Insert writes a new comment.
func (r CommentRepository) Insert(ctx context.Context, comment work.Comment) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(comment.ID)
	if err != nil {
		return err
	}
	itemID, err := uuidOf(comment.ItemID)
	if err != nil {
		return err
	}
	authorID, err := uuidOf(comment.AuthorID)
	if err != nil {
		return err
	}
	parentID, err := optionalUUID(comment.ParentCommentID)
	if err != nil {
		return err
	}

	err = queries.InsertComment(ctx, sqlc.InsertCommentParams{
		ID:              id,
		ItemID:          itemID,
		AuthorID:        authorID,
		ParentCommentID: parentID,
		Body:            comment.Body,
		CreatedAt:       timestampOf(comment.CreatedAt),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the comment %s: %w", comment.ID, err))
	}
	return nil
}

// Find returns the comment, tombstone or not.
func (r CommentRepository) Find(ctx context.Context, id shared.ID) (work.Comment, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.Comment{}, err
	}
	commentID, err := uuidOf(id)
	if err != nil {
		return work.Comment{}, err
	}

	row, err := queries.FindComment(ctx, commentID)
	if err != nil {
		if IsNoRows(err) {
			return work.Comment{}, shared.ErrNotFound
		}
		return work.Comment{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the comment %s: %w", id, err))
	}
	return commentFrom(
		row.ID, row.TenantID, row.ItemID, row.AuthorID, row.ParentCommentID,
		row.Body, row.CreatedAt, row.EditedAt, row.DeletedAt, row.Version,
	)
}

// List returns one page of one entry's comments, oldest first.
func (r CommentRepository) List(
	ctx context.Context, itemID shared.ID, page repository.Page,
) (repository.CommentPage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.CommentPage{}, err
	}
	id, err := uuidOf(itemID)
	if err != nil {
		return repository.CommentPage{}, err
	}
	boundary, err := commentCursor(r.cursors, page.Cursor)
	if err != nil {
		return repository.CommentPage{}, err
	}

	rows, err := queries.ListComments(ctx, sqlc.ListCommentsParams{
		ItemID:          id,
		CursorCreatedAt: boundary.createdAt,
		CursorID:        boundary.id,
		PageSize:        pageProbe(page.Size),
	})
	if err != nil {
		return repository.CommentPage{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the comments of %s: %w", itemID, err))
	}

	comments := make([]work.Comment, 0, len(rows))
	for _, row := range rows {
		comment, err := commentFrom(
			row.ID, row.TenantID, row.ItemID, row.AuthorID, row.ParentCommentID,
			row.Body, row.CreatedAt, row.EditedAt, row.DeletedAt, row.Version,
		)
		if err != nil {
			return repository.CommentPage{}, err
		}
		comments = append(comments, comment)
	}

	kept, info := pageOf(comments, page.Size, r.cursors, func(comment work.Comment) security.Position {
		return security.At(comment.CreatedAt.UTC().Format(time.RFC3339Nano), comment.ID)
	})
	return repository.CommentPage{Comments: kept, Info: repository.PageInfo(info)}, nil
}

// SetBody writes the rewritten text under the optimistic lock. A tombstone is never matched (see
// the statement), so an edit racing a deletion comes back as a version conflict - the same answer
// as any other row that moved on, which is all the caller can act on either way.
func (r CommentRepository) SetBody(
	ctx context.Context, comment work.Comment, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(comment.ID)
	if err != nil {
		return err
	}
	if comment.EditedAt == nil {
		// The domain stamps every edit; a write without the stamp is this code disagreeing with
		// itself rather than a request that can be fixed.
		return shared.ErrInternal.
			WithDetail("postgres.row_incoherent").
			WithCause(fmt.Errorf("the edit of comment %s carries no stamp", comment.ID))
	}

	affected, err := queries.SetCommentBody(ctx, sqlc.SetCommentBodyParams{
		Body:     comment.Body,
		EditedAt: timestampOf(*comment.EditedAt),
		ID:       id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the body of comment %s: %w", comment.ID, err))
	}
	return commentConflictIfUntouched(affected, comment.ID, expectedVersion)
}

// SetDeleted writes the tombstone under the optimistic lock.
func (r CommentRepository) SetDeleted(
	ctx context.Context, comment work.Comment, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(comment.ID)
	if err != nil {
		return err
	}
	if comment.DeletedAt == nil {
		return shared.ErrInternal.
			WithDetail("postgres.row_incoherent").
			WithCause(fmt.Errorf("the tombstone of comment %s carries no stamp", comment.ID))
	}

	affected, err := queries.SetCommentDeleted(ctx, sqlc.SetCommentDeletedParams{
		DeletedAt: timestampOf(*comment.DeletedAt),
		ID:        id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("deleting the comment %s: %w", comment.ID, err))
	}
	return commentConflictIfUntouched(affected, comment.ID, expectedVersion)
}

// commentConflictIfUntouched is the shared answer for a write that matched nothing: the row moved
// on, or - through row level security - was never this tenant's to move. One answer for both,
// deliberately (multi-tenancy.md §2).
func commentConflictIfUntouched(affected int64, id shared.ID, expectedVersion int) error {
	if affected != 0 {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("comments.version_conflict").
		WithParams(map[string]string{
			"comment_id": id.String(), "expected_version": fmt.Sprint(expectedVersion),
		})
}

// commentBoundary is a decoded comment cursor, both fields absent for the first page.
type commentBoundary struct {
	createdAt pgtype.Timestamptz
	id        pgtype.UUID
}

func commentCursor(cursors security.CursorCodec, cursor string) (commentBoundary, error) {
	if cursor == "" {
		return commentBoundary{}, nil
	}

	position, err := cursors.Decode(cursor)
	if err != nil {
		return commentBoundary{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, position.SortKey())
	if err != nil {
		return commentBoundary{}, shared.ErrValidation.
			WithDetail("shared.cursor_invalid").WithCause(err)
	}
	id, err := uuidOf(position.ID)
	if err != nil {
		return commentBoundary{}, err
	}
	return commentBoundary{createdAt: timestampOf(createdAt), id: id}, nil
}

// commentFrom maps a stored row onto the domain's comment. One mapper for both selects, so the
// two cannot disagree about a field.
func commentFrom(
	id, tenantID, itemID, authorID, parentID pgtype.UUID, body string,
	createdAt, editedAt, deletedAt pgtype.Timestamptz, version int32,
) (work.Comment, error) {
	commentID, err := idFrom(id)
	if err != nil {
		return work.Comment{}, err
	}
	tenant, err := idFrom(tenantID)
	if err != nil {
		return work.Comment{}, err
	}
	item, err := idFrom(itemID)
	if err != nil {
		return work.Comment{}, err
	}
	author, err := idFrom(authorID)
	if err != nil {
		return work.Comment{}, err
	}
	parent, err := optionalID(parentID)
	if err != nil {
		return work.Comment{}, err
	}
	if !createdAt.Valid {
		return work.Comment{}, shared.ErrInternal.WithDetail("postgres.row_incoherent")
	}

	return work.Comment{
		ID:              commentID,
		TenantID:        tenant,
		ItemID:          item,
		AuthorID:        author,
		ParentCommentID: parent,
		Body:            body,
		CreatedAt:       timeFrom(createdAt),
		EditedAt:        optionalTime(editedAt),
		DeletedAt:       optionalTime(deletedAt),
		Version:         int(version),
	}, nil
}
