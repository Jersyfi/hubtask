// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
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
	SetDueDateName   = "SetDueDate"
	ClearDueDateName = "ClearDueDate"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	ItemDueSetAction     audit.Action = "item.due_set"
	ItemDueClearedAction audit.Action = "item.due_cleared"
)

// DueDateWriter is what both directions of the due date need.
//
// One dependency set held by both use cases, for the reason AssignmentWriter is one: the two
// operations are the same walk in opposite directions - the same guards, the same four records -
// and the only thing that differs is whether a date arrives or leaves. It is also the writer the
// create and update paths dispatch into (D-01): the contract has carried the three due fields on
// their schemas since 0.1.0, and serving them through a second implementation would be a second
// answer to what a due date means.
type DueDateWriter struct {
	Items      repository.Items
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	// Reminders is here because a due date that moves takes the relative reminders with it: the
	// reminder lives on the same entry, so its moment is recomputed in the same transaction rather
	// than by a job that would leave the two disagreeing until it ran (D-02).
	Reminders repository.Reminders
	// Jobs is where the tenant's next wake-up is asked for, in the transaction that moved the
	// date: a reminder that came forward with it needs the schedule to come forward too (D-03).
	Jobs       queue.Queue
	Authorizer Authorizer
	Events     outbox.Events
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	Activity   ActivityJournal
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// SetDueDate puts a due date on an entry: the instant, the all-day flag and the IANA zone,
// written together because the three describe one date (i18n-l10n.md §4).
//
// Moving a due date is this use case again: the trio is a scalar family, so it is one decision,
// one version and one step of the history.
type SetDueDate struct {
	Due DueDateWriter
}

// ClearDueDate takes the due date off an entry - the instant, the flag and the zone together,
// because none of them means anything alone.
type ClearDueDate struct {
	Due DueDateWriter
}

// DueDateCommand is the input of both directions, typed. The due date is nil for a clearing,
// which is the one field the two do not share.
type DueDateCommand struct {
	ItemID shared.ID
	Due    *domain.DueDate
	// ExpectedVersion is the version the caller read, from If-Match. Zero means the caller read
	// none and accepts whatever is there (api-guidelines.md §5).
	ExpectedVersion int
}

// Execute puts the due date on the entry and returns it.
func (h SetDueDate) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd DueDateCommand,
) (domain.WorkItem, error) {
	return h.Due.change(ctx, actor, cmd)
}

// Execute takes the due date off the entry and returns it.
func (h ClearDueDate) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd DueDateCommand,
) (domain.WorkItem, error) {
	cmd.Due = nil
	return h.Due.change(ctx, actor, cmd)
}

// dueDirection reads which way the change went off the target state, so that the trail cannot
// record a clearing while the row holds a date. Which direction a call *is* falls out of the
// command: the pair's own routes say it by name, and the merge patch says it by the null.
func dueDirection(due *domain.DueDate) (audit.Action, activity.Verb) {
	if due == nil {
		return ItemDueClearedAction, activity.ItemDueCleared
	}
	return ItemDueSetAction, activity.ItemDueSet
}

