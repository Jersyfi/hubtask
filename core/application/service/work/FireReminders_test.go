// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// notifier is the Notification context as the firing pass reaches it: one call, recorded.
type notifier struct {
	told []reminderMessage
	err  error
}

type reminderMessage struct {
	tenantID   shared.ID
	itemID     shared.ID
	recipients []shared.ID
}

func (n *notifier) Execute(
	_ context.Context, tenantID, itemID shared.ID, recipients []shared.ID,
) error {
	if n.err != nil {
		return n.err
	}
	n.told = append(n.told, reminderMessage{
		tenantID: tenantID, itemID: itemID, recipients: recipients,
	})
	return nil
}

// reminderSignals records the one measurement SLO-5 is written about.
type reminderSignals struct {
	delays   []float64
	channels []string
}

func (s *reminderSignals) ReminderFired(_ context.Context, channel string, delaySeconds float64) {
	s.channels = append(s.channels, channel)
	s.delays = append(s.delays, delaySeconds)
}

// announcements is the scheduling scan as a pass sees it: what a deadline has made true, claimed
// and stamped once. The fake stamps by removing the entry from its own list, which is the same
// exactly-once the statement gets from claiming and stamping in one go.
type announcements struct {
	soon    []repository.DueAnnouncement
	overdue []repository.DueAnnouncement
	next    *time.Time
}

func (a *announcements) ClaimDueSoon(
	_ context.Context, _, threshold time.Time, limit int,
) ([]repository.DueAnnouncement, error) {
	claimed := a.take(&a.soon, threshold, limit)
	return claimed, nil
}

func (a *announcements) ClaimOverdue(
	_ context.Context, now time.Time, limit int,
) ([]repository.DueAnnouncement, error) {
	claimed := a.take(&a.overdue, now, limit)
	return claimed, nil
}

func (a *announcements) take(
	from *[]repository.DueAnnouncement, boundary time.Time, limit int,
) []repository.DueAnnouncement {
	var claimed, left []repository.DueAnnouncement
	for _, candidate := range *from {
		if len(claimed) < limit && !candidate.Due.At.After(boundary) {
			claimed = append(claimed, candidate)
			continue
		}
		left = append(left, candidate)
	}
	*from = left
	return claimed
}

func (a *announcements) NextDueAnnouncement(
	_ context.Context, _ time.Duration,
) (*time.Time, error) {
	return a.next, nil
}

type firingHarness struct {
	firing     FireReminders
	reminders  *reminders
	items      *items
	containers *containers
	members    *itemMembers
	visibility *visibility
	notifier   *notifier
	signals    *reminderSignals
	schedule   *announcements
	events     *events
	changes    *changes
}

// firedAt is the moment the pass runs at: an hour after the reminders below promised, so the
// delay it reports is a number a test can name.
var firedAt = now.Add(time.Hour)

func newFiringHarness(t *testing.T) *firingHarness {
	t.Helper()

	h := &firingHarness{
		reminders:  newReminders(),
		items:      &items{stored: map[shared.ID]domain.WorkItem{}},
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		members:    newItemMembers(),
		visibility: newVisibility(accountID, otherAccount),
		notifier:   &notifier{},
		signals:    &reminderSignals{},
		schedule:   &announcements{},
		events:     &events{},
		changes:    &changes{},
	}
	h.firing = FireReminders{
		Reminders: h.reminders, Items: h.items, Schedule: h.schedule, Containers: h.containers,
		ItemMembers: h.members, Visibility: h.visibility, Notifier: h.notifier,
		Events: h.events, Changes: h.changes,
		Clock: clock.Fixed(firedAt), IDs: &ids{}, HLC: &hlcSource{},
		Signals: h.signals, BatchSize: 2,
	}

	h.containers.stored[hubID] = domain.Container{
		ID: hubID, TenantID: tenantID, Type: domain.ContainerHub, Name: "Private",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	return h
}

func (h *firingHarness) withItem() domain.WorkItem {
	item := domain.WorkItem{
		ID: remindedItem, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemTask,
		Path: domain.RootPath(remindedItem), Depth: 1, Title: "Buy oat milk",
		OrderKey: "a0", AssigneeID: accountID, CreatedBy: accountID, CreatedAt: now,
		UpdatedAt: now, Version: 1,
	}
	h.items.stored[remindedItem] = item
	return item
}

// withDueReminder seeds one reminder whose moment has passed.
func (h *firingHarness) withDueReminder(id shared.ID, recipients ...shared.ID) domain.Reminder {
	moment := now
	reminder := domain.Reminder{
		ID: id, TenantID: tenantID, ItemID: remindedItem,
		Offset:   domain.ReminderOffset{Spec: "ABS:" + now.Format(time.RFC3339), Instant: now},
		Channels: []domain.ReminderChannel{domain.ReminderChannelEmail},
		State:    domain.ReminderPending, FireAt: &moment, CreatedAt: now, Version: 1,
	}
	reminder.Recipients = recipients
	h.reminders.stored[id] = reminder
	return reminder
}

func systemActor() appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantID}
}

