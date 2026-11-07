// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// PortfolioPublicLinkRepository stores a portfolio roadmap's public link (migration 0329) and finds the one a token
// names. The token is hashed here and nowhere else: the pepper is this layer's secret.
type PortfolioPublicLinkRepository struct {
	hasher security.InboundTokenHasher
}

// NewPortfolioPublicLinkRepository takes the hasher under the roadmap link's own purpose label.
func NewPortfolioPublicLinkRepository(hasher security.InboundTokenHasher) PortfolioPublicLinkRepository {
	return PortfolioPublicLinkRepository{hasher: hasher}
}

var _ repository.RoadmapLinks = PortfolioPublicLinkRepository{}

// Put writes the link under the token's hash, replacing the portfolio's previous one.
func (r PortfolioPublicLinkRepository) Put(ctx context.Context, link goal.RoadmapLink, token integration.RoadmapLinkToken) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(link.PortfolioID, link.CreatedBy)
	if err != nil {
		return err
	}
	if err := queries.PutPortfolioPublicLink(ctx, sqlc.PutPortfolioPublicLinkParams{
		PortfolioID: ids[0], TokenHash: r.hasher.Hash(token.Secret()), CreatedBy: ids[1], At: timestampOf(link.CreatedAt),
	}); err != nil {
		// No token in the message: it is the credential (rule 10, T-21).
		return pageFailed("writing the public link", err)
	}
	return nil
}

// Find answers the portfolio's link.
func (r PortfolioPublicLinkRepository) Find(ctx context.Context, portfolioID shared.ID) (goal.RoadmapLink, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return goal.RoadmapLink{}, false, err
	}
	key, err := uuidOf(portfolioID)
	if err != nil {
		return goal.RoadmapLink{}, false, err
	}
	row, err := queries.FindPortfolioPublicLink(ctx, key)
	if IsNoRows(err) {
		return goal.RoadmapLink{}, false, nil
	}
	if err != nil {
		return goal.RoadmapLink{}, false, pageFailed("reading the public link", err)
	}
	ids, err := pageIDs(row.PortfolioID, row.CreatedBy)
	if err != nil {
		return goal.RoadmapLink{}, false, err
	}
	return goal.RoadmapLink{PortfolioID: ids[0], CreatedBy: ids[1], CreatedAt: timeFrom(row.CreatedAt)}, true, nil
}

// FindByToken is the unauthenticated route's lookup, bounded to the token's tenant by the
// caller's transaction and row level security (ADR-0010).
func (r PortfolioPublicLinkRepository) FindByToken(ctx context.Context, token integration.RoadmapLinkToken) (goal.RoadmapLink, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return goal.RoadmapLink{}, err
	}
	row, err := queries.FindPortfolioPublicLinkByHash(ctx, r.hasher.Hash(token.Secret()))
	if IsNoRows(err) {
		return goal.RoadmapLink{}, shared.ErrNotFound.WithDetail("portfolios.link_not_found")
	}
	if err != nil {
		return goal.RoadmapLink{}, pageFailed("reading the public link by its token", err)
	}
	ids, err := pageIDs(row.PortfolioID, row.CreatedBy)
	if err != nil {
		return goal.RoadmapLink{}, err
	}
	return goal.RoadmapLink{
		TenantID: token.TenantID(), PortfolioID: ids[0], CreatedBy: ids[1], CreatedAt: timeFrom(row.CreatedAt),
	}, nil
}

// Delete takes the portfolio's link away.
func (r PortfolioPublicLinkRepository) Delete(ctx context.Context, portfolioID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(portfolioID)
	if err != nil {
		return false, err
	}
	rows, err := queries.DeletePortfolioPublicLink(ctx, key)
	if err != nil {
		return false, pageFailed("removing the public link", err)
	}
	return rows > 0, nil
}
