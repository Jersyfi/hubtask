// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	lifecycle "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
)

// DocumentTrash is the pages' and boards' trash the retention pass removes (TRASH); the statements
// are db/queries/DocumentRetention.sql, bounded to the transaction's tenant.
type DocumentTrash struct{}

// PageVersionThinning removes the page versions PAGE_VERSION lets go.
type PageVersionThinning struct{}

func documentRetentionFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// Expired answers the pages first, then the boards, up to batch together.
func (DocumentTrash) Expired(
	ctx context.Context, cutoff time.Time, passed []shared.ID, batch int,
) ([]lifecycle.TrashedDocument, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	skip, err := uuidsOf(passed)
	if err != nil {
		return nil, err
	}
	pages, err := queries.ExpiredTrashedPages(ctx, sqlc.ExpiredTrashedPagesParams{
		Cutoff: timestampOf(cutoff), Passed: skip, Batch: boundedInt32(batch),
	})
	if err != nil {
		return nil, documentRetentionFailed("reading expired trashed pages", err)
	}
	documents := make([]lifecycle.TrashedDocument, 0, len(pages))
	for _, row := range pages {
		document, err := trashedDocument(knowledge.DocumentKind, row.ID, row.HubID, row.Contributors)
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	if batch > 0 && len(documents) >= batch {
		return documents, nil
	}
	boards, err := queries.ExpiredTrashedWhiteboards(ctx, sqlc.ExpiredTrashedWhiteboardsParams{
		Cutoff: timestampOf(cutoff), Passed: skip, Batch: boundedInt32(batch - len(documents)),
	})
	if err != nil {
		return nil, documentRetentionFailed("reading expired trashed boards", err)
	}
	for _, row := range boards {
		document, err := trashedDocument(whiteboard.DocumentKind, row.ID, row.HubID, row.Contributors)
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	return documents, nil
}

func trashedDocument(entity string, id, hub pgtype.UUID, contributors []pgtype.UUID) (lifecycle.TrashedDocument, error) {
	documentID, err := idFrom(id)
	if err != nil {
		return lifecycle.TrashedDocument{}, err
	}
	hubID, err := idFrom(hub)
	if err != nil {
		return lifecycle.TrashedDocument{}, err
	}
	who, err := idsFrom(contributors)
	if err != nil {
		return lifecycle.TrashedDocument{}, err
	}
	return lifecycle.TrashedDocument{Entity: entity, ID: documentID, HubID: hubID, Contributors: who}, nil
}

// Remove deletes what the pass judged, pages and boards apart.
func (DocumentTrash) Remove(
	ctx context.Context, documents []lifecycle.TrashedDocument, cutoff time.Time,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	var pages, boards []shared.ID
	for _, document := range documents {
		if document.Entity == whiteboard.DocumentKind {
			boards = append(boards, document.ID)
		} else {
			pages = append(pages, document.ID)
		}
	}
	removed := 0
	if len(pages) > 0 {
		ids, err := uuidsOf(pages)
		if err != nil {
			return 0, err
		}
		n, err := queries.DeleteTrashedPages(ctx, sqlc.DeleteTrashedPagesParams{Ids: ids, Cutoff: timestampOf(cutoff)})
		if err != nil {
			return 0, documentRetentionFailed("removing trashed pages", err)
		}
		removed += int(n)
	}
	if len(boards) > 0 {
		ids, err := uuidsOf(boards)
		if err != nil {
			return 0, err
		}
		n, err := queries.DeleteTrashedWhiteboards(ctx, sqlc.DeleteTrashedWhiteboardsParams{Ids: ids, Cutoff: timestampOf(cutoff)})
		if err != nil {
			return 0, documentRetentionFailed("removing trashed boards", err)
		}
		removed += int(n)
	}
	return removed, nil
}

// DeleteThinned removes a batch of thinned versions.
func (PageVersionThinning) DeleteThinned(
	ctx context.Context, cutoff time.Time, heldHubs, heldAccounts []shared.ID, batch int,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	hubs, accounts, err := heldArrays(heldHubs, heldAccounts)
	if err != nil {
		return 0, err
	}
	n, err := queries.DeleteThinnedPageVersions(ctx, sqlc.DeleteThinnedPageVersionsParams{
		Cutoff: timestampOf(cutoff), HeldHubs: hubs, HeldAccounts: accounts, Batch: boundedInt32(batch),
	})
	if err != nil {
		return 0, documentRetentionFailed("thinning page versions", err)
	}
	return int(n), nil
}

// CountThinned counts the versions due, up to ceiling.
func (PageVersionThinning) CountThinned(
	ctx context.Context, cutoff time.Time, heldHubs, heldAccounts []shared.ID, ceiling int,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	hubs, accounts, err := heldArrays(heldHubs, heldAccounts)
	if err != nil {
		return 0, err
	}
	n, err := queries.CountThinnedPageVersions(ctx, sqlc.CountThinnedPageVersionsParams{
		Cutoff: timestampOf(cutoff), HeldHubs: hubs, HeldAccounts: accounts, Ceiling: boundedInt32(ceiling),
	})
	if err != nil {
		return 0, documentRetentionFailed("counting thinned page versions", err)
	}
	return int(n), nil
}

func heldArrays(hubs, accounts []shared.ID) (heldHubs, heldAccounts []pgtype.UUID, err error) {
	if heldHubs, err = uuidsOf(hubs); err != nil {
		return nil, nil, err
	}
	heldAccounts, err = uuidsOf(accounts)
	return heldHubs, heldAccounts, err
}
