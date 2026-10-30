// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// InsertComment writes a board's comment (migration 0266).
func (WhiteboardRepository) InsertComment(ctx context.Context, comment whiteboard.Comment) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(comment.ID, comment.BoardID, comment.AuthorID)
	if err != nil {
		return err
	}
	parent, err := optionalUUID(comment.ParentID)
	if err != nil {
		return err
	}
	if err := queries.InsertWhiteboardComment(ctx, sqlc.InsertWhiteboardCommentParams{
		ID: ids[0], WhiteboardID: ids[1], ParentID: parent, AuthorID: ids[2], Body: comment.Body,
		ElementID: comment.Element, X: comment.X, Y: comment.Y, At: timestampOf(comment.CreatedAt),
	}); err != nil {
		return boardFailed("writing the comment", err)
	}
	return nil
}

// FindComment answers a board's comment; another workspace's is not found.
func (WhiteboardRepository) FindComment(ctx context.Context, id shared.ID) (whiteboard.Comment, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return whiteboard.Comment{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return whiteboard.Comment{}, shared.ErrNotFound.WithDetail("whiteboards.comment_not_found")
	}
	row, err := queries.FindWhiteboardComment(ctx, key)
	if IsNoRows(err) {
		return whiteboard.Comment{}, shared.ErrNotFound.WithDetail("whiteboards.comment_not_found")
	}
	if err != nil {
		return whiteboard.Comment{}, boardFailed("reading the comment", err)
	}
	return boardCommentFrom(row)
}

// Comments answers a board's comments, oldest first.
func (WhiteboardRepository) Comments(ctx context.Context, boardID shared.ID, limit int) ([]whiteboard.Comment, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	board, err := uuidOf(boardID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListWhiteboardComments(ctx, sqlc.ListWhiteboardCommentsParams{
		WhiteboardID: board, Batch: int32(limit), //nolint:gosec // bounded by the use case
	})
	if err != nil {
		return nil, boardFailed("listing the comments", err)
	}
	return mapRows(rows, func(row sqlc.ListWhiteboardCommentsRow) (whiteboard.Comment, error) {
		return boardCommentFrom(sqlc.FindWhiteboardCommentRow(row))
	})
}

// UpdateComment writes a comment at the version expected.
func (WhiteboardRepository) UpdateComment(ctx context.Context, comment whiteboard.Comment, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(comment.ID)
	if err != nil {
		return err
	}
	resolvedBy, err := optionalUUID(comment.ResolvedBy)
	if err != nil {
		return err
	}
	rows, err := queries.UpdateWhiteboardComment(ctx, sqlc.UpdateWhiteboardCommentParams{
		ID: id, Body: comment.Body, ResolvedAt: optionalTimestamp(comment.ResolvedAt), ResolvedBy: resolvedBy,
		EditedAt: optionalTimestamp(comment.EditedAt), DeletedAt: optionalTimestamp(comment.DeletedAt),
		ExpectedVersion: int32(expected), //nolint:gosec // a row's version
	})
	if err != nil {
		return boardFailed("writing the comment", err)
	}
	if rows == 0 {
		return shared.ErrVersionConflict.WithDetail("whiteboards.comment_version_conflict")
	}
	return nil
}

func boardCommentFrom(row sqlc.FindWhiteboardCommentRow) (whiteboard.Comment, error) {
	ids, err := pageIDs(row.ID, row.WhiteboardID)
	if err != nil {
		return whiteboard.Comment{}, err
	}
	nullable := make([]shared.ID, 3)
	for i, value := range []pgtype.UUID{row.ParentID, row.AuthorID, row.ResolvedBy} {
		if nullable[i], err = optionalID(value); err != nil {
			return whiteboard.Comment{}, err
		}
	}
	return whiteboard.Comment{
		ID: ids[0], BoardID: ids[1], ParentID: nullable[0], AuthorID: nullable[1], Body: row.Body,
		Element: row.ElementID, X: row.X, Y: row.Y, ResolvedAt: optionalTime(row.ResolvedAt), ResolvedBy: nullable[2],
		CreatedAt: timeFrom(row.CreatedAt), EditedAt: optionalTime(row.EditedAt),
		DeletedAt: optionalTime(row.DeletedAt), Version: int(row.Version),
	}, nil
}
