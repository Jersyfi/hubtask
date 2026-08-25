// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"strconv"
	"time"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	UpdateReminderName = "UpdateReminder"
	DeleteReminderName = "DeleteReminder"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	ReminderUpdatedAction audit.Action = "reminder.updated"
	ReminderDeletedAction audit.Action = "reminder.deleted"
)

// UpdateReminder changes a reminder that has not fired yet.
type UpdateReminder struct {
	Writer ReminderWriter
}

// DeleteReminder removes a reminder.
type DeleteReminder struct {
	Writer ReminderWriter
}

// ChangeReminderCommand is the input of both, typed. The patch is the update's; a deletion carries
// none, which is the one field the two do not share.
type ChangeReminderCommand struct {
	ItemID     shared.ID
	ReminderID shared.ID
	Patch      domain.ReminderPatch
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read
	// none and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// Execute changes the reminder and returns it.
func (h UpdateReminder) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeReminderCommand,
) (domain.Reminder, error) {
	if cmd.Patch.IsEmpty() {
		return domain.Reminder{}, shared.ErrValidation.
			WithDetail("reminders.update_empty").
			WithFields(shared.FieldError{Path: "/", Code: "reminders.update_empty"})
	}
	return h.Writer.change(ctx, actor, cmd, updatingReminder)
}

// Execute removes the reminder.
func (h DeleteReminder) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeReminderCommand,
) error {
	_, err := h.Writer.change(ctx, actor, cmd, removingReminder)
	return err
}

// reminderChange is which change the caller asked for. Not a boolean, for the reason the comment's
// is not: this is the parameter that decides which of two audit trails is written.
type reminderChange bool

const (
	updatingReminder reminderChange = true
	removingReminder reminderChange = false
)

func (c reminderChange) action() audit.Action {
	if c == updatingReminder {
		return ReminderUpdatedAction
	}
	return ReminderDeletedAction
}

// change is the whole of both use cases.
func (w ReminderWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeReminderCommand,
	want reminderChange,
) (domain.Reminder, error) {
	if cmd.ItemID.IsZero() {
		return domain.Reminder{}, itemIDRequired()
	}
	if cmd.ReminderID.IsZero() {
		return domain.Reminder{}, shared.ErrValidation.
			WithDetail("reminders.reminder_id_required").
			WithFields(shared.FieldError{
				Path: "/reminder_id", Code: "reminders.reminder_id_required",
			})
	}

	// The reminder, its entry and the collection are read before the permission question, because
	// the answer depends on the path (domain-model.md §3.2). Nothing read here is trusted
	// afterwards - the state that decides the write is read again inside the transaction.
	subject, collection, err := w.readReminderScope(ctx, actor, cmd)
	if err != nil {
		return domain.Reminder{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     want.action(),
		TokenScope: remindersWrite,
		TargetType: reminderTarget,
		TargetID:   cmd.ReminderID,
		On:         changing(subject),
	}); err != nil {
		return domain.Reminder{}, err
	}

	if want == updatingReminder && cmd.Patch.Recipients != nil {
		if err := w.ensureRecipientsCanSee(
			ctx, actor, *cmd.Patch.Recipients, collection,
		); err != nil {
			return domain.Reminder{}, err
		}
	}

	var changed domain.Reminder
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		current, err := w.findReminder(ctx, cmd.ReminderID)
		if err != nil {
			return err
		}
		// I-C3, I-W4: an archived or trashed entry is read-only, and its reminders inherit that -
		// for the deletion too, which is also a write. Removal for good has its own path (the
		// purge, through the entry), and it does not run through here.
		item, err := w.readWritableItem(ctx, current.ItemID)
		if err != nil {
			return err
		}

		if want == removingReminder {
			return w.remove(ctx, actor, current, item, cmd.ExpectedVersion, now)
		}

		wanted, changes, err := current.Patched(cmd.Patch, item.Due, now)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// Already in the state asked for. Nothing is written, no version is spent and nothing
			// is recorded - which is what makes a repeat harmless rather than merely accepted.
			//
			// The If-Match is still honoured: a caller that wrote against a version somebody else
			// has moved on is told so, even when its own change would have been a no-op, because
			// the state it was reasoning about is not the state that is there.
			if err := ensureReminderVersion(current, cmd.ExpectedVersion); err != nil {
				return err
			}
			changed = current
			return nil
		}

		expected := cmd.ExpectedVersion
		if expected == 0 {
			// The caller read no version and accepts whatever is there. Not the same as skipping
			// the check: the version in hand is still the one the update matches on, so a
			// concurrent write between the read and here is still caught.
			expected = current.Version
		}
		if err := w.Reminders.Update(ctx, wanted, expected); err != nil {
			return err
		}
		wanted.Version = expected + 1

		if err := w.recordFieldChanges(ctx, wanted, item, actor, changes); err != nil {
			return err
		}
		if err := w.recordAudit(
			ctx, wanted, actor, ReminderUpdatedAction, reminderFieldAudit(changes), now,
		); err != nil {
			return err
		}
		changed = wanted
		return nil
	})
	if err != nil {
		return domain.Reminder{}, err
	}
	return changed, nil
}

