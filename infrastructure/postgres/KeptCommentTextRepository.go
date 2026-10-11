// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// KeptCommentTextRepository reads and clears the text a legal hold kept on deleted comments
// (data-retention.md §4). Its own type rather than methods on CommentRepository: the retention pass
// takes this half, and a port that carried both would let it write a comment's body.
type KeptCommentTextRepository struct{}

var _ repository.KeptCommentTexts = KeptCommentTextRepository{}

// Kept returns one page of the deleted comments whose text is kept, after the identifier given.
func (KeptCommentTextRepository) Kept(
	ctx context.Context, after shared.ID, batch int,
) ([]repository.KeptCommentText, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := pageKey(after)
	if err != nil {
		return nil, err
	}

	rows, err := queries.KeptCommentTexts(ctx, sqlc.KeptCommentTextsParams{
		After: key,
		//nolint:gosec // G115: a batch is the pass's page size, a few thousand at most
		Batch: int32(batch),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the kept comment texts: %w", err))
	}

	kept := make([]repository.KeptCommentText, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		item, err := idFrom(row.ItemID)
		if err != nil {
			return nil, err
		}
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		hub, err := optionalID(row.HubID)
		if err != nil {
			return nil, err
		}
		kept = append(kept, repository.KeptCommentText{
			ID: id, ItemID: item, Path: row.Path, CollectionID: collection, HubID: hub,
		})
	}
	return kept, nil
}

// Clear removes the kept text of the comments named.
func (KeptCommentTextRepository) Clear(ctx context.Context, ids []shared.ID) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	keys, err := uuidsOf(ids)
	if err != nil {
		return 0, err
	}

	cleared, err := queries.ClearCommentTexts(ctx, keys)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing kept comment texts: %w", err))
	}
	return int(cleared), nil
}