// change is the whole of both use cases.
func (w DueDateWriter) change(
	ctx context.Context, actor appshared.ActorContext, cmd DueDateCommand,
) (domain.WorkItem, error) {
	if cmd.ItemID.IsZero() {
		return domain.WorkItem{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}

	// The entry and its collection are read before the permission question, because the answer
	// depends on the path: a membership held at the hub applies downwards (domain-model.md §3.2).
	// Nothing read here is trusted afterwards - the state that decides the write is read again
	// inside the transaction.
	subject, collection, err := readItemScope(ctx, w.UnitOfWork, w.Items, w.Containers, actor, cmd.ItemID)
	if err != nil {
		return domain.WorkItem{}, err
	}

	action, _ := dueDirection(cmd.Due)

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     action,
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		TargetID:   cmd.ItemID,
		On:         changing(subject),
	}); err != nil {
		return domain.WorkItem{}, err
	}

	var changed domain.WorkItem
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		item, err := findItem(ctx, w.Items, cmd.ItemID)
		if err != nil {
			return err
		}
		collection, err := findCollection(ctx, w.Containers, item.CollectionID)
		if err != nil {
			return err
		}
		// I-C3: an archived or trashed collection is read-only, and its entries inherit that.
		if err := collection.EnsureAcceptsItems(); err != nil {
			return err
		}
		profile, err := profileOf(ctx, w.Profiles, item.Type)
		if err != nil {
			return err
		}

		wanted, changes, err := item.WithDueDate(cmd.Due, profile, now)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			// Already in the state asked for. Nothing is written, no version is spent and nothing
			// is announced - which is what makes a repeat harmless rather than merely accepted.
			//
			// The If-Match is still honoured: a caller that wrote against a version somebody else
			// has moved on is told so, even when its own change would have been a no-op, because
			// the state it was reasoning about is not the state that is there.
			if err := ensureExpectedVersion(item, cmd.ExpectedVersion); err != nil {
				return err
			}
			changed = item
			return nil
		}

		written, err := w.write(ctx, actor, item, wanted, changes, cmd.ExpectedVersion, profile, now)
		changed = written
		return err
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return changed, nil
}

// write stores the trio and records what the change owes: the event outwards, the change log for
// offline clients, the audit entry, and the step of the entry's own history - all inside the
// caller's transaction (test AT-5). It is the one implementation every door shares: the pair's
// routes, the update's merge patch, and the create's declared fields all end here.
func (w DueDateWriter) write(
	ctx context.Context, actor appshared.ActorContext, before, after domain.WorkItem,
	changes []domain.FieldChange, expectedVersion int, profile domain.CapabilityProfile,
	now time.Time,
) (domain.WorkItem, error) {
	expected := expectedVersion
	if expected == 0 {
		// The caller read no version and accepts whatever is there. Not the same as skipping the
		// check: the version in hand is still the one the update matches on, so a concurrent
		// write between the read and here is still caught.
		expected = before.Version
	}
	if err := w.Items.SetDueDate(ctx, after, expected); err != nil {
		return domain.WorkItem{}, err
	}
	after.Version = expected + 1

	// Built from the stored state rather than from the command, so that what the event says and
	// what the row holds cannot disagree.
	announcement, err := event.NewItemDueChanged(w.IDs.NewID(), after, before.Due,
		event.Actor{Kind: actor.Kind, ID: actor.AccountID}, now, event.Cause{})
	if err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.Events.Append(ctx, announcement); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.rescheduleReminders(ctx, after); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.recordChanges(ctx, after, actor, changes); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.recordAudit(ctx, after, actor, changes, now); err != nil {
		return domain.WorkItem{}, err
	}
	if err := w.recordActivity(ctx, after, actor, changes, profile, now); err != nil {
		return domain.WorkItem{}, err
	}
	return after, nil
}

// rescheduleReminders moves every relative reminder of the entry to the moment the new due date
// implies, in the transaction that moved the date.
//
// In the same transaction deliberately: the reminder lives on the same entry, and a job doing this
// afterwards would leave a window in which the row says one thing and the schedule another - which
// is exactly the window a reminder would fire in. Absolute reminders and reminders that have
// already fired are not touched, which the domain decides (Reminder.Rescheduled); a due date that
// was cleared leaves the relative ones without a moment rather than deleting them, because nobody
// asked for the reminder to go.
//
// Nothing is recorded for it: fire_at is derived from the offset and the entry's date, so no
// client merges it and no auditor reads it as a separate act - the audit entry for the due date
// is the record of what happened here.
func (w DueDateWriter) rescheduleReminders(ctx context.Context, item domain.WorkItem) error {
	pending, err := w.Reminders.ListPendingForItem(ctx, item.ID)
	if err != nil {
		return err
	}

	var earliest time.Time
	for _, reminder := range pending {
		rescheduled, moved, err := reminder.Rescheduled(item.Due)
		if err != nil {
			return err
		}
		if !moved {
			continue
		}
		if err := w.Reminders.Reschedule(ctx, rescheduled); err != nil {
			return err
		}
		earliest = earliestMoment(&earliest, rescheduled.FireAt)
	}

	// The wake-up follows the reminders, in the same transaction: a date pulled forward moves its
	// reminders with it, and a schedule still pointing at the old moment would fire them late
	// (D-03). A date pushed back needs nothing - a wake-up that is too early finds nothing due and
	// reschedules itself.
	return scheduleReminderFire(ctx, w.Jobs, item.TenantID, earliest)
}

