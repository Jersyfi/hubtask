// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/application/repository/outbox"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// ReminderNotifier is the slice of the Notification context this pass needs: somebody is to be
// told about an entry, now.
//
// A slice rather than the package, for the reason the notification consumer declares slices of the
// work repositories: what the firing pass can do to the notification context is one line - it can
// ask for a record to be written - and it cannot read, suppress or send anything.
type ReminderNotifier interface {
	Execute(ctx context.Context, tenantID, itemID shared.ID, recipients []shared.ID) error
}

// ReminderSignals is the slice of the metrics adapter this pass reports through
// (observability-reliability.md §3.2, SLO-5).
//
// One measurement, and it is the one the service level objective is written about: how late a
// reminder was against the moment it promised. The channel is the label because that is where the
// difference will be once there is more than one - an email that waits for a mail server is a
// different story from a push that does not.
type ReminderSignals interface {
	ReminderFired(ctx context.Context, channel string, delaySeconds float64)
}

// FireReminders turns the reminders whose moment has come into notifications (D-03).
//
// Internal, and deliberately absent from the catalogue in domain-model.md §5, for the reason
// ReconcileMedia is absent from it (C-06): the catalogue is what a person, an agent or a rule can
// ask for, and "fire everybody's reminders now" is not something anybody should be able to ask
// for - the way to influence when a reminder fires is the reminder.
//
// It runs inside the transaction the job runner opened, which is the whole reliability argument:
// the guarded transition, the notification records, the delivery jobs and the job's own completion
// commit together. A process killed halfway leaves none of them and the reminders are still
// pending when the job is claimed again - nothing is lost and nothing is doubled (test RT-3).
type FireReminders struct {
	Reminders repository.Reminders
	Items     repository.Items
	// Schedule is the scan the two announcements run on: the same repository as Items, under the
	// narrow interface that says what this pass may do with it.
	Schedule    repository.DueAnnouncements
	Containers  repository.Containers
	ItemMembers repository.ItemMembers
	// Visibility answers whether a named recipient can still see the entry. A membership revoked
	// since the reminder was written is not survived by a reminder that remembers better days -
	// the same question D-02 asked at the write, asked again at the moment it matters.
	Visibility Visibility
	Notifier   ReminderNotifier
	// Events is where the two scheduling announcements go: item.due_soon and item.overdue are
	// facts about a deadline rather than messages to a person, and what reacts to them is
	// automation (D-03, domain-model.md §4).
	Events outbox.Events
	// Changes is how a device learns that a reminder fired. offline-sync.md §8 requires a client
	// to reconcile the local notification it scheduled, and it can only do that if the server's
	// decision reaches it - so the state the pass writes travels like any other field.
	Changes changelog.ChangeLog
	Clock   clock.Clock
	IDs     clock.IDGenerator
	HLC     clock.HLCSource
	Signals ReminderSignals
	// BatchSize bounds one pass. A pass is one transaction, and a transaction that fired ten
	// thousand reminders would hold its locks for as long as that took.
	BatchSize int
}

// ReminderOutcome is what one pass did and what it leaves behind.
type ReminderOutcome struct {
	// Fired counts the reminders that became notifications, Cancelled the ones whose entry no
	// longer warrants one, and Announced the deadlines the pass told automation about.
	Fired     int
	Cancelled int
	Announced int
	// NextAt is when the tenant next owes a reminder, and nil when it owes none - which is what
	// lets the job finish instead of idling forever.
	NextAt *time.Time
}

// FilledBatch reports whether the pass ran into its own bound, in which case there is known work
// left and the job comes straight back for it rather than sleeping until the next moment.
func (o ReminderOutcome) FilledBatch(batch int) bool {
	if batch <= 0 {
		return false
	}
	return o.Fired+o.Cancelled >= batch || o.Announced >= batch
}

// DefaultReminderBatch is how many reminders one pass settles. Large enough that a normal tenant
// is done in one, small enough that the transaction stays short.
const DefaultReminderBatch = 100

