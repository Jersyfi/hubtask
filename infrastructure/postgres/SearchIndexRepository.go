// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// SearchIndexRepository is the adapter for the search's bookkeeping (M-09).
type SearchIndexRepository struct{}

func NewSearchIndexRepository() SearchIndexRepository { return SearchIndexRepository{} }

var _ repository.SearchIndex = SearchIndexRepository{}

// Stale counts the workspace's rows whose document would be built differently today.
func (r SearchIndexRepository) Stale(ctx context.Context) (int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	stale, err := queries.CountStaleSearchDocuments(ctx)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the stale search documents: %w", err))
	}
	return stale, nil
}

// Rebuild rewrites one batch of stale rows.
func (r SearchIndexRepository) Rebuild(ctx context.Context, batch int) (int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	if batch <= 0 {
		batch = 1
	}
	rewritten, err := queries.RebuildStaleSearchDocuments(ctx, int32(min(batch, 10000)))
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("rebuilding the search documents: %w", err))
	}
	return rewritten, nil
}
