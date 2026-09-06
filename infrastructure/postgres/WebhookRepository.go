// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// WebhookSubscriptionRepository stores the standing requests external systems make (G-03).
//
// It holds no key and does no sealing: the ciphertext arrives already sealed and leaves as it
// came. That is the same division the access token's hash uses in the other direction - the layer
// that owns the secret material is the one that owns the operation on it, and this one owns the
// row.
type WebhookSubscriptionRepository struct{}

func NewWebhookSubscriptionRepository() WebhookSubscriptionRepository {
	return WebhookSubscriptionRepository{}
}

func (WebhookSubscriptionRepository) Insert(
	ctx context.Context, stored repository.StoredSubscription,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	subscription := stored.Subscription
	id, err := uuidOf(subscription.ID)
	if err != nil {
		return err
	}
	createdBy, err := uuidOf(subscription.CreatedBy)
	if err != nil {
		return err
	}

	if err := queries.InsertWebhookSubscription(ctx, sqlc.InsertWebhookSubscriptionParams{
		ID:          id,
		TargetUrl:   subscription.TargetURL,
		EventTypes:  typeStrings(subscription.EventTypes),
		FilterExpr:  optionalText(subscription.Filter),
		SecretEnc:   stored.Secret.Ciphertext,
		SecretKeyID: optionalText(stored.Secret.KeyID),
		State:       string(subscription.State),
		CreatedBy:   createdBy,
		CreatedAt:   timestampOf(subscription.CreatedAt),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the webhook subscription: %w", err))
	}
	return nil
}

func (WebhookSubscriptionRepository) Find(
	ctx context.Context, subscriptionID shared.ID,
) (repository.StoredSubscription, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.StoredSubscription{}, err
	}
	id, err := uuidOf(subscriptionID)
	if err != nil {
		return repository.StoredSubscription{}, err
	}

	row, err := queries.FindWebhookSubscription(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return repository.StoredSubscription{}, shared.ErrNotFound.
				WithDetail("webhooks.subscription_not_found").
				WithParams(map[string]string{"webhook_id": subscriptionID.String()})
		}
		return repository.StoredSubscription{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the webhook subscription: %w", err))
	}
	return storedFrom(sqlc.ListWebhookSubscriptionsRow(row))
}

func (WebhookSubscriptionRepository) List(
	ctx context.Context,
) ([]domain.WebhookSubscription, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListWebhookSubscriptions(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the webhook subscriptions: %w", err))
	}

	subscriptions := make([]domain.WebhookSubscription, 0, len(rows))
	for _, row := range rows {
		stored, err := storedFrom(row)
		if err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, stored.Subscription)
	}
	return subscriptions, nil
}

func (WebhookSubscriptionRepository) WantingEvent(
	ctx context.Context, eventType event.Type,
) ([]repository.StoredSubscription, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.SubscriptionsForEventType(ctx, eventType.String())
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("finding the subscriptions for %s: %w", eventType, err))
	}

	wanting := make([]repository.StoredSubscription, 0, len(rows))
	for _, row := range rows {
		stored, err := storedFrom(sqlc.ListWebhookSubscriptionsRow(row))
		if err != nil {
			return nil, err
		}
		wanting = append(wanting, stored)
	}
	return wanting, nil
}

func (WebhookSubscriptionRepository) Update(
	ctx context.Context, subscription domain.WebhookSubscription, expectedVersion int,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(subscription.ID)
	if err != nil {
		return false, err
	}

	changed, err := queries.UpdateWebhookSubscription(ctx, sqlc.UpdateWebhookSubscriptionParams{
		ID:              id,
		TargetUrl:       subscription.TargetURL,
		EventTypes:      typeStrings(subscription.EventTypes),
		FilterExpr:      optionalText(subscription.Filter),
		State:           string(subscription.State),
		FailureCount:    int32(subscription.FailureCount), //nolint:gosec // G115: a count bounded by the disable threshold.
		LastError:       optionalText(subscription.LastError),
		DisabledAt:      instantOrNull(subscription.DisabledAt),
		ExpectedVersion: int32(expectedVersion), //nolint:gosec // G115: a row version.
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("updating the webhook subscription: %w", err))
	}
	return changed > 0, nil
}

