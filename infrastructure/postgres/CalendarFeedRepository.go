// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// CalendarFeedRepository stores the subscriptions and finds the one a token names (D-08).
//
// It is the only place that knows how a feed token becomes a hash, for the reason the access
// token repository is: the pepper is a secret of this layer (security.md §8), which is why the
// port takes the presented token whole rather than a value the application layer would have had
// to compute.
type CalendarFeedRepository struct {
	hasher security.FeedTokenHasher
}

func NewCalendarFeedRepository(hasher security.FeedTokenHasher) CalendarFeedRepository {
	return CalendarFeedRepository{hasher: hasher}
}

var _ repository.CalendarFeeds = CalendarFeedRepository{}

// Insert writes the feed and the hash of the token minted beside it. The token itself is not
// stored, is not logged, and does not leave the call it was answered in.
func (r CalendarFeedRepository) Insert(
	ctx context.Context, feed domain.CalendarFeed, token domain.FeedToken,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(feed.ID)
	if err != nil {
		return err
	}
	accountID, err := uuidOf(feed.AccountID)
	if err != nil {
		return err
	}
	viewID, err := uuidOf(feed.ViewID)
	if err != nil {
		return err
	}

	if err := queries.InsertCalendarFeed(ctx, sqlc.InsertCalendarFeedParams{
		ID: id, AccountID: accountID, ViewID: viewID,
		TokenHash: r.hasher.Hash(token.Secret()),
		CreatedAt: timestampOf(feed.CreatedAt),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the calendar feed %s: %w", feed.ID, err))
	}
	return nil
}

// FindByToken is the public route's whole lookup, and it runs inside the caller's transaction -
// which is what bounds it to a tenant, because the query carries no tenant condition and row
// level security applies one no query can forget (ADR-0010). A token quoting a tenant it does not
// belong to therefore finds nothing.
func (r CalendarFeedRepository) FindByToken(
	ctx context.Context, token domain.FeedToken,
) (domain.CalendarFeed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.CalendarFeed{}, err
	}

	row, err := queries.FindCalendarFeedByHash(ctx, r.hasher.Hash(token.Secret()))
	if err != nil {
		if IsNoRows(err) {
			// The one answer an unknown token gets, here and everywhere above it.
			return domain.CalendarFeed{}, shared.ErrNotFound.WithDetail("calendar.feed_unknown")
		}
		return domain.CalendarFeed{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			// No token in the message: this is the credential, and an error travels into logs
			// (rule 10, T-21).
			WithCause(fmt.Errorf("reading the calendar feed by token: %w", err))
	}
	return feedFrom(row.ID, row.TenantID, row.AccountID, row.ViewID, row.CreatedAt, row.RevokedAt)
}

// Find returns one feed, revoked or not.
func (r CalendarFeedRepository) Find(
	ctx context.Context, id shared.ID,
) (domain.CalendarFeed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.CalendarFeed{}, err
	}
	feedID, err := uuidOf(id)
	if err != nil {
		return domain.CalendarFeed{}, err
	}

	row, err := queries.FindCalendarFeed(ctx, feedID)
	if err != nil {
		if IsNoRows(err) {
			return domain.CalendarFeed{}, shared.ErrNotFound.WithDetail("calendar.feed_unknown")
		}
		return domain.CalendarFeed{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the calendar feed %s: %w", id, err))
	}
	return feedFrom(row.ID, row.TenantID, row.AccountID, row.ViewID, row.CreatedAt, row.RevokedAt)
}

// ListForAccount returns one person's own feeds, newest first.
func (r CalendarFeedRepository) ListForAccount(
	ctx context.Context, accountID shared.ID,
) ([]domain.CalendarFeed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	owner, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListCalendarFeedsForAccount(ctx, owner)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the calendar feeds: %w", err))
	}

	feeds := make([]domain.CalendarFeed, 0, len(rows))
	for _, row := range rows {
		feed, err := feedFrom(
			row.ID, row.TenantID, row.AccountID, row.ViewID, row.CreatedAt, row.RevokedAt)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, feed)
	}
	return feeds, nil
}

// Revoke stamps the feed and says whether it changed anything. The statement's own
// `revoked_at IS NULL` decides that, so a second revocation costs one round trip and no read.
func (r CalendarFeedRepository) Revoke(
	ctx context.Context, id shared.ID, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	feedID, err := uuidOf(id)
	if err != nil {
		return false, err
	}

	affected, err := queries.RevokeCalendarFeed(ctx, sqlc.RevokeCalendarFeedParams{
		ID: feedID, RevokedAt: timestampOf(at),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("revoking the calendar feed %s: %w", id, err))
	}
	return affected == 1, nil
}

// feedFrom rebuilds the aggregate from a row. Every one of the five queries answers the same
// columns, so they share one reader.
func feedFrom(
	id, tenantID, accountID, viewID pgtype.UUID, createdAt, revokedAt pgtype.Timestamptz,
) (domain.CalendarFeed, error) {
	feedID, err := idFrom(id)
	if err != nil {
		return domain.CalendarFeed{}, err
	}
	tenant, err := idFrom(tenantID)
	if err != nil {
		return domain.CalendarFeed{}, err
	}
	owner, err := idFrom(accountID)
	if err != nil {
		return domain.CalendarFeed{}, err
	}
	// The view reference nulls when the view is deleted, which is a state the aggregate has a
	// word for rather than an error.
	view, err := optionalID(viewID)
	if err != nil {
		return domain.CalendarFeed{}, err
	}

	return domain.CalendarFeed{
		ID: feedID, TenantID: tenant, AccountID: owner, ViewID: view,
		CreatedAt: createdAt.Time.UTC(), RevokedAt: timeFrom(revokedAt),
	}, nil
}