// Execute fires what is due for the tenant the actor names.
func (h FireReminders) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (ReminderOutcome, error) {
	if actor.TenantID.IsZero() {
		return ReminderOutcome{}, shared.ErrInternal.WithDetail("reminders.fire_without_tenant")
	}

	now := h.Clock.Now()
	due, err := h.Reminders.ClaimDue(ctx, now, h.batch())
	if err != nil {
		return ReminderOutcome{}, err
	}

	outcome := ReminderOutcome{}
	for _, reminder := range due {
		fired, err := h.settle(ctx, actor, reminder, now)
		if err != nil {
			return ReminderOutcome{}, err
		}
		switch {
		case fired:
			outcome.Fired++
		default:
			outcome.Cancelled++
		}
	}

	announced, err := h.announce(ctx, now)
	if err != nil {
		return ReminderOutcome{}, err
	}
	outcome.Announced = announced

	// Read after everything the pass wrote, so that what it answers is what is left rather than
	// what was there when the pass began. A full batch leaves the rest of it in this answer, and
	// the job comes straight back for it.
	nextReminder, err := h.Reminders.NextMoment(ctx)
	if err != nil {
		return ReminderOutcome{}, err
	}
	nextAnnouncement, err := h.Schedule.NextDueAnnouncement(ctx, domain.DueSoonLead)
	if err != nil {
		return ReminderOutcome{}, err
	}
	if moment := earliestMoment(nextReminder, nextAnnouncement); !moment.IsZero() {
		outcome.NextAt = &moment
	}
	return outcome, nil
}

// announce tells automation what the clock has just made true: a deadline that has come within the
// lead, and one that has passed with the work not done.
//
// Both are claimed and stamped in one statement, which is what makes each of them happen once per
// due date (D-03): a second pass finds nothing left to claim. A date that moves clears the stamps
// where it is written, because a new deadline may be approached and missed again.
//
// The order matters for an entry whose deadline is already past: due_soon is claimed first, so a
// deadline that arrived while nothing was running is announced as approaching and then as missed,
// rather than only as missed. That is the sequence a rule reads, and it is the same one it would
// have read had the scheduler never been away.
func (h FireReminders) announce(ctx context.Context, now time.Time) (int, error) {
	soon, err := h.Schedule.ClaimDueSoon(ctx, now, now.Add(domain.DueSoonLead), h.batch())
	if err != nil {
		return 0, err
	}
	for _, claimed := range soon {
		if err := h.publish(ctx, claimed, domain.DueSoonThresholdSpec, now); err != nil {
			return 0, err
		}
	}

	overdue, err := h.Schedule.ClaimOverdue(ctx, now, h.batch())
	if err != nil {
		return 0, err
	}
	for _, claimed := range overdue {
		if err := h.publish(ctx, claimed, "", now); err != nil {
			return 0, err
		}
	}
	return len(soon) + len(overdue), nil
}

// publish writes one announcement to the outbox. The threshold decides which of the two it is: the
// lead for the approach, nothing for the deadline itself.
func (h FireReminders) publish(
	ctx context.Context, claimed repository.DueAnnouncement, thresholdSpec string, now time.Time,
) error {
	// What the event needs and nothing more. The scan reads four fields precisely so that a pass
	// announcing a deadline never carries a title or a note (rule 10).
	item := domain.WorkItem{
		ID: claimed.ItemID, TenantID: claimed.TenantID, CollectionID: claimed.CollectionID,
		Due: &claimed.Due,
	}

	announcement, err := event.NewItemOverdue(h.IDs.NewID(), item, now, event.Cause{})
	if thresholdSpec != "" {
		announcement, err = event.NewItemDueSoon(
			h.IDs.NewID(), item, thresholdSpec, now, event.Cause{})
	}
	if err != nil {
		return err
	}
	return h.Events.Append(ctx, announcement)
}

// settle decides what becomes of one reminder and writes it, reporting whether it fired.
//
// The transition is guarded and comes first: a reminder leaves PENDING exactly once, so a second
// pass - another leader, a retried job - finds nothing to move and writes no second notification.
// Everything else in here happens only for the pass that won it.
func (h FireReminders) settle(
	ctx context.Context, actor appshared.ActorContext, reminder domain.Reminder, now time.Time,
) (bool, error) {
	item, err := h.Items.Find(ctx, reminder.ItemID)
	switch {
	case errors.Is(err, shared.ErrNotFound):
		// The entry went between the claim and here. Nothing to remind about, and the row goes
		// with the entry anyway (the foreign key cascades) - so there is nothing to write either.
		return false, nil
	case err != nil:
		return false, err
	}

	if !remindable(item) {
		// Completed, trashed or archived: nobody wants to be reminded of it, and offline-sync.md
		// §8 says so from the client's side - a device that reminds about a task completed long
		// ago is the failure being avoided. CANCELLED is the state that exists for exactly this,
		// and the row goes on saying why nothing was sent.
		moved, err := h.Reminders.Settle(ctx, reminder.ID, domain.ReminderCancelled)
		if err != nil || !moved {
			return false, err
		}
		return false, h.recordState(ctx, reminder, item, domain.ReminderCancelled)
	}

	moved, err := h.Reminders.Settle(ctx, reminder.ID, domain.ReminderSent)
	if err != nil || !moved {
		return false, err
	}

	recipients, err := h.recipients(ctx, actor, item, reminder)
	if err != nil {
		return false, err
	}
	if err := h.Notifier.Execute(ctx, reminder.TenantID, item.ID, recipients); err != nil {
		return false, err
	}
	if err := h.recordState(ctx, reminder, item, domain.ReminderSent); err != nil {
		return false, err
	}
	h.report(ctx, reminder, now)
	return true, nil
}

