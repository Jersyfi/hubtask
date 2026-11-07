// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// SatisfactionLinkRepository stores the satisfaction links of resolution mails (migration 0311)
// and finds the one a token names. The token is hashed here and nowhere else.
type SatisfactionLinkRepository struct {
	hasher security.InboundTokenHasher
}

// NewSatisfactionLinkRepository takes the hasher under the satisfaction link's own purpose label.
func NewSatisfactionLinkRepository(hasher security.InboundTokenHasher) SatisfactionLinkRepository {
	return SatisfactionLinkRepository{hasher: hasher}
}

var _ repository.SatisfactionLinks = SatisfactionLinkRepository{}

func satisfactionLinkFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// Put writes the requester's link on the entry under the token's hash, replacing the earlier one.
func (r SatisfactionLinkRepository) Put(ctx context.Context, link servicedesk.SatisfactionLink, token integration.SatisfactionLinkToken) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(link.RequesterID, link.ItemID)
	if err != nil {
		return err
	}
	if err := queries.PutSatisfactionLink(ctx, sqlc.PutSatisfactionLinkParams{
		RequesterID: ids[0], ItemID: ids[1], TokenHash: r.hasher.Hash(token.Secret()),
		CreatedAt: timestampOf(link.CreatedAt), ExpiresAt: timestampOf(link.ExpiresAt),
	}); err != nil {
		// No token in the message: it is the credential (rule 10, T-21).
		return satisfactionLinkFailed("writing a satisfaction link", err)
	}
	return nil
}

// FindByToken is the unauthenticated route's lookup, bounded to the token's tenant by the
// caller's transaction and row level security (ADR-0010).
func (r SatisfactionLinkRepository) FindByToken(ctx context.Context, token integration.SatisfactionLinkToken) (servicedesk.SatisfactionLink, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.SatisfactionLink{}, err
	}
	row, err := queries.FindSatisfactionLinkByHash(ctx, r.hasher.Hash(token.Secret()))
	if IsNoRows(err) {
		return servicedesk.SatisfactionLink{}, shared.ErrNotFound.WithDetail("satisfaction.link_not_found")
	}
	if err != nil {
		return servicedesk.SatisfactionLink{}, satisfactionLinkFailed("reading a satisfaction link by its token", err)
	}
	ids, err := pageIDs(row.RequesterID, row.ItemID)
	if err != nil {
		return servicedesk.SatisfactionLink{}, err
	}
	return servicedesk.SatisfactionLink{
		TenantID: token.TenantID(), RequesterID: ids[0], ItemID: ids[1],
		CreatedAt: timeFrom(row.CreatedAt), ExpiresAt: timeFrom(row.ExpiresAt), UsedAt: optionalTime(row.UsedAt),
	}, nil
}

// Use marks the link answered, once and only while it lives.
func (r SatisfactionLinkRepository) Use(ctx context.Context, token integration.SatisfactionLinkToken, now time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	rows, err := queries.UseSatisfactionLink(ctx, sqlc.UseSatisfactionLinkParams{At: timestampOf(now), TokenHash: r.hasher.Hash(token.Secret())})
	if err != nil {
		return false, satisfactionLinkFailed("answering a satisfaction link", err)
	}
	return rows > 0, nil
}
