// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// PagePublicLinkRepository stores a page's public link (migration 0262) and finds the one a token
// names. The token is hashed here and nowhere else: the pepper is this layer's secret.
type PagePublicLinkRepository struct {
	hasher security.InboundTokenHasher
}

// NewPagePublicLinkRepository takes the hasher under the public link's own purpose label.
func NewPagePublicLinkRepository(hasher security.InboundTokenHasher) PagePublicLinkRepository {
	return PagePublicLinkRepository{hasher: hasher}
}

var _ repository.PublicLinks = PagePublicLinkRepository{}

// Put writes the link under the token's hash, replacing the page's previous one.
func (r PagePublicLinkRepository) Put(ctx context.Context, link knowledge.PublicLink, token integration.PageLinkToken) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(link.PageID, link.CreatedBy)
	if err != nil {
		return err
	}
	if err := queries.PutPagePublicLink(ctx, sqlc.PutPagePublicLinkParams{
		PageID: ids[0], TokenHash: r.hasher.Hash(token.Secret()), CreatedBy: ids[1], At: timestampOf(link.CreatedAt),
	}); err != nil {
		// No token in the message: it is the credential (rule 10, T-21).
		return pageFailed("writing the public link", err)
	}
	return nil
}

// Find answers the page's link.
func (r PagePublicLinkRepository) Find(ctx context.Context, pageID shared.ID) (knowledge.PublicLink, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return knowledge.PublicLink{}, false, err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return knowledge.PublicLink{}, false, err
	}
	row, err := queries.FindPagePublicLink(ctx, key)
	if IsNoRows(err) {
		return knowledge.PublicLink{}, false, nil
	}
	if err != nil {
		return knowledge.PublicLink{}, false, pageFailed("reading the public link", err)
	}
	ids, err := pageIDs(row.PageID, row.CreatedBy)
	if err != nil {
		return knowledge.PublicLink{}, false, err
	}
	return knowledge.PublicLink{PageID: ids[0], CreatedBy: ids[1], CreatedAt: timeFrom(row.CreatedAt)}, true, nil
}

// FindByToken is the unauthenticated route's lookup, bounded to the token's tenant by the
// caller's transaction and row level security (ADR-0010).
func (r PagePublicLinkRepository) FindByToken(ctx context.Context, token integration.PageLinkToken) (knowledge.PublicLink, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return knowledge.PublicLink{}, err
	}
	row, err := queries.FindPagePublicLinkByHash(ctx, r.hasher.Hash(token.Secret()))
	if IsNoRows(err) {
		return knowledge.PublicLink{}, shared.ErrNotFound.WithDetail("pages.link_not_found")
	}
	if err != nil {
		return knowledge.PublicLink{}, pageFailed("reading the public link by its token", err)
	}
	ids, err := pageIDs(row.PageID, row.CreatedBy)
	if err != nil {
		return knowledge.PublicLink{}, err
	}
	return knowledge.PublicLink{
		TenantID: token.TenantID(), PageID: ids[0], CreatedBy: ids[1], CreatedAt: timeFrom(row.CreatedAt),
	}, nil
}

// Delete takes the page's link away.
func (r PagePublicLinkRepository) Delete(ctx context.Context, pageID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return false, err
	}
	rows, err := queries.DeletePagePublicLink(ctx, key)
	if err != nil {
		return false, pageFailed("removing the public link", err)
	}
	return rows > 0, nil
}