// recordState tells offline clients what the server decided about a reminder.
//
// One entry carrying the one field that moved, exactly as an edit records one per field
// (offline-sync.md §4.2). The actor is nobody: a state written by the clock is not somebody's
// change, and a device filtering its own writes out of a pull must not filter this one away.
func (h FireReminders) recordState(
	ctx context.Context, reminder domain.Reminder, item domain.WorkItem, state domain.ReminderState,
) error {
	if h.Changes == nil {
		return nil
	}
	return h.Changes.Record(ctx, changelog.Change{
		TenantID:    reminder.TenantID,
		Entity:      reminderTarget,
		EntityID:    reminder.ID,
		Op:          changelog.Upsert,
		ContainerID: item.CollectionID,
		HLC:         h.HLC.Next(),
		Payload:     map[string]any{"state": state.String()},
	})
}

// recipients is who is told: the accounts the reminder names, or - for the empty list - the
// entry's assignee and its members.
//
// Resolved here rather than when the reminder was written, which is the acceptance's own sentence:
// an empty list stays empty in the row, so somebody added to the entry tomorrow is reached
// tomorrow. A named recipient who can no longer see the entry is dropped: the message carries the
// entry's title, and a revoked membership must not be survived by a reminder.
func (h FireReminders) recipients(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem,
	reminder domain.Reminder,
) ([]shared.ID, error) {
	if len(reminder.Recipients) == 0 {
		return h.everybodyOn(ctx, item)
	}

	collection, err := findCollection(ctx, h.Containers, item.CollectionID)
	if err != nil {
		return nil, err
	}

	reached := make([]shared.ID, 0, len(reminder.Recipients))
	for _, recipient := range reminder.Recipients {
		permitted, err := h.Visibility.CanSee(ctx, actor, recipient, containerPath(collection))
		if err != nil {
			return nil, err
		}
		if permitted {
			reached = append(reached, recipient)
		}
	}
	return reached, nil
}

// everybodyOn is what an empty recipient list means: the entry's assignee and its member list,
// read now rather than when the reminder was written.
//
// Not its watchers, for the reason the comment notification does not use them (C-09): the schema
// reserves the set name and nothing writes it yet. No authorisation question either, and for the
// same reason - every recipient here is derived from a membership of the entry itself, which is
// the narrowing C-04 built.
func (h FireReminders) everybodyOn(
	ctx context.Context, item domain.WorkItem,
) ([]shared.ID, error) {
	members, err := h.ItemMembers.List(ctx, item.ID)
	if err != nil {
		return nil, err
	}

	seen := make(map[shared.ID]bool, len(members)+1)
	recipients := make([]shared.ID, 0, len(members)+1)
	for _, candidate := range append([]shared.ID{item.AssigneeID}, members...) {
		if candidate.IsZero() || seen[candidate] {
			continue
		}
		seen[candidate] = true
		recipients = append(recipients, candidate)
	}
	return recipients, nil
}

// report records how late the reminder was against the moment it promised. That number is SLO-5.
func (h FireReminders) report(ctx context.Context, reminder domain.Reminder, now time.Time) {
	if h.Signals == nil || reminder.FireAt == nil {
		return
	}

	delay := now.Sub(*reminder.FireAt).Seconds()
	if delay < 0 {
		// A pass that ran a moment early - a clock that stepped back, a wake-up rounded down - is
		// not negatively late. Reported as nothing rather than as a negative sample, which would
		// pull a percentile towards a punctuality nobody achieved.
		delay = 0
	}
	for _, channel := range reminder.Channels {
		h.Signals.ReminderFired(ctx, channel.String(), delay)
	}
}

// remindable reports whether an entry still warrants a reminder: not completed, not trashed, not
// archived (I-W4).
func remindable(item domain.WorkItem) bool {
	return !item.Completion.IsCompleted && !item.IsArchived() && !item.IsTrashed()
}

func (h FireReminders) batch() int {
	if h.BatchSize <= 0 {
		return DefaultReminderBatch
	}
	return h.BatchSize
}
