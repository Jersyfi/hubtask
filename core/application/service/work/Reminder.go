// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateReminderName = "CreateReminder"
	ListRemindersName  = "ListReminders"

	reminderTarget = "reminder"
	remindersWrite = "reminders:write"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	ReminderCreatedAction audit.Action = "reminder.created"
	// RemindersReadAction is the audit code of an attempted read, declared for the reason
	// CommentsReadAction is: an ordinary read writes no entry, a refused one does.
	RemindersReadAction audit.Action = "reminder.list_read"
)

// ReminderWriter is what every use case that writes a reminder shares: the same reads, the same
// permission question, and the same two records - the change log entry for offline clients, and
// the audit entry.
//
// Two records rather than four. There is no reminder event in the §4 catalogue and none is
// invented here: what consumers react to is the entry moving (`item.due_changed`) and the reminder
// firing (D-03), not somebody writing one down. And there is no step of the entry's history
// either: a reminder is a person's own arrangement with the clock rather than a change to the work
// - the history says what happened to the entry, and "Anna set herself a reminder" is not that.
// Both decisions are recorded in the pull request, because the next reader will ask.
type ReminderWriter struct {
	Reminders  repository.Reminders
	Items      repository.Items
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	Authorizer Authorizer
	// Visibility answers the one question the permission cannot: whether the account somebody
	// named can see the entry at all. The same question an assignment asks (C-01).
	Visibility Visibility
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	// Jobs is where the tenant's next wake-up is asked for. The write that makes something due is
	// what seeds it, because nothing may enumerate tenants (multi-tenancy.md §2.1) - the same
	// shape the retention sweep and the media reconciliation already have (D-03).
	Jobs       queue.Queue
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// CreateReminder puts a reminder on an entry.
type CreateReminder struct {
	Writer ReminderWriter
}

// CreateReminderCommand is the input, typed.
type CreateReminderCommand struct {
	ItemID     shared.ID
	OffsetSpec string
	// Channels as the caller named them, and empty for the default. Strings rather than the
	// domain's type, because an unknown channel is a validation answer rather than a parse error
	// (ADR-0011): the domain names the value it refused.
	Channels   []string
	Recipients []shared.ID
}

// Execute writes the reminder and returns it.
func (h CreateReminder) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateReminderCommand,
) (domain.Reminder, error) {
	w := h.Writer
	if cmd.ItemID.IsZero() {
		return domain.Reminder{}, itemIDRequired()
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	// Nothing read here is trusted afterwards - the state that decides the write is read again
	// inside the transaction.
	subject, collection, err := readItemScope(
		ctx, w.UnitOfWork, w.Items, w.Containers, actor, cmd.ItemID)
	if err != nil {
		return domain.Reminder{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	//
	// The entry's own permission and nothing narrower. A reminder is a property of the entry
	// rather than somebody's words about it, and the row carries no author to narrow by - whoever
	// may change the entry may arrange its reminders (recorded in the pull request).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     ReminderCreatedAction,
		TokenScope: remindersWrite,
		TargetType: reminderTarget,
		// The reminder does not exist yet, so the refusal names the entry it would have sat on.
		TargetID: cmd.ItemID,
		On:       changing(subject),
	}); err != nil {
		return domain.Reminder{}, err
	}

	// And the second person's question, which is a different one: the actor may write here, and
	// everybody they named still has to be able to see what they are being reminded of. Outside
	// the transaction for the reason the check above is outside it - it reads through the
	// authorisation service, which opens its own.
	if err := w.ensureRecipientsCanSee(ctx, actor, cmd.Recipients, collection); err != nil {
		return domain.Reminder{}, err
	}

	var created domain.Reminder
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		item, err := w.readWritableItem(ctx, cmd.ItemID)
		if err != nil {
			return err
		}

		// Counted inside the transaction, so that two concurrent creations cannot both find room
		// for the last one.
		existing, err := w.Reminders.CountForItem(ctx, item.ID)
		if err != nil {
			return err
		}
		if err := domain.EnsureReminderCapacity(existing); err != nil {
			return err
		}

		reminder, err := domain.NewReminder(domain.NewReminderInput{
			ID:         w.IDs.NewID(),
			TenantID:   actor.TenantID,
			ItemID:     item.ID,
			OffsetSpec: cmd.OffsetSpec,
			Channels:   cmd.Channels,
			Recipients: cmd.Recipients,
			// The entry's due date as it is stored, which is what a relative offset counts from -
			// read here rather than passed in, so that a reminder cannot be written against a
			// date the caller last saw an hour ago.
			Due: item.Due,
			Now: now,
		})
		if err != nil {
			return err
		}
		if err := w.Reminders.Insert(ctx, reminder); err != nil {
			return err
		}

		if err := w.recordUpsert(ctx, reminder, item, actor, reminderOutput(reminder)); err != nil {
			return err
		}
		if err := w.recordAudit(
			ctx, reminder, actor, ReminderCreatedAction, reminderAuditChanges(reminder), now,
		); err != nil {
			return err
		}
		if err := scheduleReminderFire(
			ctx, w.Jobs, reminder.TenantID, earliestMoment(reminder.FireAt),
		); err != nil {
			return err
		}

		created = reminder
		return nil
	})
	if err != nil {
		return domain.Reminder{}, err
	}
	return created, nil
}