// recordChanges writes what an offline client has to be told: one entry per field of the trio
// that moved, each with its own HLC.
//
// That is the scalar rule of offline-sync.md §4.2 written down, and the acceptance's own case:
// two devices moving the date and the zone independently converge to both, which one entry
// carrying the pair would destroy - the later HLC deciding the whole payload and silently
// discarding the other device's field. A cleared field travels as the empty string rather than
// being left out: an absent field means "not touched", and a device that read it that way would
// keep a due date somebody removed.
func (w DueDateWriter) recordChanges(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	changes []domain.FieldChange,
) error {
	for _, change := range changes {
		err := w.Changes.Record(ctx, changelog.Change{
			TenantID:    item.TenantID,
			Entity:      itemTarget,
			EntityID:    item.ID,
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

// recordAudit writes the evidence, values included: a due date is a schedule rather than user
// content - no title, no notes - and "what was the deadline moved to, by whom, and when" is not
// answerable without both sides (audit.md §4, rule 10).
func (w DueDateWriter) recordAudit(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	changes []domain.FieldChange, now time.Time,
) error {
	recorded := make([]audit.Change, 0, len(changes)+2)
	for _, change := range changes {
		recorded = append(recorded, audit.Change{
			Field: change.Field, Classification: audit.Open,
			From: change.From, To: change.To,
		})
	}
	recorded = append(recorded,
		audit.Change{Field: "type", Classification: audit.Open, To: string(item.Type)},
		audit.Change{
			Field: "collection_id", Classification: audit.Open,
			To: item.CollectionID.String(),
		})

	action, _ := dueDirection(item.Due)
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   item.TenantID,
		OccurredAt: now,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: itemTarget,
		TargetID:   item.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(recorded...),
	})
}

// recordActivity writes the step of the entry's own history, both sides included: "the deadline
// moved from Tuesday to Friday" is what a person opening the history is looking for, and an
// entry showing only the new date would leave them unable to see that it moved at all.
func (w DueDateWriter) recordActivity(
	ctx context.Context, item domain.WorkItem, actor appshared.ActorContext,
	changes []domain.FieldChange, profile domain.CapabilityProfile, now time.Time,
) error {
	_, verb := dueDirection(item.Due)
	changeSet := activity.ChangeSet(historyForm(profile), historyFields(changes)...)
	return w.Activity.record(ctx, actor, item, verb, changeSet, now)
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h SetDueDate) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetDueDateName,
		Summary: "Puts a due date on an entry, or moves it: the instant, the all-day flag and the " +
			"IANA time zone, written together because the three describe one date. An all-day due " +
			"date is a date in that zone, never a midnight that shifts with the viewer. A zone " +
			"that is not an IANA name, a zone without a date and a flag without a date are each " +
			"refused by name. Idempotent: setting the due date an entry already has succeeds and " +
			"announces nothing.",
		SideEffects: "Writes the due trio, announces " + string(event.ItemDueChanged) +
			" with both sides of the move, records one change per field for offline clients, " +
			"writes an audit entry and a step of the entry's history.",
		TokenScope: itemsWrite,
		Input: append(
			[]usecase.Field{
				{
					Name: "due_at", Kind: usecase.KindString, Required: true,
					Description: "When the entry is due, RFC 3339. Stored in UTC.",
				},
				{
					Name: "due_date_only", Kind: usecase.KindBool,
					Description: "True for an all-day due date: due_at is read as a date in " +
						"due_time_zone, never as an instant.",
				},
				{
					Name: "due_time_zone", Kind: usecase.KindString,
					Description: "The IANA time zone the date is local to, such as Europe/Berlin. " +
						"Omitted for a due date that is a plain instant.",
				},
			},
			dueDateInput("The entry to put the due date on.")...,
		),
		Audit: usecase.AuditDeclaration{
			Action: ItemDueSetAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemDueSet},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h ClearDueDate) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ClearDueDateName,
		Summary: "Takes the due date off an entry - the instant, the all-day flag and the zone " +
			"together, because none of them means anything alone. Idempotent: an entry with no " +
			"due date succeeds and announces nothing.",
		SideEffects: "Clears the due trio, announces " + string(event.ItemDueChanged) +
			" naming the date that was there, records one change per field for offline clients, " +
			"writes an audit entry and a step of the entry's history.",
		TokenScope: itemsWrite,
		Input:      dueDateInput("The entry to take the due date off."),
		Audit: usecase.AuditDeclaration{
			Action: ItemDueClearedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Verb: activity.ItemDueCleared},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// dueDateInput is what both directions take beyond the date. One list, so that a client which
// learned one of them from /meta/capabilities does not find the other spelled differently.
func dueDateInput(itemDescription string) []usecase.Field {
	return []usecase.Field{
		{Name: "item_id", Kind: usecase.KindID, Required: true, Description: itemDescription},
		{
			Name: "expected_version", Kind: usecase.KindInt,
			Description: "The version last read, from the If-Match header over REST. Omitted means " +
				"the caller read none and accepts whatever is there; a version that has moved on " +
				"since is refused rather than overwritten.",
		},
	}
}

func (h SetDueDate) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := dueDateCommand(in)
	if err != nil {
		return nil, err
	}
	cmd.Due, err = dueDateOf(in)
	if err != nil {
		return nil, err
	}

	item, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}

