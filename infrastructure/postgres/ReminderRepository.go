// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// ReminderRepository stores the reminders beside the entries (D-02).
//
// No cursor codec, unlike the comment store: what one entry may carry is bounded where reminders
// are written, so the list is one answer rather than a page.
type ReminderRepository struct{}

func NewReminderRepository() ReminderRepository { return ReminderRepository{} }

var _ repository.Reminders = ReminderRepository{}

// Find returns the reminder.
func (r ReminderRepository) Find(ctx context.Context, id shared.ID) (work.Reminder, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.Reminder{}, err
	}
	reminderID, err := uuidOf(id)
	if err != nil {
		return work.Reminder{}, err
	}

	row, err := queries.FindReminder(ctx, reminderID)
	if err != nil {
		if IsNoRows(err) {
			return work.Reminder{}, shared.ErrNotFound
		}
		return work.Reminder{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the reminder %s: %w", id, err))
	}
	return reminderFrom(
		row.ID, row.TenantID, row.ItemID, row.OffsetSpec, row.Channels, row.Recipients,
		row.State, row.FireAt, row.CreatedAt, row.UpdatedAt, row.Version,
	)
}

// ListForItem returns one entry's reminders, oldest first.
func (r ReminderRepository) ListForItem(
	ctx context.Context, itemID shared.ID,
) ([]work.Reminder, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListRemindersOfItem(ctx, id)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the reminders of %s: %w", itemID, err))
	}

	reminders := make([]work.Reminder, 0, len(rows))
	for _, row := range rows {
		reminder, err := reminderFrom(
			row.ID, row.TenantID, row.ItemID, row.OffsetSpec, row.Channels, row.Recipients,
			row.State, row.FireAt, row.CreatedAt, row.UpdatedAt, row.Version,
		)
		if err != nil {
			return nil, err
		}
		reminders = append(reminders, reminder)
	}
	return reminders, nil
}

// ListPendingForItem returns the entry's reminders that are still waiting.
func (r ReminderRepository) ListPendingForItem(
	ctx context.Context, itemID shared.ID,
) ([]work.Reminder, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListPendingRemindersOfItem(ctx, id)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the pending reminders of %s: %w", itemID, err))
	}

	reminders := make([]work.Reminder, 0, len(rows))
	for _, row := range rows {
		reminder, err := reminderFrom(
			row.ID, row.TenantID, row.ItemID, row.OffsetSpec, row.Channels, row.Recipients,
			row.State, row.FireAt, row.CreatedAt, row.UpdatedAt, row.Version,
		)
		if err != nil {
			return nil, err
		}
		reminders = append(reminders, reminder)
	}
	return reminders, nil
}

// CountForItem returns how many reminders the entry carries.
func (r ReminderRepository) CountForItem(ctx context.Context, itemID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	id, err := uuidOf(itemID)
	if err != nil {
		return 0, err
	}

	count, err := queries.CountRemindersOfItem(ctx, id)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the reminders of %s: %w", itemID, err))
	}
	return int(count), nil
}

// Insert writes a new reminder.
func (r ReminderRepository) Insert(ctx context.Context, reminder work.Reminder) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(reminder.ID)
	if err != nil {
		return err
	}
	itemID, err := uuidOf(reminder.ItemID)
	if err != nil {
		return err
	}
	recipients, err := recipientUUIDs(reminder.Recipients)
	if err != nil {
		return err
	}

	err = queries.InsertReminder(ctx, sqlc.InsertReminderParams{
		ID:         id,
		ItemID:     itemID,
		OffsetSpec: reminder.Offset.Spec,
		Channels:   channelNames(reminder.Channels),
		Recipients: recipients,
		State:      reminder.State.String(),
		FireAt:     optionalTimestamp(reminder.FireAt),
		CreatedAt:  timestampOf(reminder.CreatedAt),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the reminder %s: %w", reminder.ID, err))
	}
	return nil
}

// Update writes the edited fields under the optimistic lock. A reminder that is no longer pending
// is never matched, so an edit racing a firing comes back as a version conflict - the same answer
// as any other row that moved on, which is all the caller can act on either way.
func (r ReminderRepository) Update(
	ctx context.Context, reminder work.Reminder, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(reminder.ID)
	if err != nil {
		return err
	}
	recipients, err := recipientUUIDs(reminder.Recipients)
	if err != nil {
		return err
	}
	if reminder.UpdatedAt == nil {
		// The domain stamps every edit; a write without the stamp is this code disagreeing with
		// itself rather than a request that can be fixed.
		return shared.ErrInternal.
			WithDetail("postgres.row_incoherent").
			WithCause(fmt.Errorf("the edit of reminder %s carries no stamp", reminder.ID))
	}

	affected, err := queries.UpdateReminder(ctx, sqlc.UpdateReminderParams{
		OffsetSpec: reminder.Offset.Spec,
		Channels:   channelNames(reminder.Channels),
		Recipients: recipients,
		FireAt:     optionalTimestamp(reminder.FireAt),
		UpdatedAt:  timestampOf(*reminder.UpdatedAt),
		ID:         id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the reminder %s: %w", reminder.ID, err))
	}
	return reminderConflictIfUntouched(affected, reminder.ID, expectedVersion)
}

// Reschedule writes a recomputed moment alone.
//
// No version and no conflict: the row was not edited, and a reminder that has fired in the
// meantime is simply not matched - which is the right outcome rather than an error, because a due
// date moving is not an instruction about a reminder that already went out.
func (r ReminderRepository) Reschedule(ctx context.Context, reminder work.Reminder) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(reminder.ID)
	if err != nil {
		return err
	}

	if _, err := queries.SetReminderFireAt(ctx, sqlc.SetReminderFireAtParams{
		FireAt: optionalTimestamp(reminder.FireAt),
		ID:     id,
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("rescheduling the reminder %s: %w", reminder.ID, err))
	}
	return nil
}