// readWritableItem reads the entry inside the writing transaction and applies the guards every
// reminder write shares: a collection that accepts no writes, an
// entry that is trashed or archived, and a type that carries no reminders at all (I-C3, I-W4).
func (w ReminderWriter) readWritableItem(
	ctx context.Context, itemID shared.ID,
) (domain.WorkItem, error) {
	item, err := findItem(ctx, w.Items, itemID)
	if err != nil {
		return domain.WorkItem{}, err
	}
	collection, err := findCollection(ctx, w.Containers, item.CollectionID)
	if err != nil {
		return domain.WorkItem{}, err
	}
	// I-C3: an archived or trashed collection is read-only, and its entries inherit that.
	if err := collection.EnsureAcceptsItems(); err != nil {
		return domain.WorkItem{}, err
	}
	profile, err := profileOf(ctx, w.Profiles, item.Type)
	if err != nil {
		return domain.WorkItem{}, err
	}
	if err := item.EnsureRemindable(profile); err != nil {
		return domain.WorkItem{}, err
	}
	return item, nil
}

// ensureRecipientsCanSee holds every named recipient to the reach an assignee is held to (C-01):
// somebody who cannot see the entry cannot be reminded of it, because the reminder would name
// something they may not open.
func (w ReminderWriter) ensureRecipientsCanSee(
	ctx context.Context, actor appshared.ActorContext, recipients []shared.ID,
	collection domain.Container,
) error {
	for _, recipient := range recipients {
		if recipient.IsZero() {
			continue
		}
		permitted, err := w.Visibility.CanSee(ctx, actor, recipient, containerPath(collection))
		if err != nil {
			return err
		}
		if !permitted {
			return shared.ErrValidation.
				WithDetail("reminders.recipient_without_access").
				WithParams(map[string]string{"account_id": recipient.String()}).
				WithFields(shared.FieldError{
					Path: "/recipients", Code: "reminders.recipient_without_access",
					Params: map[string]string{"account_id": recipient.String()},
				})
		}
	}
	return nil
}

// findReminder reads the reminder, or answers that there is none - which is also the answer for
// one that belongs to another tenant, because row level security makes the two indistinguishable
// (multi-tenancy.md §2).
func (w ReminderWriter) findReminder(
	ctx context.Context, id shared.ID,
) (domain.Reminder, error) {
	reminder, err := w.Reminders.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.Reminder{}, shared.ErrNotFound.
				WithDetail("reminders.not_found").
				WithParams(map[string]string{"reminder_id": id.String()})
		}
		return domain.Reminder{}, err
	}
	return reminder, nil
}

// recordUpsert writes what an offline client has to be told about a reminder that arrived or
// changed as a whole.
//
// The payload is the shape the API answers with. There is no event to borrow it from - reminders
// have none (see ReminderWriter) - so this is the one description of the entity, which is also
// what keeps a client from having to know two.
func (w ReminderWriter) recordUpsert(
	ctx context.Context, reminder domain.Reminder, item domain.WorkItem,
	actor appshared.ActorContext, payload map[string]any,
) error {
	return w.Changes.Record(ctx, changelog.Change{
		TenantID: reminder.TenantID,
		Entity:   reminderTarget,
		EntityID: reminder.ID,
		Op:       changelog.Upsert,
		// The visibility filter a pull applies: a reminder is visible to a device subscribed to
		// the collection its entry is in, exactly as the entry itself is.
		ContainerID: item.CollectionID,
		ActorID:     actor.AccountID,
		HLC:         w.HLC.Next(),
		Payload:     payload,
	})
}