func (h ClearDueDate) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := dueDateCommand(in)
	if err != nil {
		return nil, err
	}

	item, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return itemOutput(item), nil
}

// dueDateCommand is the adapter between the catalogue's untyped input and the typed command, for
// both directions and all three channels.
func dueDateCommand(in usecase.Input) (DueDateCommand, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return DueDateCommand{}, err
	}
	return DueDateCommand{ItemID: itemID, ExpectedVersion: in.Int("expected_version")}, nil
}

// dueDateOf builds the trio out of the three input fields. The parse happens here because the
// catalogue's vocabulary has no timestamp kind: every channel hands the instant over as a string,
// and an unparseable one is refused with the field error a client can act on.
func dueDateOf(in usecase.Input) (*domain.DueDate, error) {
	raw := in.String("due_at")
	if raw == "" {
		// The field is declared required, so the catalogue has already refused an absent one;
		// this is the merge-patch door, where the trio's other members may arrive alone and the
		// domain decides what a qualifier without its date means.
		return domain.NewDueDate(nil, in.Bool("due_date_only"), in.String("due_time_zone"))
	}
	at, err := parseInstantField(raw, "due_at")
	if err != nil {
		return nil, err
	}
	return domain.NewDueDate(&at, in.Bool("due_date_only"), in.String("due_time_zone"))
}

// parseInstantField reads the one spelling the contract declares, RFC 3339, and refuses anything
// else with the field the client sent - the same shape for the due date and the start.
func parseInstantField(raw, field string) (time.Time, error) {
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		code := "items." + field + "_malformed"
		return time.Time{}, shared.ErrValidation.
			WithDetail(code).
			WithParams(map[string]string{"value": raw}).
			WithFields(shared.FieldError{Path: "/" + field, Code: code})
	}
	return at, nil
}