// One pass owes three things: the transition, the record, and the number SLO-5 is written about.
func TestFiringSettlesTheReminderTellsSomebodyAndReportsTheDelay(t *testing.T) {
	h := newFiringHarness(t)
	h.withItem()
	reminder := h.withDueReminder(reminderID)

	outcome, err := h.firing.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	if outcome.Fired != 1 || outcome.Cancelled != 0 {
		t.Errorf("the pass reported %+v", outcome)
	}
	if state := h.reminders.stored[reminder.ID].State; state != domain.ReminderSent {
		t.Errorf("the reminder is %s rather than sent", state)
	}
	if len(h.notifier.told) != 1 {
		t.Fatalf("%d people were told", len(h.notifier.told))
	}
	told := h.notifier.told[0]
	if told.tenantID != tenantID || told.itemID != remindedItem {
		t.Errorf("the message is about %+v", told)
	}
	if len(h.signals.delays) != 1 || h.signals.delays[0] != time.Hour.Seconds() {
		t.Errorf("the delay reported is %v", h.signals.delays)
	}
	// And the device that scheduled a local notification for this reminder learns that the server
	// fired it (offline-sync.md §8): one entry, the one field that moved, caused by nobody.
	if len(h.changes.recorded) != 1 {
		t.Fatalf("the change entries are %+v", h.changes.recorded)
	}
	recorded := h.changes.recorded[0]
	if recorded.Entity != "reminder" || recorded.Payload["state"] != "SENT" {
		t.Errorf("the change entry is %+v", recorded)
	}
	if !recorded.ActorID.IsZero() {
		t.Errorf("the firing was recorded as %s's change", recorded.ActorID)
	}
	if h.signals.channels[0] != "EMAIL" {
		t.Errorf("the channel reported is %q", h.signals.channels[0])
	}
	// Nothing is left, so the job may finish: the next write brings it back.
	if outcome.NextAt != nil {
		t.Errorf("the tenant still owes %v", outcome.NextAt)
	}
}

// The guarded transition is the whole of the exactly-once argument: a second pass over the same
// reminder - another leader, a retried job - moves nothing and tells nobody.
func TestASecondPassFiresNothingTwice(t *testing.T) {
	h := newFiringHarness(t)
	h.withItem()
	h.withDueReminder(reminderID)

	if _, err := h.firing.Execute(t.Context(), systemActor()); err != nil {
		t.Fatalf("the first pass failed: %v", err)
	}
	outcome, err := h.firing.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the second pass failed: %v", err)
	}

	if outcome.Fired != 0 {
		t.Errorf("the second pass fired %d", outcome.Fired)
	}
	if len(h.notifier.told) != 1 {
		t.Errorf("%d messages were written for one reminder", len(h.notifier.told))
	}
}