// recordAudit writes the evidence, inside the same transaction as the change (test AT-5).
//
// Values included, for the reason a due date's are: a reminder is a schedule and a list of
// accounts, not user content - no title, no notes travel here (rule 10, audit.md §4) - and "who
// was to be reminded, when" is not answerable without them.
func (w ReminderWriter) recordAudit(
	ctx context.Context, reminder domain.Reminder, actor appshared.ActorContext,
	action audit.Action, changes []audit.Change, now time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   reminder.TenantID,
		OccurredAt: now,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: reminderTarget,
		TargetID:   reminder.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

// reminderAuditChanges is what a creation and a deletion record: the whole reminder, because both
// are about the reminder as a whole.
func reminderAuditChanges(reminder domain.Reminder) []audit.Change {
	return []audit.Change{
		{Field: "item_id", Classification: audit.Open, To: reminder.ItemID.String()},
		{Field: "offset_spec", Classification: audit.Open, To: reminder.Offset.Spec},
		{
			Field: "channels", Classification: audit.Open,
			To: domain.ChannelList(reminder.Channels),
		},
		{
			Field: "recipients", Classification: audit.Open,
			To: domain.RecipientList(reminder.Recipients),
		},
		{Field: "fire_at", Classification: audit.Open, To: instantOrNil(reminder.FireAt)},
	}
}

// instantOrNil spells an optional moment for the audit trail: the instant, or nothing at all
// rather than an empty string that would read as a value.
func instantOrNil(at *time.Time) any {
	if at == nil {
		return nil
	}
	return at.UTC().Format(time.RFC3339Nano)
}

// scheduleReminderFire asks for the tenant's next wake-up, in the transaction that made something
// due.
//
// The same transaction on purpose: a reminder whose wake-up was never scheduled would wait for the
// next write to notice it, and nothing would say why. The dedupe key is the tenant, so a request
// that writes five reminders leaves one job - and a tenant whose wake-up is already waiting has it
// pulled forward rather than duplicated, which is what EnqueueJob's LEAST(run_at) is for.
//
// A moment that is zero is nothing to wake for: an offset with no due date to count from leaves
// the reminder standing without one, and there is nothing to schedule until a date comes back. A
// nil queue is a build without one, which the composition root does not produce and a test may.
func scheduleReminderFire(
	ctx context.Context, jobs queue.Queue, tenantID shared.ID, at time.Time,
) error {
	if jobs == nil || at.IsZero() {
		return nil
	}
	_, enqueued := jobs.Enqueue(ctx, queue.Request{
		Kind:      queue.KindReminderFire,
		TenantID:  tenantID,
		DedupeKey: tenantID.String(),
		RunAt:     at.UTC(),
	})
	return enqueued
}

// earliestMoment answers the first of the moments given, ignoring the ones that are not there. It
// is what a write hands to scheduleReminderFire: several reminders may have moved, and the wake-up
// belongs to the nearest of them.
func earliestMoment(moments ...*time.Time) time.Time {
	var earliest time.Time
	for _, moment := range moments {
		if moment == nil || moment.IsZero() {
			continue
		}
		if earliest.IsZero() || moment.Before(earliest) {
			earliest = *moment
		}
	}
	return earliest
}

// reminderOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema Reminder).
func reminderOutput(reminder domain.Reminder) usecase.Output {
	channels := make([]string, 0, len(reminder.Channels))
	for _, channel := range reminder.Channels {
		channels = append(channels, channel.String())
	}
	recipients := make([]string, 0, len(reminder.Recipients))
	for _, recipient := range reminder.Recipients {
		recipients = append(recipients, recipient.String())
	}

	return usecase.Output{
		"id":          reminder.ID.String(),
		"item_id":     reminder.ItemID.String(),
		"offset_spec": reminder.Offset.Spec,
		"channels":    channels,
		"recipients":  recipients,
		"state":       reminder.State.String(),
		"fire_at":     timeOrNil(reminder.FireAt),
		"created_at":  reminder.CreatedAt,
		"updated_at":  timeOrNil(reminder.UpdatedAt),
		"version":     reminder.Version,
	}
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h CreateReminder) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateReminderName,
		Summary: "Puts a reminder on an entry. offset_spec has two forms: REL: with an ISO-8601 " +
			"duration counted from the entry's due date - negative for before it, and the entry " +
			"needs a due date at all - or ABS: with an RFC 3339 instant no due date moves. Years " +
			"and months are refused in a duration, because they are calendar arithmetic rather " +
			"than a length of time. An empty recipient list means the assignee and the entry's " +
			"members, resolved when the reminder fires, so somebody added tomorrow is reached " +
			"tomorrow. A type without the REMINDER capability, a trashed or archived entry, and " +
			"a recipient who cannot see the entry are each refused by name.",
		SideEffects: "Writes the reminder, records the change for offline clients and writes an " +
			"audit entry.",
		TokenScope: remindersWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry to be reminded about.",
			},
			{
				Name: "offset_spec", Kind: usecase.KindString, Required: true,
				Description: "REL:-PT1H for an hour before the due date, or " +
					"ABS:2026-09-01T08:00:00Z for a fixed moment.",
			},
			{
				Name: "channels", Kind: usecase.KindList,
				Description: "What carries the reminder. Omitted means EMAIL, the one channel " +
					"this installation sends on.",
			},
			{
				Name: "recipients", Kind: usecase.KindIDList,
				Description: "Who is reminded. Omitted or empty means the assignee and the " +
					"entry's members.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ReminderCreatedAction, TargetType: reminderTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// invoke is the adapter between the catalogue's untyped input and the typed command, for all
// three channels.
func (h CreateReminder) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	recipients, err := in.IDList("recipients")
	if err != nil {
		return nil, err
	}
	channels, err := in.StringList("channels")
	if err != nil {
		return nil, err
	}

	reminder, err := h.Execute(ctx, actor, CreateReminderCommand{
		ItemID:     itemID,
		OffsetSpec: in.String("offset_spec"),
		Channels:   channels,
		Recipients: recipients,
	})
	if err != nil {
		return nil, err
	}
	return reminderOutput(reminder), nil
}