func (WebhookSubscriptionRepository) Rotate(
	ctx context.Context, subscriptionID shared.ID, secret repository.SealedSecret,
	previousUntil time.Time, expectedVersion int,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(subscriptionID)
	if err != nil {
		return false, err
	}

	changed, err := queries.RotateWebhookSecret(ctx, sqlc.RotateWebhookSecretParams{
		ID:                  id,
		SecretEnc:           secret.Ciphertext,
		SecretKeyID:         optionalText(secret.KeyID),
		PreviousSecretUntil: instantOrNull(previousUntil),
		ExpectedVersion:     int32(expectedVersion), //nolint:gosec // G115: a row version.
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("rotating the webhook secret: %w", err))
	}
	return changed > 0, nil
}

func (WebhookSubscriptionRepository) Delete(
	ctx context.Context, subscriptionID shared.ID,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(subscriptionID)
	if err != nil {
		return false, err
	}

	removed, err := queries.DeleteWebhookSubscription(ctx, id)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("deleting the webhook subscription: %w", err))
	}
	return removed > 0, nil
}

// storedFrom maps a row. The three selects list the same columns in the same order, so one mapping
// serves all three - a second copy is how two of them come to disagree about what a null means.
func storedFrom(row sqlc.ListWebhookSubscriptionsRow) (repository.StoredSubscription, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return repository.StoredSubscription{}, err
	}
	createdBy, err := idFrom(row.CreatedBy)
	if err != nil {
		return repository.StoredSubscription{}, err
	}

	types := make([]event.Type, 0, len(row.EventTypes))
	for _, name := range row.EventTypes {
		types = append(types, event.Type(name))
	}

	return repository.StoredSubscription{
		Subscription: domain.WebhookSubscription{
			ID:                  id,
			TargetURL:           row.TargetUrl,
			EventTypes:          types,
			Filter:              stringFrom(row.FilterExpr),
			State:               domain.SubscriptionState(row.State),
			FailureCount:        int(row.FailureCount),
			LastError:           stringFrom(row.LastError),
			DisabledAt:          timeFrom(row.DisabledAt),
			PreviousSecretUntil: timeFrom(row.PreviousSecretUntil),
			CreatedBy:           createdBy,
			CreatedAt:           timeFrom(row.CreatedAt),
			Version:             int(row.Version),
		},
		Secret: repository.SealedSecret{
			Ciphertext: row.SecretEnc, KeyID: stringFrom(row.SecretKeyID),
		},
		Previous: repository.SealedSecret{
			Ciphertext: row.PreviousSecretEnc, KeyID: stringFrom(row.PreviousSecretKeyID),
		},
	}, nil
}

func typeStrings(types []event.Type) []string {
	names := make([]string, 0, len(types))
	for _, wanted := range types {
		names = append(names, wanted.String())
	}
	return names
}

// instantOrNull maps a zero time to SQL NULL. The zero value is how this domain says "there is no
// such moment" - not disabled, no grace - and a column that stored the year 1 would read as one.
func instantOrNull(at time.Time) pgtype.Timestamptz {
	if at.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: at.UTC(), Valid: true}
}

// WebhookDeliveryRepository is the log of what was sent and what became of it.
type WebhookDeliveryRepository struct{}

func NewWebhookDeliveryRepository() WebhookDeliveryRepository { return WebhookDeliveryRepository{} }

func (WebhookDeliveryRepository) Insert(ctx context.Context, delivery domain.WebhookDelivery) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(delivery.ID)
	if err != nil {
		return err
	}
	subscriptionID, err := uuidOf(delivery.SubscriptionID)
	if err != nil {
		return err
	}
	eventID, err := uuidOf(delivery.EventID)
	if err != nil {
		return err
	}

	if err := queries.InsertWebhookDelivery(ctx, sqlc.InsertWebhookDeliveryParams{
		ID:             id,
		SubscriptionID: subscriptionID,
		EventID:        eventID,
		Attempt:        int32(delivery.Attempt), //nolint:gosec // G115: bounded by MaxDeliveryAttempts.
		Status:         string(delivery.Status),
		NextAttemptAt:  instantOrNull(delivery.NextAttemptAt),
		CreatedAt:      timestampOf(delivery.CreatedAt),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the webhook delivery: %w", err))
	}
	return nil
}

func (WebhookDeliveryRepository) Find(
	ctx context.Context, deliveryID shared.ID,
) (domain.WebhookDelivery, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.WebhookDelivery{}, err
	}
	id, err := uuidOf(deliveryID)
	if err != nil {
		return domain.WebhookDelivery{}, err
	}

	row, err := queries.FindWebhookDelivery(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return domain.WebhookDelivery{}, shared.ErrNotFound.
				WithDetail("webhooks.delivery_not_found").
				WithParams(map[string]string{"delivery_id": deliveryID.String()})
		}
		return domain.WebhookDelivery{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the webhook delivery: %w", err))
	}
	return deliveryFrom(sqlc.WebhookDeliveriesRow(row))
}

