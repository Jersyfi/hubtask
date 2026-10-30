// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

var _ repository.Ratings = ServiceDeskRatings{}

// ServiceDeskRatings stores requesters' satisfaction, one per request (migration 0243).
type ServiceDeskRatings struct{}

func (ServiceDeskRepository) Ratings() ServiceDeskRatings { return ServiceDeskRatings{} }

func ratingFrom(row sqlc.SatisfactionRating) (servicedesk.Rating, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return servicedesk.Rating{}, err
	}
	tenant, err := idFrom(row.TenantID)
	if err != nil {
		return servicedesk.Rating{}, err
	}
	item, err := idFrom(row.ItemID)
	if err != nil {
		return servicedesk.Rating{}, err
	}
	return servicedesk.Rating{
		ID: id, TenantID: tenant, ItemID: item, Score: int(row.Score), RatedAt: row.RatedAt.Time,
		Version: int(row.Version),
	}, nil
}

func (ServiceDeskRatings) Save(ctx context.Context, rating servicedesk.Rating) (servicedesk.Rating, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.Rating{}, err
	}
	id, err := uuidOf(rating.ID)
	if err != nil {
		return servicedesk.Rating{}, err
	}
	tenant, err := uuidOf(rating.TenantID)
	if err != nil {
		return servicedesk.Rating{}, err
	}
	item, err := uuidOf(rating.ItemID)
	if err != nil {
		return servicedesk.Rating{}, err
	}
	row, err := queries.SaveSatisfactionRating(ctx, sqlc.SaveSatisfactionRatingParams{
		ID: id, TenantID: tenant, ItemID: item,
		Score: int16(rating.Score), //nolint:gosec // the domain keeps a score between 1 and 5
		RatedAt: timestampOf(rating.RatedAt),
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return servicedesk.Rating{}, shared.ErrNotFound.WithDetail("items.not_found")
		}
		return servicedesk.Rating{}, queryFailed("writing a satisfaction rating", err)
	}
	return ratingFrom(row)
}

func (ServiceDeskRatings) OfItem(ctx context.Context, itemID shared.ID) (servicedesk.Rating, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.Rating{}, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return servicedesk.Rating{}, err
	}
	row, err := queries.FindSatisfactionRating(ctx, item)
	if err != nil {
		if IsNoRows(err) {
			return servicedesk.Rating{}, shared.ErrNotFound.WithDetail("satisfaction.not_rated")
		}
		return servicedesk.Rating{}, queryFailed("reading a satisfaction rating", err)
	}
	return ratingFrom(row)
}