// remove deletes the row and records what the deletion owes: the tombstone offline clients need,
// and the audit entry. A reminder that has fired is deleted like any other - the row is a plan,
// and removing a plan that was carried out is housekeeping rather than a rewrite of history.
func (w ReminderWriter) remove(
	ctx context.Context, actor appshared.ActorContext, reminder domain.Reminder,
	item domain.WorkItem, expectedVersion int, now time.Time,
) error {
	expected := expectedVersion
	if expected == 0 {
		expected = reminder.Version
	}
	if err := w.Reminders.Delete(ctx, reminder.ID, expected); err != nil {
		return err
	}

	if err := w.Changes.Record(ctx, changelog.Change{
		TenantID:    reminder.TenantID,
		Entity:      reminderTarget,
		EntityID:    reminder.ID,
		Op:          changelog.Delete,
		ContainerID: item.CollectionID,
		ActorID:     actor.AccountID,
		HLC:         w.HLC.Next(),
	}); err != nil {
		return err
	}
	return w.recordAudit(
		ctx, reminder, actor, ReminderDeletedAction, reminderAuditChanges(reminder), now)
}

// recordFieldChanges writes what an offline client has to be told: one entry per field that moved,
// each with its own HLC.
//
// That is the scalar rule of offline-sync.md §4.2 written down, and the acceptance's own case: two
// devices editing the offset and the channels converge to both, which one entry carrying the pair
// would destroy - the later HLC deciding the whole payload and silently discarding the other
// device's field. fire_at is not among them: it is derived from the offset and the entry's date,
// and a client that merged it would be merging the server's arithmetic.
func (w ReminderWriter) recordFieldChanges(
	ctx context.Context, reminder domain.Reminder, item domain.WorkItem,
	actor appshared.ActorContext, changes []domain.FieldChange,
) error {
	for _, change := range changes {
		err := w.Changes.Record(ctx, changelog.Change{
			TenantID:    reminder.TenantID,
			Entity:      reminderTarget,
			EntityID:    reminder.ID,
			Op:          changelog.Upsert,
			ContainerID: item.CollectionID,
			ActorID:     actor.AccountID,
			HLC:         w.HLC.Next(),
			Payload:     map[string]any{change.Field: change.To},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// reminderFieldAudit records both sides of every field that moved: "what was this reminder before
// somebody changed it" is not answerable from the new value alone (audit.md §4).
func reminderFieldAudit(changes []domain.FieldChange) []audit.Change {
	recorded := make([]audit.Change, 0, len(changes))
	for _, change := range changes {
		recorded = append(recorded, audit.Change{
			Field: change.Field, Classification: audit.Open,
			From: change.From, To: change.To,
		})
	}
	return recorded
}

// readReminderScope reads the reminder, the entry it sits on and that entry's collection, outside
// the write transaction, because the permission check needs the path first.
//
// A reminder reached through the wrong entry is refused by name rather than served: the route says
// which entry it belongs to, and answering for a different one would let a caller with access to
// one entry act on another's reminder.
func (w ReminderWriter) readReminderScope(
	ctx context.Context, actor appshared.ActorContext, cmd ChangeReminderCommand,
) (domain.WorkItem, domain.Container, error) {
	var reminder domain.Reminder
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		reminder, err = w.findReminder(ctx, cmd.ReminderID)
		return err
	})
	if err != nil {
		return domain.WorkItem{}, domain.Container{}, err
	}
	if reminder.ItemID != cmd.ItemID {
		return domain.WorkItem{}, domain.Container{}, shared.ErrValidation.
			WithDetail("reminders.not_on_item").
			WithFields(shared.FieldError{Path: "/item_id", Code: "reminders.not_on_item"})
	}
	return readItemScope(ctx, w.UnitOfWork, w.Items, w.Containers, actor, reminder.ItemID)
}

// ensureReminderVersion holds a no-op change to the version the caller read, for the reason
// ensureExpectedVersion does it for an entry: a caller reasoning about a version that has moved on
// is told so, whatever its own change would have written.
func ensureReminderVersion(reminder domain.Reminder, expected int) error {
	if expected == 0 || expected == reminder.Version {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("reminders.version_conflict").
		WithParams(map[string]string{
			"reminder_id": reminder.ID.String(), "expected_version": strconv.Itoa(expected),
		})
}

// Descriptor is the catalogue entry.
func (h UpdateReminder) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateReminderName,
		Summary: "Changes a reminder that has not fired yet: the offset, the channels, the " +
			"recipients. A merge patch - a field that is not sent is not touched - and the lists " +
			"are chosen whole, so the list sent replaces the list stored. A reminder that has " +
			"already fired or was cancelled is refused rather than given a new future.",
		SideEffects: "Writes the changed fields, records one change per field for offline clients " +
			"and writes an audit entry.",
		TokenScope: remindersWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry the reminder sits on.",
			},
			{
				Name: "reminder_id", Kind: usecase.KindID, Required: true,
				Description: "The reminder to change.",
			},
			{
				Name: "offset_spec", Kind: usecase.KindString,
				Description: "REL: with an ISO-8601 duration, or ABS: with an RFC 3339 instant.",
			},
			{
				Name: "channels", Kind: usecase.KindList,
				Description: "What carries the reminder. The list sent replaces the list stored.",
			},
			{
				Name: "recipients", Kind: usecase.KindIDList,
				Description: "Who is reminded. An empty list means the assignee and the entry's " +
					"members.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read (If-Match). Omitted accepts what is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ReminderUpdatedAction, TargetType: reminderTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateReminder) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := changeReminderCommand(in)
	if err != nil {
		return nil, err
	}

	patch := domain.ReminderPatch{OffsetSpec: in.OptionalString("offset_spec")}
	if in.Present("channels") {
		channels, err := in.StringList("channels")
		if err != nil {
			return nil, err
		}
		patch.Channels = &channels
	}
	if in.Present("recipients") {
		recipients, err := in.IDList("recipients")
		if err != nil {
			return nil, err
		}
		if recipients == nil {
			// An empty list is an instruction - "reach the assignee and the members" - and a nil
			// slice here would be indistinguishable from an absent member, which is not one.
			recipients = []shared.ID{}
		}
		patch.Recipients = &recipients
	}
	cmd.Patch = patch

	reminder, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return reminderOutput(reminder), nil
}

// Descriptor is the catalogue entry.
func (h DeleteReminder) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteReminderName,
		Summary: "Removes a reminder. A hard delete: a reminder is created and deleted whole, so " +
			"what somebody deleted is gone rather than kept as a cancelled row they can still " +
			"see. Deleting one that is not there answers that it is not there.",
		SideEffects: "Deletes the reminder, records the deletion for offline clients and writes " +
			"an audit entry.",
		TokenScope: remindersWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry the reminder sits on.",
			},
			{
				Name: "reminder_id", Kind: usecase.KindID, Required: true,
				Description: "The reminder to remove.",
			},
			{
				Name: "expected_version", Kind: usecase.KindInt,
				Description: "The version last read (If-Match). Omitted accepts what is there.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ReminderDeletedAction, TargetType: reminderTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DeleteReminder) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := changeReminderCommand(in)
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, cmd); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// changeReminderCommand reads what both changes share out of the catalogue's untyped input.
func changeReminderCommand(in usecase.Input) (ChangeReminderCommand, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return ChangeReminderCommand{}, err
	}
	reminderID, err := in.ID("reminder_id")
	if err != nil {
		return ChangeReminderCommand{}, err
	}
	return ChangeReminderCommand{
		ItemID: itemID, ReminderID: reminderID, ExpectedVersion: in.Int("expected_version"),
	}, nil
}
