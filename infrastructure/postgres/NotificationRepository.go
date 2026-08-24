// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"math"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// NotificationRepository stores the notification records and the preferences (C-09).
type NotificationRepository struct{}

func NewNotificationRepository() NotificationRepository { return NotificationRepository{} }

var _ repository.Notifications = NotificationRepository{}

// NotificationPreferenceRepository stores what people have said about being told.
//
// A type of its own rather than more methods on the one above, because both interfaces need a
// Find and a Save and one type cannot carry two of each. The split is honest anyway: a record and
// a preference are different tables with different lifetimes, and the delivery reads one of them
// without ever writing the other.
type NotificationPreferenceRepository struct{}

func NewNotificationPreferenceRepository() NotificationPreferenceRepository {
	return NotificationPreferenceRepository{}
}

var _ repository.Preferences = NotificationPreferenceRepository{}

// Insert writes a record, reporting whether this call was the one that wrote it.
func (r NotificationRepository) Insert(
	ctx context.Context, record domain.Notification,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(record.ID)
	if err != nil {
		return false, err
	}
	recipient, err := uuidOf(record.RecipientID)
	if err != nil {
		return false, err
	}
	eventID, err := optionalUUID(record.EventID)
	if err != nil {
		return false, err
	}
	itemID, err := optionalUUID(record.ItemID)
	if err != nil {
		return false, err
	}
	actorID, err := optionalUUID(record.ActorID)
	if err != nil {
		return false, err
	}

	written, err := queries.InsertNotification(ctx, sqlc.InsertNotificationParams{
		ID:          id,
		RecipientID: recipient,
		Category:    string(record.Category),
		Channel:     string(record.Channel),
		State:       string(record.State),
		Reason:      optionalText(record.Reason),
		EventID:     eventID,
		ItemID:      itemID,
		ActorID:     actorID,
		CreatedAt:   timestampOf(record.CreatedAt),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the notification %s: %w", record.ID, err))
	}
	return written == 1, nil
}

// Find returns the record.
func (r NotificationRepository) Find(
	ctx context.Context, id shared.ID,
) (domain.Notification, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Notification{}, err
	}
	recordID, err := uuidOf(id)
	if err != nil {
		return domain.Notification{}, err
	}

	row, err := queries.FindNotification(ctx, recordID)
	if err != nil {
		if IsNoRows(err) {
			return domain.Notification{}, shared.ErrNotFound
		}
		return domain.Notification{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the notification %s: %w", id, err))
	}
	return notificationFrom(row)
}

// Save writes back what happened to a record.
func (r NotificationRepository) Save(ctx context.Context, record domain.Notification) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(record.ID)
	if err != nil {
		return err
	}

	var sentAt time.Time
	if record.SentAt != nil {
		sentAt = *record.SentAt
	}

	matched, err := queries.SaveNotificationOutcome(ctx, sqlc.SaveNotificationOutcomeParams{
		ID:       id,
		State:    string(record.State),
		Reason:   optionalText(record.Reason),
		SentAt:   timestampOf(sentAt),
		Attempts: int32(record.Attempts), //nolint:gosec // bounded by the queue's attempt budget
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the outcome of the notification %s: %w", record.ID, err))
	}
	if matched == 0 {
		// The record is gone - the retention sweep reached it while its delivery was in flight.
		// Reported rather than swallowed: a delivery writing into nothing is a state the caller
		// decides about, not one a repository decides for it.
		return shared.ErrNotFound
	}
	return nil
}

// DeleteExpired removes one batch of records written before the cutoff.
func (r NotificationRepository) DeleteExpired(
	ctx context.Context, cutoff time.Time, batch int,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	removed, err := queries.DeleteExpiredNotifications(ctx, sqlc.DeleteExpiredNotificationsParams{
		Cutoff: timestampOf(cutoff),
		Batch:  boundedInt32(batch),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing expired notifications: %w", err))
	}
	return int(removed), nil
}

// CountExpired reports how many records are due, counted no higher than ceiling.
func (r NotificationRepository) CountExpired(
	ctx context.Context, cutoff time.Time, ceiling int,
) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	due, err := queries.CountExpiredNotifications(ctx, sqlc.CountExpiredNotificationsParams{
		Cutoff:  timestampOf(cutoff),
		Ceiling: boundedInt32(ceiling),
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting expired notifications: %w", err))
	}
	return int(due), nil
}

// Find returns one preference, or ErrNotFound where the account has said nothing.
func (r NotificationPreferenceRepository) Find(
	ctx context.Context, accountID shared.ID, category domain.Category, channel domain.Channel,
) (domain.Preference, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Preference{}, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return domain.Preference{}, err
	}

	row, err := queries.FindNotificationPreference(ctx, sqlc.FindNotificationPreferenceParams{
		AccountID: account,
		Category:  string(category),
		Channel:   string(channel),
	})
	if err != nil {
		if IsNoRows(err) {
			return domain.Preference{}, shared.ErrNotFound
		}
		return domain.Preference{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading a notification preference: %w", err))
	}
	return preferenceFrom(row)
}

// Save writes a preference, replacing whatever the account said before.
func (r NotificationPreferenceRepository) Save(
	ctx context.Context, preference domain.Preference,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	account, err := uuidOf(preference.AccountID)
	if err != nil {
		return err
	}

	err = queries.SaveNotificationPreference(ctx, sqlc.SaveNotificationPreferenceParams{
		AccountID:    account,
		Category:     string(preference.Category),
		Channel:      string(preference.Channel),
		Enabled:      preference.Enabled,
		IncludeTitle: preference.IncludeTitle,
		UpdatedAt:    timestampOf(preference.UpdatedAt),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing a notification preference: %w", err))
	}
	return nil
}

func notificationFrom(row sqlc.Notification) (domain.Notification, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Notification{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return domain.Notification{}, err
	}
	recipientID, err := idFrom(row.RecipientID)
	if err != nil {
		return domain.Notification{}, err
	}
	eventID, err := optionalID(row.EventID)
	if err != nil {
		return domain.Notification{}, err
	}
	itemID, err := optionalID(row.ItemID)
	if err != nil {
		return domain.Notification{}, err
	}
	actorID, err := optionalID(row.ActorID)
	if err != nil {
		return domain.Notification{}, err
	}

	var reason string
	if row.Reason != nil {
		reason = *row.Reason
	}
	return domain.Notification{
		ID:          id,
		TenantID:    tenantID,
		RecipientID: recipientID,
		Category:    domain.Category(row.Category),
		Channel:     domain.Channel(row.Channel),
		State:       domain.State(row.State),
		Reason:      reason,
		EventID:     eventID,
		ItemID:      itemID,
		ActorID:     actorID,
		CreatedAt:   row.CreatedAt.Time.UTC(),
		SentAt:      optionalTime(row.SentAt),
		Attempts:    int(row.Attempts),
	}, nil
}

func preferenceFrom(row sqlc.NotificationPreference) (domain.Preference, error) {
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return domain.Preference{}, err
	}
	accountID, err := idFrom(row.AccountID)
	if err != nil {
		return domain.Preference{}, err
	}
	return domain.Preference{
		TenantID:     tenantID,
		AccountID:    accountID,
		Category:     domain.Category(row.Category),
		Channel:      domain.Channel(row.Channel),
		Enabled:      row.Enabled,
		IncludeTitle: row.IncludeTitle,
		UpdatedAt:    row.UpdatedAt.Time.UTC(),
	}, nil
}

// boundedInt32 keeps a configured batch inside the column type. A configuration that asked for
// more rows than an int32 can name is a configuration mistake, and clamping it is a kinder answer
// than an overflow that silently asks for a negative batch.
func boundedInt32(value int) int32 {
	switch {
	case value < 0:
		return 0
	case value > math.MaxInt32:
		return math.MaxInt32
	default:
		return int32(value)
	}
}
