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

// PlatformSubscriptionRepository keeps the installation's subscriptions (repository.PlatformSubscriptions).
type PlatformSubscriptionRepository struct{}

// NewPlatformSubscriptionRepository builds the repository.
func NewPlatformSubscriptionRepository() PlatformSubscriptionRepository {
	return PlatformSubscriptionRepository{}
}

var _ repository.PlatformSubscriptions = PlatformSubscriptionRepository{}

func subscriptionFailed(what string, err error) error {
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("%s an installation subscription: %w", what, err))
}

func momentOf(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: !t.IsZero()}
}

// Insert writes a new subscription.
func (PlatformSubscriptionRepository) Insert(
	ctx context.Context, subscription integration.PlatformSubscription, secret repository.PlatformSecret,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(subscription.ID)
	if err != nil {
		return err
	}
	createdBy, err := optionalUUID(subscription.CreatedBy)
	if err != nil {
		return err
	}
	if err := queries.InsertPlatformSubscription(ctx, sqlc.InsertPlatformSubscriptionParams{
		ID: id, TargetUrl: subscription.TargetURL, SecretEnc: secret.Ciphertext, SecretKeyID: secret.KeyID,
		FromFile: subscription.FromFile, Now: momentOf(subscription.CreatedAt), CreatedBy: createdBy,
	}); err != nil {
		return subscriptionFailed("writing", err)
	}
	return nil
}

// List answers every subscription.
func (PlatformSubscriptionRepository) List(ctx context.Context) ([]integration.PlatformSubscription, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListPlatformSubscriptions(ctx)
	if err != nil {
		return nil, subscriptionFailed("listing", err)
	}
	subscriptions := make([]integration.PlatformSubscription, 0, len(rows))
	for _, row := range rows {
		subscription, err := platformSubscriptionOf(sqlc.FindPlatformSubscriptionRow(row))
		if err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, subscription)
	}
	return subscriptions, nil
}

// Find answers one subscription with its sealed secret.
func (PlatformSubscriptionRepository) Find(ctx context.Context, id shared.ID) (repository.StoredPlatformSubscription, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.StoredPlatformSubscription{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return repository.StoredPlatformSubscription{}, shared.ErrNotFound.WithDetail("admin.subscription_not_found")
	}
	row, err := queries.FindPlatformSubscription(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return repository.StoredPlatformSubscription{}, shared.ErrNotFound.WithDetail("admin.subscription_not_found")
		}
		return repository.StoredPlatformSubscription{}, subscriptionFailed("reading", err)
	}
	subscription, err := platformSubscriptionOf(row)
	if err != nil {
		return repository.StoredPlatformSubscription{}, err
	}
	return repository.StoredPlatformSubscription{
		Subscription: subscription,
		Secret:       repository.PlatformSecret{Ciphertext: row.SecretEnc, KeyID: row.SecretKeyID},
	}, nil
}

// Update writes the target and the state, guarded on the version.
func (PlatformSubscriptionRepository) Update(
	ctx context.Context, subscription integration.PlatformSubscription, expectedVersion int, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(subscription.ID)
	if err != nil {
		return false, err
	}
	written, err := queries.UpdatePlatformSubscription(ctx, sqlc.UpdatePlatformSubscriptionParams{
		TargetUrl: subscription.TargetURL, State: string(subscription.State), Now: momentOf(now),
		ID: id, ExpectedVersion: int32(expectedVersion), //nolint:gosec // a row's version
	})
	if err != nil {
		return false, subscriptionFailed("changing", err)
	}
	return written == 1, nil
}

// Rotate replaces the sealed secret.
func (PlatformSubscriptionRepository) Rotate(
	ctx context.Context, id shared.ID, secret repository.PlatformSecret, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return false, err
	}
	written, err := queries.RotatePlatformSubscriptionSecret(ctx, sqlc.RotatePlatformSubscriptionSecretParams{
		SecretEnc: secret.Ciphertext, SecretKeyID: secret.KeyID, Now: momentOf(now), ID: key,
	})
	if err != nil {
		return false, subscriptionFailed("rotating", err)
	}
	return written == 1, nil
}

// Delete removes one, its deliveries with it.
func (PlatformSubscriptionRepository) Delete(ctx context.Context, id shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return false, err
	}
	removed, err := queries.DeletePlatformSubscription(ctx, key)
	if err != nil {
		return false, subscriptionFailed("removing", err)
	}
	return removed == 1, nil
}

// Active answers the subscriptions an event is sent to.
func (PlatformSubscriptionRepository) Active(ctx context.Context) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ActivePlatformSubscriptionIDs(ctx)
	if err != nil {
		return nil, subscriptionFailed("listing", err)
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

// MarkFromFile records that an enforcing file decides the subscription.
func (PlatformSubscriptionRepository) MarkFromFile(ctx context.Context, id shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	if err := queries.MarkPlatformSubscriptionFromFile(ctx, key); err != nil {
		return subscriptionFailed("marking", err)
	}
	return nil
}

// Outcome records the last attempt for the screen.
func (PlatformSubscriptionRepository) Outcome(ctx context.Context, id shared.ID, deliveredAt time.Time, lastError string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	params := sqlc.RecordPlatformSubscriptionOutcomeParams{DeliveredAt: momentOf(deliveredAt), ID: key}
	if lastError != "" {
		params.LastError = &lastError
	}
	if err := queries.RecordPlatformSubscriptionOutcome(ctx, params); err != nil {
		return subscriptionFailed("recording an attempt of", err)
	}
	return nil
}

func platformSubscriptionOf(row sqlc.FindPlatformSubscriptionRow) (integration.PlatformSubscription, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return integration.PlatformSubscription{}, err
	}
	return integration.PlatformSubscription{
		ID: id, TargetURL: row.TargetUrl, State: integration.PlatformState(row.State),
		LastError: stringFrom(row.LastError), LastDeliveryAt: timeFrom(row.LastDeliveryAt),
		DeadLetters: row.DeadLetters, FromFile: row.FromFile, CreatedAt: timeFrom(row.CreatedAt),
		Version: int(row.Version),
	}, nil
}