func (WebhookDeliveryRepository) List(
	ctx context.Context, query repository.DeliveryQuery,
) ([]domain.WebhookDelivery, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	subscriptionID, err := uuidOf(query.SubscriptionID)
	if err != nil {
		return nil, err
	}

	params := sqlc.WebhookDeliveriesParams{
		SubscriptionID: subscriptionID,
		PageSize:       int32(query.PageSize), //nolint:gosec // G115: bounded by the page size cap.
	}
	if query.Status != "" {
		status := string(query.Status)
		params.Status = &status
	}
	if !query.Before.IsZero() {
		before, err := uuidOf(query.Before)
		if err != nil {
			return nil, err
		}
		params.Before = before
	}

	rows, err := queries.WebhookDeliveries(ctx, params)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the webhook deliveries: %w", err))
	}

	deliveries := make([]domain.WebhookDelivery, 0, len(rows))
	for _, row := range rows {
		delivery, err := deliveryFrom(row)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	return deliveries, nil
}

func (WebhookDeliveryRepository) RecordOutcome(
	ctx context.Context, outcome repository.DeliveryOutcome,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(outcome.ID)
	if err != nil {
		return err
	}

	params := sqlc.RecordWebhookDeliveryOutcomeParams{
		ID:            id,
		Status:        string(outcome.Status),
		NextAttemptAt: instantOrNull(outcome.NextAttemptAt),
		ErrorCode:     optionalText(outcome.ErrorCode),
	}
	if outcome.ResponseStatus != 0 {
		// Zero means the target answered nothing at all - a refused connection, a timeout, a name
		// that does not resolve - and NULL is what says that, where 0 would read as a status code.
		status := int32(outcome.ResponseStatus) //nolint:gosec // G115: an HTTP status.
		params.ResponseStatus = &status
	}

	if _, err := queries.RecordWebhookDeliveryOutcome(ctx, params); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the webhook delivery outcome: %w", err))
	}
	return nil
}

func deliveryFrom(row sqlc.WebhookDeliveriesRow) (domain.WebhookDelivery, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.WebhookDelivery{}, err
	}
	subscriptionID, err := idFrom(row.SubscriptionID)
	if err != nil {
		return domain.WebhookDelivery{}, err
	}
	eventID, err := idFrom(row.EventID)
	if err != nil {
		return domain.WebhookDelivery{}, err
	}

	delivery := domain.WebhookDelivery{
		ID: id, SubscriptionID: subscriptionID, EventID: eventID,
		Attempt:       int(row.Attempt),
		Status:        domain.DeliveryStatus(row.Status),
		ErrorCode:     stringFrom(row.ErrorCode),
		NextAttemptAt: timeFrom(row.NextAttemptAt),
		CreatedAt:     timeFrom(row.CreatedAt),
	}
	if row.ResponseStatus != nil {
		delivery.ResponseStatus = int(*row.ResponseStatus)
	}
	return delivery, nil
}

var _ repository.WebhookSealings = WebhookSubscriptionRepository{}

func (r WebhookSubscriptionRepository) SealedNotUnder(
	ctx context.Context, keyID string,
) ([]repository.StoredSubscription, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.WebhookSubscriptionsSealedNotUnder(ctx, &keyID)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the subscriptions to re-seal: %w", err))
	}
	stored := make([]repository.StoredSubscription, 0, len(rows))
	for _, row := range rows {
		subscription, err := storedFrom(sqlc.ListWebhookSubscriptionsRow(row))
		if err != nil {
			return nil, err
		}
		stored = append(stored, subscription)
	}
	return stored, nil
}

func (r WebhookSubscriptionRepository) Rewrap(
	ctx context.Context, id shared.ID, secret, previous repository.SealedSecret, expectedVersion int,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return false, err
	}
	params := sqlc.RewrapWebhookSecretsParams{
		SecretEnc: secret.Ciphertext, SecretKeyID: optionalText(secret.KeyID),
		ID: key,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	}
	if !previous.IsZero() {
		params.PreviousSecretEnc = previous.Ciphertext
		params.PreviousSecretKeyID = optionalText(previous.KeyID)
	}
	changed, err := queries.RewrapWebhookSecrets(ctx, params)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("re-sealing webhook subscription %s: %w", id, err))
	}
	return changed > 0, nil
}
