// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PageDraftRepository stores what waits on a page to be published (migration 0313).
type PageDraftRepository struct{}

// NewPageDraftRepository is the adapter.
func NewPageDraftRepository() PageDraftRepository { return PageDraftRepository{} }

var _ repository.Drafts = PageDraftRepository{}

// Find answers the page's draft.
func (PageDraftRepository) Find(ctx context.Context, pageID shared.ID) (knowledge.Draft, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return knowledge.Draft{}, false, err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return knowledge.Draft{}, false, err
	}
	row, err := queries.FindPageDraft(ctx, key)
	if IsNoRows(err) {
		return knowledge.Draft{}, false, nil
	}
	if err != nil {
		return knowledge.Draft{}, false, pageFailed("reading the draft", err)
	}
	by, err := idFrom(row.UpdatedBy)
	if err != nil {
		return knowledge.Draft{}, false, err
	}
	draft := knowledge.Draft{PageID: pageID, Unpublished: row.Unpublished, UpdatedBy: by, UpdatedAt: timeFrom(row.UpdatedAt)}
	if row.Title != nil {
		draft.Title = *row.Title
	}
	if row.Body != nil {
		draft.Body = *row.Body
	}
	return draft, true, nil
}

// Put writes the page's draft; the mark of a page never published carries no words.
func (PageDraftRepository) Put(ctx context.Context, draft knowledge.Draft) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(draft.PageID, draft.UpdatedBy)
	if err != nil {
		return err
	}
	params := sqlc.PutPageDraftParams{
		PageID: ids[0], Unpublished: draft.Unpublished, UpdatedBy: ids[1], UpdatedAt: timestampOf(draft.UpdatedAt),
	}
	if !draft.Unpublished {
		params.Title, params.Body = &draft.Title, &draft.Body
	}
	if err := queries.PutPageDraft(ctx, params); err != nil {
		return pageFailed("writing the draft", err)
	}
	return nil
}

// Delete removes the page's draft.
func (PageDraftRepository) Delete(ctx context.Context, pageID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return false, err
	}
	rows, err := queries.DeletePageDraft(ctx, key)
	if err != nil {
		return false, pageFailed("removing the draft", err)
	}
	return rows > 0, nil
}

// Pending counts the drafts of published pages in a hub's space.
func (PageDraftRepository) Pending(ctx context.Context, hubID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	key, err := uuidOf(hubID)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountPendingPageDrafts(ctx, key)
	if err != nil {
		return 0, pageFailed("counting the drafts", err)
	}
	return int(count), nil
}