// I-W4 at the moment it matters: nobody is reminded of work that is done, deleted or put away, and
// the row goes on saying why nothing was sent.
func TestAReminderOnAnEntryNobodyWantsIsCancelled(t *testing.T) {
	for name, prepare := range map[string]func(item *domain.WorkItem){
		"completed": func(item *domain.WorkItem) {
			completed := now
			item.Completion = domain.Completion{IsCompleted: true, CompletedAt: &completed}
		},
		"trashed":  func(item *domain.WorkItem) { trashed := now; item.DeletedAt = &trashed },
		"archived": func(item *domain.WorkItem) { archived := now; item.ArchivedAt = &archived },
	} {
		t.Run(name, func(t *testing.T) {
			h := newFiringHarness(t)
			item := h.withItem()
			prepare(&item)
			h.items.stored[remindedItem] = item
			h.withDueReminder(reminderID)

			outcome, err := h.firing.Execute(t.Context(), systemActor())
			if err != nil {
				t.Fatalf("the pass failed: %v", err)
			}
			if outcome.Fired != 0 || outcome.Cancelled != 1 {
				t.Errorf("the pass reported %+v", outcome)
			}
			if state := h.reminders.stored[reminderID].State; state != domain.ReminderCancelled {
				t.Errorf("the reminder is %s rather than cancelled", state)
			}
			if len(h.notifier.told) != 0 {
				t.Error("somebody was reminded of work nobody wants a reminder about")
			}
			if len(h.signals.delays) != 0 {
				t.Error("a cancelled reminder was counted against the punctuality objective")
			}
			if len(h.changes.recorded) != 1 ||
				h.changes.recorded[0].Payload["state"] != "CANCELLED" {
				t.Errorf("the change entries are %+v", h.changes.recorded)
			}
		})
	}
}

// The acceptance's own sentence: an empty list is resolved when the reminder fires, so somebody
// added to the entry tomorrow is reached tomorrow.
func TestAnEmptyRecipientListMeansTheAssigneeAndTheMembersNow(t *testing.T) {
	h := newFiringHarness(t)
	h.withItem()
	h.members.carried[remindedItem] = []shared.ID{otherAccount, accountID}
	h.withDueReminder(reminderID)

	if _, err := h.firing.Execute(t.Context(), systemActor()); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	told := h.notifier.told[0].recipients
	if len(told) != 2 || told[0] != accountID || told[1] != otherAccount {
		t.Errorf("the recipients are %v, want the assignee first and then the member", told)
	}
}

// A membership revoked since the reminder was written is not survived by it: the message carries
// the entry's title, and somebody who can no longer open the entry does not get it.
func TestANamedRecipientWhoLostAccessIsDropped(t *testing.T) {
	h := newFiringHarness(t)
	h.withItem()
	stranger := shared.MustParseID("0192f000-0000-7000-8000-0000000000fb")
	h.withDueReminder(reminderID, otherAccount, stranger)

	if _, err := h.firing.Execute(t.Context(), systemActor()); err != nil {
		t.Fatalf("the pass failed: %v", err)
	}

	told := h.notifier.told[0].recipients
	if len(told) != 1 || told[0] != otherAccount {
		t.Errorf("the recipients are %v", told)
	}
}

// The pass is bounded, and what it leaves behind is what brings it straight back.
func TestAFullBatchLeavesTheRestForTheNextPass(t *testing.T) {
	h := newFiringHarness(t)
	h.withItem()
	h.withDueReminder("0192f000-0000-7000-8000-000000000101")
	h.withDueReminder("0192f000-0000-7000-8000-000000000102")
	h.withDueReminder("0192f000-0000-7000-8000-000000000103")

	outcome, err := h.firing.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if outcome.Fired != 2 {
		t.Fatalf("the pass fired %d, want the batch", outcome.Fired)
	}
	if outcome.NextAt == nil {
		t.Fatal("the pass left nothing behind, though a reminder is still due")
	}

	rest, err := h.firing.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the second pass failed: %v", err)
	}
	if rest.Fired != 1 || rest.NextAt != nil {
		t.Errorf("the second pass reported %+v", rest)
	}
}

// A relative reminder whose entry lost its due date has no moment, and a pass never claims it: the
// reminder waits for a date to come back rather than firing at once.
func TestAReminderWithoutAMomentIsNeverDue(t *testing.T) {
	h := newFiringHarness(t)
	h.withItem()
	reminder := h.withDueReminder(reminderID)
	reminder.FireAt = nil
	h.reminders.stored[reminder.ID] = reminder

	outcome, err := h.firing.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if outcome.Fired != 0 || outcome.NextAt != nil {
		t.Errorf("the pass reported %+v", outcome)
	}
	if state := h.reminders.stored[reminder.ID].State; state != domain.ReminderPending {
		t.Errorf("the reminder is %s rather than still pending", state)
	}
}

// A pass without a tenant is a programming error with a name, not an empty pass: every read it
// makes is made for the tenant the job names.
func TestAPassWithoutATenantIsRefused(t *testing.T) {
	h := newFiringHarness(t)

	_, err := h.firing.Execute(t.Context(), appshared.ActorContext{Kind: appshared.ActorSystem})
	if refusal := shared.AsError(err); refusal == nil ||
		refusal.DetailCode != "reminders.fire_without_tenant" {
		t.Fatalf("refused as %v", err)
	}
}

