// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PlatformDeliveryRepository keeps the deliveries of the installation's subscriptions
// (repository.PlatformDeliveries).
type PlatformDeliveryRepository struct{}

// NewPlatformDeliveryRepository builds the repository.
func NewPlatformDeliveryRepository() PlatformDeliveryRepository { return PlatformDeliveryRepository{} }

var _ repository.PlatformDeliveries = PlatformDeliveryRepository{}

func deliveryFailed(what string, err error) error {
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("%s a platform delivery: %w", what, err))
}

// Insert queues a delivery once.
func (PlatformDeliveryRepository) Insert(ctx context.Context, d integration.PlatformDelivery) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(d.ID)
	if err != nil {
		return false, err
	}
	sub, err := uuidOf(d.SubscriptionID)
	if err != nil {
		return false, err
	}
	eventID, err := uuidOf(d.EventID)
	if err != nil {
		return false, err
	}
	written, err := queries.InsertPlatformDelivery(ctx, sqlc.InsertPlatformDeliveryParams{
		ID: id, SubscriptionID: sub, EventID: eventID, EventType: d.EventType, Now: momentOf(d.CreatedAt),
	})
	if err != nil {
		return false, deliveryFailed("queuing", err)
	}
	return written == 1, nil
}

// Find answers one delivery.
func (PlatformDeliveryRepository) Find(ctx context.Context, id shared.ID) (integration.PlatformDelivery, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return integration.PlatformDelivery{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return integration.PlatformDelivery{}, shared.ErrNotFound.WithDetail("admin.delivery_not_found")
	}
	row, err := queries.FindPlatformDelivery(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return integration.PlatformDelivery{}, shared.ErrNotFound.WithDetail("admin.delivery_not_found")
		}
		return integration.PlatformDelivery{}, deliveryFailed("reading", err)
	}
	return platformDeliveryOf(sqlc.PlatformDelivery(row))
}

// Page answers a subscription's deliveries, newest first.
func (PlatformDeliveryRepository) Page(
	ctx context.Context, subscriptionID shared.ID, status string, before shared.ID, size int,
) ([]integration.PlatformDelivery, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	sub, err := uuidOf(subscriptionID)
	if err != nil {
		return nil, err
	}
	params := sqlc.PlatformDeliveriesParams{SubscriptionID: sub, PageSize: int32(size)} //nolint:gosec // bounded by the use case
	if status != "" {
		params.Status = &status
	}
	if !before.IsZero() {
		if params.Before, err = uuidOf(before); err != nil {
			return nil, err
		}
	}
	rows, err := queries.PlatformDeliveries(ctx, params)
	if err != nil {
		return nil, deliveryFailed("listing", err)
	}
	deliveries := make([]integration.PlatformDelivery, 0, len(rows))
	for _, row := range rows {
		d, err := platformDeliveryOf(row)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, nil
}

// Record writes one attempt's outcome.
func (PlatformDeliveryRepository) Record(ctx context.Context, d integration.PlatformDelivery) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(d.ID)
	if err != nil {
		return false, err
	}
	params := sqlc.RecordPlatformDeliveryAttemptParams{
		Attempt: int32(d.Attempt), Status: string(d.Status), //nolint:gosec // a count of attempts
		NextAttemptAt: momentOf(d.NextAttemptAt), Now: momentOf(d.UpdatedAt), ID: id,
	}
	if d.ResponseStatus != 0 {
		status := int32(d.ResponseStatus) //nolint:gosec // an HTTP status
		params.ResponseStatus = &status
	}
	if d.ErrorCode != "" {
		params.ErrorCode = &d.ErrorCode
	}
	written, err := queries.RecordPlatformDeliveryAttempt(ctx, params)
	if err != nil {
		return false, deliveryFailed("recording", err)
	}
	return written == 1, nil
}

// Requeue returns a batch of dead letters to pending.
func (PlatformDeliveryRepository) Requeue(ctx context.Context, subscriptionID shared.ID, now time.Time, batch int) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	sub, err := uuidOf(subscriptionID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.RequeueDeadPlatformDeliveries(ctx, sqlc.RequeueDeadPlatformDeliveriesParams{
		Now: momentOf(now), SubscriptionID: sub, Batch: int32(batch), //nolint:gosec // bounded by the use case
	})
	if err != nil {
		return nil, deliveryFailed("requeuing", err)
	}
	ids := make([]shared.ID, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// DeadLetters counts the installation's dead letters.
func (PlatformDeliveryRepository) DeadLetters(ctx context.Context) (int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountPlatformDeadLetters(ctx)
	if err != nil {
		return 0, deliveryFailed("counting", err)
	}
	return count, nil
}

// Sweep removes a batch of settled deliveries.
func (PlatformDeliveryRepository) Sweep(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	removed, err := queries.DeleteExpiredPlatformDeliveries(ctx, sqlc.DeleteExpiredPlatformDeliveriesParams{
		Cutoff: pgtype.Timestamptz{Time: cutoff.UTC(), Valid: true}, Batch: int32(batch), //nolint:gosec // bounded by the caller
	})
	if err != nil {
		return 0, deliveryFailed("sweeping", err)
	}
	return int(removed), nil
}

func platformDeliveryOf(row sqlc.PlatformDelivery) (integration.PlatformDelivery, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return integration.PlatformDelivery{}, err
	}
	sub, err := idFrom(row.SubscriptionID)
	if err != nil {
		return integration.PlatformDelivery{}, err
	}
	eventID, err := idFrom(row.EventID)
	if err != nil {
		return integration.PlatformDelivery{}, err
	}
	d := integration.PlatformDelivery{
		ID: id, SubscriptionID: sub, EventID: eventID, EventType: row.EventType, Attempt: int(row.Attempt),
		Status: integration.DeliveryStatus(row.Status), ErrorCode: stringFrom(row.ErrorCode),
		NextAttemptAt: timeFrom(row.NextAttemptAt), CreatedAt: timeFrom(row.CreatedAt), UpdatedAt: timeFrom(row.UpdatedAt),
	}
	if row.ResponseStatus != nil {
		d.ResponseStatus = int(*row.ResponseStatus)
	}
	return d, nil
}