// ListReminders reads one entry's reminders, oldest first.
//
// The right to read the entry is the right to read what it reminds about, for the reason the
// discussion draws the same line: a separate right would be one more thing to get wrong for no
// protection gained (domain-model.md §3.2). Unpaged, because what one entry may carry is bounded
// where reminders are written.
type ListReminders struct {
	Reminders  repository.Reminders
	Items      repository.Items
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// ListRemindersQuery is the input, typed.
type ListRemindersQuery struct {
	ItemID shared.ID
}

// Execute returns the entry's reminders.
//
// Two transactions, and the permission question between them, for the reason ListComments splits
// there: a refusal writes an audit entry, and an entry written inside a read-only transaction
// cannot be written at all (audit.md §7).
func (h ListReminders) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListRemindersQuery,
) ([]domain.Reminder, error) {
	if query.ItemID.IsZero() {
		return nil, itemIDRequired()
	}

	subject, collection, err := readItemScope(
		ctx, h.UnitOfWork, h.Items, h.Containers, actor, query.ItemID)
	if err != nil {
		return nil, err
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(collection),
		Action:     RemindersReadAction,
		TokenScope: itemsRead,
		TargetType: itemTarget,
		TargetID:   query.ItemID,
		On:         reading(subject),
	}); err != nil {
		return nil, err
	}

	var reminders []domain.Reminder
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		reminders, err = h.Reminders.ListForItem(ctx, query.ItemID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return reminders, nil
}

// Descriptor is the catalogue entry.
func (h ListReminders) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListRemindersName,
		Summary: "Reads an entry's reminders, oldest first, all of them: what one entry may " +
			"carry is bounded, so there is no page to ask for. Readable by whoever may read the " +
			"entry.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry whose reminders are wanted.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RemindersReadAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListReminders) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}

	reminders, err := h.Execute(ctx, actor, ListRemindersQuery{ItemID: itemID})
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(reminders))
	for _, reminder := range reminders {
		rows = append(rows, reminderOutput(reminder))
	}
	return usecase.Output{"data": rows}, nil
}