// The two announcements the same pass makes, and what tells them apart: the threshold. Nobody
// caused either, so the actor on both is the system.
func TestThePassAnnouncesWhatTheClockHasMadeTrue(t *testing.T) {
	h := newFiringHarness(t)
	h.withItem()

	approaching := repository.DueAnnouncement{
		ItemID: remindedItem, TenantID: tenantID, CollectionID: collectionID,
		Due: domain.DueDate{At: firedAt.Add(6 * time.Hour), TimeZone: "Europe/Berlin"},
	}
	missed := repository.DueAnnouncement{
		ItemID: shared.MustParseID("0192f000-0000-7000-8000-0000000000fc"), TenantID: tenantID, CollectionID: collectionID,
		Due: domain.DueDate{At: firedAt.Add(-2 * time.Hour)},
	}
	h.schedule.soon = []repository.DueAnnouncement{approaching}
	h.schedule.overdue = []repository.DueAnnouncement{missed}

	outcome, err := h.firing.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if outcome.Announced != 2 {
		t.Fatalf("the pass announced %d", outcome.Announced)
	}
	if len(h.events.appended) != 2 {
		t.Fatalf("the events are %+v", h.events.appended)
	}

	soon := h.events.appended[0]
	if soon.Type != event.ItemDueSoon {
		t.Errorf("the first announcement is %s", soon.Type)
	}
	if soon.Payload["threshold_spec"] != domain.DueSoonThresholdSpec {
		t.Errorf("the threshold is %v", soon.Payload["threshold_spec"])
	}
	if soon.Payload["item_id"] != remindedItem.String() ||
		soon.Payload["collection_id"] != collectionID.String() {
		t.Errorf("the announcement is about %+v", soon.Payload)
	}
	if soon.Actor.Kind != shared.ActorSystem {
		t.Errorf("the announcement was caused by %v", soon.Actor)
	}
	// Rule 10: what the pass carries is a deadline, never a title or a note.
	if _, carried := soon.Payload["title"]; carried {
		t.Error("the announcement carries the entry's title")
	}

	overdue := h.events.appended[1]
	if overdue.Type != event.ItemOverdue || overdue.Payload["threshold_spec"] != "PT0S" {
		t.Errorf("the second announcement is %s with %v",
			overdue.Type, overdue.Payload["threshold_spec"])
	}
}

// Claimed once means announced once: the second pass finds nothing left, which is what keeps a
// rule from escalating the same overdue entry forever.
func TestADeadlineIsAnnouncedOnce(t *testing.T) {
	h := newFiringHarness(t)
	h.withItem()
	h.schedule.overdue = []repository.DueAnnouncement{{
		ItemID: remindedItem, TenantID: tenantID, CollectionID: collectionID,
		Due: domain.DueDate{At: firedAt.Add(-time.Hour)},
	}}

	if _, err := h.firing.Execute(t.Context(), systemActor()); err != nil {
		t.Fatalf("the first pass failed: %v", err)
	}
	outcome, err := h.firing.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the second pass failed: %v", err)
	}

	if outcome.Announced != 0 || len(h.events.appended) != 1 {
		t.Errorf("the deadline was announced %d times", len(h.events.appended))
	}
}

// What the pass leaves behind is the earliest of the two clocks it watches: the next reminder and
// the next announcement. Whichever comes first is when the job comes back.
func TestTheNextMomentIsTheEarliestOfBothSchedules(t *testing.T) {
	h := newFiringHarness(t)
	h.withItem()
	announcement := firedAt.Add(30 * time.Minute)
	h.schedule.next = &announcement

	reminder := h.withDueReminder(reminderID)
	later := firedAt.Add(2 * time.Hour)
	reminder.FireAt = &later
	h.reminders.stored[reminder.ID] = reminder

	outcome, err := h.firing.Execute(t.Context(), systemActor())
	if err != nil {
		t.Fatalf("the pass failed: %v", err)
	}
	if outcome.NextAt == nil || !outcome.NextAt.Equal(announcement) {
		t.Errorf("the pass comes back at %v rather than %v", outcome.NextAt, announcement)
	}
}