// Delete removes the row under the optimistic lock.
func (r ReminderRepository) Delete(
	ctx context.Context, id shared.ID, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	reminderID, err := uuidOf(id)
	if err != nil {
		return err
	}

	affected, err := queries.DeleteReminder(ctx, sqlc.DeleteReminderParams{
		ID: reminderID,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("deleting the reminder %s: %w", id, err))
	}
	return reminderConflictIfUntouched(affected, id, expectedVersion)
}

// ClaimDue takes what is due, locking the rows for the caller's transaction.
func (r ReminderRepository) ClaimDue(
	ctx context.Context, now time.Time, limit int,
) ([]work.Reminder, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListDueReminders(ctx, sqlc.ListDueRemindersParams{
		Now: timestampOf(now),
		//nolint:gosec // G115: the batch is this process's own constant, not a value from a request
		BatchSize: int32(limit),
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("claiming the due reminders: %w", err))
	}

	reminders := make([]work.Reminder, 0, len(rows))
	for _, row := range rows {
		reminder, err := reminderFrom(
			row.ID, row.TenantID, row.ItemID, row.OffsetSpec, row.Channels, row.Recipients,
			row.State, row.FireAt, row.CreatedAt, row.UpdatedAt, row.Version,
		)
		if err != nil {
			return nil, err
		}
		reminders = append(reminders, reminder)
	}
	return reminders, nil
}

// Settle writes the guarded transition and reports whether this caller made it.
func (r ReminderRepository) Settle(
	ctx context.Context, id shared.ID, state work.ReminderState,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	reminderID, err := uuidOf(id)
	if err != nil {
		return false, err
	}

	affected, err := queries.SetReminderState(ctx, sqlc.SetReminderStateParams{
		State: state.String(),
		ID:    reminderID,
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("settling the reminder %s: %w", id, err))
	}
	return affected != 0, nil
}

// NextMoment answers when the tenant next owes a reminder.
func (r ReminderRepository) NextMoment(ctx context.Context) (*time.Time, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	next, err := queries.NextReminderMoment(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the tenant's next reminder: %w", err))
	}
	return optionalTime(next), nil
}

// reminderConflictIfUntouched is the shared answer for a write that matched nothing: the row moved
// on, or - through row level security - was never this tenant's to move. One answer for both,
// deliberately (multi-tenancy.md §2).
func reminderConflictIfUntouched(affected int64, id shared.ID, expectedVersion int) error {
	if affected != 0 {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("reminders.version_conflict").
		WithParams(map[string]string{
			"reminder_id": id.String(), "expected_version": fmt.Sprint(expectedVersion),
		})
}

// channelNames spells the channels the column's way.
func channelNames(channels []work.ReminderChannel) []string {
	names := make([]string, 0, len(channels))
	for _, channel := range channels {
		names = append(names, channel.String())
	}
	return names
}

// recipientUUIDs maps the recipients onto the column's array, and the empty list onto an empty
// array rather than NULL - empty means the assignee and the members, which is a list, not an
// absence.
func recipientUUIDs(recipients []shared.ID) ([]pgtype.UUID, error) {
	ids := make([]pgtype.UUID, 0, len(recipients))
	for _, recipient := range recipients {
		id, err := uuidOf(recipient)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// reminderFrom maps a stored row onto the domain's reminder. One mapper for every select, so they
// cannot disagree about a field.
//
// The offset spec is parsed on the way out, which is also where a row written by an older version
// of this code would be caught: the column is text, and text is only an offset if it still reads
// as one.
func reminderFrom(
	id, tenantID, itemID pgtype.UUID, offsetSpec string, channels []string,
	recipients []pgtype.UUID, state string,
	fireAt, createdAt, updatedAt pgtype.Timestamptz, version int32,
) (work.Reminder, error) {
	reminderID, err := idFrom(id)
	if err != nil {
		return work.Reminder{}, err
	}
	tenant, err := idFrom(tenantID)
	if err != nil {
		return work.Reminder{}, err
	}
	item, err := idFrom(itemID)
	if err != nil {
		return work.Reminder{}, err
	}
	offset, err := work.ParseReminderOffset(offsetSpec)
	if err != nil {
		return work.Reminder{}, shared.ErrInternal.
			WithDetail("postgres.row_incoherent").
			WithCause(fmt.Errorf("the reminder %s carries %q as its offset", reminderID, offsetSpec))
	}
	if !createdAt.Valid {
		return work.Reminder{}, shared.ErrInternal.WithDetail("postgres.row_incoherent")
	}

	carried := make([]work.ReminderChannel, 0, len(channels))
	for _, name := range channels {
		carried = append(carried, work.ReminderChannel(name))
	}
	reached := make([]shared.ID, 0, len(recipients))
	for _, recipient := range recipients {
		accountID, err := idFrom(recipient)
		if err != nil {
			return work.Reminder{}, err
		}
		reached = append(reached, accountID)
	}

	return work.Reminder{
		ID:         reminderID,
		TenantID:   tenant,
		ItemID:     item,
		Offset:     offset,
		Channels:   carried,
		Recipients: reached,
		State:      work.ReminderState(state),
		FireAt:     optionalTime(fireAt),
		CreatedAt:  timeFrom(createdAt),
		UpdatedAt:  optionalTime(updatedAt),
		Version:    int(version),
	}, nil
}
