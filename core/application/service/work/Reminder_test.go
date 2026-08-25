// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	remindedItem = shared.MustParseID("0192f000-0000-7000-8000-0000000000f7")
	reminderID   = shared.MustParseID("0192f000-0000-7000-8000-0000000000f8")
	otherAccount = shared.MustParseID("0192f000-0000-7000-8000-0000000000f9")
)

// reminders is the in-memory fake of the reminder store. Like the others, it records what it was
// given, because what the use case owes is not only a return value.
type reminders struct {
	stored   map[shared.ID]domain.Reminder
	inserted []domain.Reminder
	// updates and reschedules record the two writes separately, because the point of their being
	// two is that one spends a version and the other does not.
	updates     []reminderWrite
	reschedules []domain.Reminder
	deleted     []reminderWrite
}

type reminderWrite struct {
	reminder        domain.Reminder
	expectedVersion int
}

func newReminders() *reminders {
	return &reminders{stored: map[shared.ID]domain.Reminder{}}
}

func (r *reminders) Find(_ context.Context, id shared.ID) (domain.Reminder, error) {
	reminder, found := r.stored[id]
	if !found {
		return domain.Reminder{}, shared.ErrNotFound
	}
	return reminder, nil
}

func (r *reminders) ListForItem(_ context.Context, itemID shared.ID) ([]domain.Reminder, error) {
	return r.filter(itemID, false), nil
}

func (r *reminders) ListPendingForItem(
	_ context.Context, itemID shared.ID,
) ([]domain.Reminder, error) {
	return r.filter(itemID, true), nil
}

func (r *reminders) filter(itemID shared.ID, pendingOnly bool) []domain.Reminder {
	var found []domain.Reminder
	for _, reminder := range r.stored {
		if reminder.ItemID != itemID {
			continue
		}
		if pendingOnly && reminder.State != domain.ReminderPending {
			continue
		}
		found = append(found, reminder)
	}
	return found
}

func (r *reminders) CountForItem(_ context.Context, itemID shared.ID) (int, error) {
	return len(r.filter(itemID, false)), nil
}

func (r *reminders) Insert(_ context.Context, reminder domain.Reminder) error {
	r.inserted = append(r.inserted, reminder)
	r.stored[reminder.ID] = reminder
	return nil
}

func (r *reminders) Update(
	_ context.Context, reminder domain.Reminder, expectedVersion int,
) error {
	stored, found := r.stored[reminder.ID]
	if !found || stored.Version != expectedVersion || stored.State != domain.ReminderPending {
		return shared.ErrVersionConflict.WithDetail("reminders.version_conflict")
	}
	r.updates = append(r.updates, reminderWrite{reminder: reminder, expectedVersion: expectedVersion})
	written := reminder
	written.Version = expectedVersion + 1
	r.stored[reminder.ID] = written
	return nil
}

func (r *reminders) Reschedule(_ context.Context, reminder domain.Reminder) error {
	r.reschedules = append(r.reschedules, reminder)
	stored, found := r.stored[reminder.ID]
	if !found {
		return nil
	}
	stored.FireAt = reminder.FireAt
	r.stored[reminder.ID] = stored
	return nil
}

func (r *reminders) Delete(_ context.Context, id shared.ID, expectedVersion int) error {
	stored, found := r.stored[id]
	if !found || stored.Version != expectedVersion {
		return shared.ErrVersionConflict.WithDetail("reminders.version_conflict")
	}
	r.deleted = append(r.deleted, reminderWrite{reminder: stored, expectedVersion: expectedVersion})
	delete(r.stored, id)
	return nil
}

// reminderProfiles is the fixture with REMINDER where the capability matrix grants it: every type
// carries reminders (domain-model.md §2). systemProfiles is deliberately narrower, which is what
// the refusal test needs - the negative case is a tenant-narrowed profile rather than a type.
func reminderProfiles() []domain.CapabilityProfile {
	rows := systemProfiles()
	for i, row := range rows {
		rows[i].Capabilities = append(row.Capabilities, domain.CapabilityReminder)
	}
	return rows
}

type reminderHarness struct {
	create     CreateReminder
	update     UpdateReminder
	remove     DeleteReminder
	list       ListReminders
	reminders  *reminders
	items      *items
	containers *containers
	changes    *changes
	audit      *sink
	visibility *visibility
	authorizer *authorizer
}

func newReminderHarness(t *testing.T, rows []domain.CapabilityProfile) *reminderHarness {
	t.Helper()

	h := &reminderHarness{
		reminders:  newReminders(),
		items:      &items{stored: map[shared.ID]domain.WorkItem{}},
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		changes:    &changes{}, audit: &sink{},
		visibility: newVisibility(accountID, otherAccount),
		authorizer: &authorizer{},
	}

	writer := ReminderWriter{
		Reminders: h.reminders, Items: h.items, Containers: h.containers,
		Profiles: &profiles{rows: rows}, Authorizer: h.authorizer, Visibility: h.visibility,
		Changes: h.changes, Audit: h.audit,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	h.create = CreateReminder{Writer: writer}
	h.update = UpdateReminder{Writer: writer}
	h.remove = DeleteReminder{Writer: writer}
	h.list = ListReminders{
		Reminders: h.reminders, Items: h.items, Containers: h.containers,
		Authorizer: h.authorizer, UnitOfWork: &unitOfWork{},
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

// withItem seeds the entry, with a due date unless told otherwise - a relative reminder needs one.
func (h *reminderHarness) withItem(due *domain.DueDate) domain.WorkItem {
	item := domain.WorkItem{
		ID: remindedItem, TenantID: tenantID, CollectionID: collectionID, Type: domain.ItemTask,
		Path: domain.RootPath(remindedItem), Depth: 1, Title: "Buy oat milk",
		OrderKey: "a0", Due: due, CreatedBy: accountID, CreatedAt: now, UpdatedAt: now,
		Version: 1,
	}
	h.items.stored[remindedItem] = item
	return item
}

func (h *reminderHarness) withReminder(spec string, due *domain.DueDate) domain.Reminder {
	reminder, err := domain.NewReminder(domain.NewReminderInput{
		ID: reminderID, TenantID: tenantID, ItemID: remindedItem, OffsetSpec: spec,
		Due: due, Now: now,
	})
	if err != nil {
		panic(err)
	}
	h.reminders.stored[reminder.ID] = reminder
	return reminder
}

func remindedDue(t *testing.T) *domain.DueDate {
	t.Helper()

	due, err := domain.NewDueDate(&berlinFriday, false, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the fixture due date does not build: %v", err)
	}
	return due
}

// One creation owes two things - the row and the two records - and no more: there is no reminder
// event in the catalogue and no step of the entry's history, both deliberately.
func TestCreatingAReminderWritesTheRowTheChangeAndTheEntry(t *testing.T) {
	h := newReminderHarness(t, reminderProfiles())
	due := remindedDue(t)
	h.withItem(due)

	reminder, err := h.create.Execute(t.Context(), actor(), CreateReminderCommand{
		ItemID: remindedItem, OffsetSpec: "REL:-PT1H", Recipients: []shared.ID{otherAccount},
	})
	if err != nil {
		t.Fatalf("creating the reminder failed: %v", err)
	}

	if reminder.Offset.Spec != "REL:-PT1H" || reminder.State != domain.ReminderPending ||
		reminder.Version != 1 {
		t.Errorf("created %+v", reminder)
	}
	if reminder.FireAt == nil || !reminder.FireAt.Equal(berlinFriday.Add(-time.Hour)) {
		t.Errorf("it fires at %v rather than an hour before the date", reminder.FireAt)
	}
	if len(h.reminders.inserted) != 1 {
		t.Fatalf("rows written: %d", len(h.reminders.inserted))
	}

	if len(h.changes.recorded) != 1 {
		t.Fatalf("changes recorded: %d", len(h.changes.recorded))
	}
	change := h.changes.recorded[0]
	if change.Entity != "reminder" || change.EntityID != reminder.ID ||
		change.ContainerID != collectionID {
		t.Errorf("the change entry is %+v", change)
	}
	if change.Payload["offset_spec"] != "REL:-PT1H" {
		t.Errorf("the change payload is %+v", change.Payload)
	}

	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ReminderCreatedAction {
		t.Fatalf("unexpected audit entries: %+v", h.audit.entries)
	}
	if h.audit.entries[0].TargetType != reminderTarget {
		t.Errorf("the audit entry targets %q", h.audit.entries[0].TargetType)
	}
}

// The reachability question an assignment asks, asked about every named recipient (C-01): somebody
// who cannot see the entry cannot be reminded of it.
func TestCreatingAReminderAsksWhetherEveryRecipientCanSeeTheEntry(t *testing.T) {
	h := newReminderHarness(t, reminderProfiles())
	h.withItem(remindedDue(t))
	stranger := shared.MustParseID("0192f000-0000-7000-8000-0000000000fa")

	_, err := h.create.Execute(t.Context(), actor(), CreateReminderCommand{
		ItemID: remindedItem, OffsetSpec: "REL:-PT1H",
		Recipients: []shared.ID{otherAccount, stranger},
	})
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "reminders.recipient_without_access" {
		t.Fatalf("refused as %v, want reminders.recipient_without_access", err)
	}
	if len(h.reminders.inserted) != 0 {
		t.Error("the reminder was written despite the refusal")
	}

	asked := map[shared.ID]bool{}
	for _, account := range h.visibility.asked {
		asked[account] = true
	}
	if !asked[otherAccount] || !asked[stranger] {
		t.Errorf("the visibility question was asked about %v", h.visibility.asked)
	}
}

// The bound the unpaged list rests on. The shelf is seeded full rather than filled through the use
// case, because what is under test is the refusal, not the twenty-five writes before it.
func TestAnEntryTakesNoMoreRemindersThanTheBound(t *testing.T) {
	h := newReminderHarness(t, reminderProfiles())
	h.withItem(nil)

	for i := 0; i < domain.MaxRemindersPerItem; i++ {
		id := shared.MustParseID(fmt.Sprintf("0192f000-0000-7000-8000-0000000001%02x", i))
		h.reminders.stored[id] = domain.Reminder{
			ID: id, TenantID: tenantID, ItemID: remindedItem, State: domain.ReminderPending,
			CreatedAt: now, Version: 1,
		}
	}

	_, err := h.create.Execute(t.Context(), actor(), CreateReminderCommand{
		ItemID: remindedItem, OffsetSpec: "ABS:2026-09-01T08:00:00Z",
	})
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "reminders.too_many" {
		t.Fatalf("refused as %v, want reminders.too_many", err)
	}
	if len(h.reminders.inserted) != 0 {
		t.Error("the reminder past the bound was written")
	}
}

// I-W4 and the capability, both refused where the write happens rather than only in the domain.
func TestAReminderNeedsARemindableEntryInAWritableState(t *testing.T) {
	t.Run("a type the tenant narrowed", func(t *testing.T) {
		h := newReminderHarness(t, systemProfiles())
		h.withItem(nil)

		_, err := h.create.Execute(t.Context(), actor(), CreateReminderCommand{
			ItemID: remindedItem, OffsetSpec: "ABS:2026-09-01T08:00:00Z",
		})
		if got := shared.AsError(err).DetailCode; got != "items.capability_not_supported" {
			t.Fatalf("refused as %q", got)
		}
	})

	t.Run("a trashed entry", func(t *testing.T) {
		h := newReminderHarness(t, reminderProfiles())
		item := h.withItem(nil)
		trashed := now
		item.DeletedAt = &trashed
		h.items.stored[remindedItem] = item

		_, err := h.create.Execute(t.Context(), actor(), CreateReminderCommand{
			ItemID: remindedItem, OffsetSpec: "ABS:2026-09-01T08:00:00Z",
		})
		if err == nil {
			t.Fatal("a trashed entry took a reminder")
		}
	})
}

// An update records one change per field that moved - the merge rule - and takes the optimistic
// lock the caller read.
func TestUpdatingAReminderRecordsOneChangePerField(t *testing.T) {
	h := newReminderHarness(t, reminderProfiles())
	due := remindedDue(t)
	h.withItem(due)
	stored := h.withReminder("REL:-PT1H", due)

	spec := "REL:-PT2H"
	recipients := []shared.ID{otherAccount}
	changed, err := h.update.Execute(t.Context(), actor(), ChangeReminderCommand{
		ItemID: remindedItem, ReminderID: stored.ID, ExpectedVersion: 1,
		Patch: domain.ReminderPatch{OffsetSpec: &spec, Recipients: &recipients},
	})
	if err != nil {
		t.Fatalf("the update failed: %v", err)
	}

	if changed.Offset.Spec != "REL:-PT2H" || changed.Version != 2 {
		t.Errorf("the reminder is %+v", changed)
	}
	if !changed.FireAt.Equal(berlinFriday.Add(-2 * time.Hour)) {
		t.Errorf("the moment is %v rather than two hours before the date", changed.FireAt)
	}
	if len(h.reminders.updates) != 1 || h.reminders.updates[0].expectedVersion != 1 {
		t.Fatalf("the writes are %+v", h.reminders.updates)
	}

	if len(h.changes.recorded) != 2 {
		t.Fatalf("changes recorded: %d, want one per field", len(h.changes.recorded))
	}
	fields := map[string]bool{}
	for _, change := range h.changes.recorded {
		if len(change.Payload) != 1 {
			t.Errorf("a change entry carries %d fields", len(change.Payload))
		}
		for field := range change.Payload {
			fields[field] = true
		}
	}
	if !fields["offset_spec"] || !fields["recipients"] {
		t.Errorf("the fields recorded are %v", fields)
	}
	// fire_at is derived: a client that merged it would be merging the server's arithmetic.
	if fields["fire_at"] {
		t.Error("the derived moment travels as a merge field")
	}
}

// An update that says what is already stored writes nothing - and still honours the If-Match, so a
// caller reasoning about a version that has moved on is told so.
func TestAnUpdateThatChangesNoReminderFieldWritesNothing(t *testing.T) {
	h := newReminderHarness(t, reminderProfiles())
	due := remindedDue(t)
	h.withItem(due)
	stored := h.withReminder("REL:-PT1H", due)

	same := "REL:-PT1H"
	if _, err := h.update.Execute(t.Context(), actor(), ChangeReminderCommand{
		ItemID: remindedItem, ReminderID: stored.ID,
		Patch: domain.ReminderPatch{OffsetSpec: &same},
	}); err != nil {
		t.Fatalf("the update failed: %v", err)
	}
	if len(h.reminders.updates) != 0 || len(h.changes.recorded) != 0 {
		t.Error("a no-op update wrote something")
	}

	_, err := h.update.Execute(t.Context(), actor(), ChangeReminderCommand{
		ItemID: remindedItem, ReminderID: stored.ID, ExpectedVersion: 7,
		Patch: domain.ReminderPatch{OffsetSpec: &same},
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale If-Match answered %v", err)
	}
}

// A reminder reached through the wrong entry is refused by name: the route says which entry it
// belongs to, and answering for another would let access to one entry act on another's reminder.
func TestAReminderIsOnlyReachableThroughItsOwnEntry(t *testing.T) {
	h := newReminderHarness(t, reminderProfiles())
	h.withItem(nil)
	stored := h.withReminder("ABS:2026-09-01T08:00:00Z", nil)

	err := h.remove.Execute(t.Context(), actor(), ChangeReminderCommand{
		ItemID: collectionID, ReminderID: stored.ID,
	})
	if refusal := shared.AsError(err); refusal == nil ||
		refusal.DetailCode != "reminders.not_on_item" {
		t.Fatalf("refused as %v, want reminders.not_on_item", err)
	}
}

// A deletion removes the row and tells offline clients so, which is where the tombstone lives.
func TestDeletingAReminderRemovesTheRowAndRecordsTheDeletion(t *testing.T) {
	h := newReminderHarness(t, reminderProfiles())
	h.withItem(nil)
	stored := h.withReminder("ABS:2026-09-01T08:00:00Z", nil)

	if err := h.remove.Execute(t.Context(), actor(), ChangeReminderCommand{
		ItemID: remindedItem, ReminderID: stored.ID, ExpectedVersion: 1,
	}); err != nil {
		t.Fatalf("the deletion failed: %v", err)
	}

	if len(h.reminders.deleted) != 1 || h.reminders.deleted[0].expectedVersion != 1 {
		t.Fatalf("the deletions are %+v", h.reminders.deleted)
	}
	if len(h.changes.recorded) != 1 || h.changes.recorded[0].Op != "DELETE" {
		t.Fatalf("the change entry is %+v", h.changes.recorded)
	}
	if h.changes.recorded[0].Payload != nil {
		t.Errorf("the deletion carries a payload: %v", h.changes.recorded[0].Payload)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ReminderDeletedAction {
		t.Fatalf("unexpected audit entries: %+v", h.audit.entries)
	}

	err := h.remove.Execute(t.Context(), actor(), ChangeReminderCommand{
		ItemID: remindedItem, ReminderID: stored.ID,
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("deleting it twice answered %v", err)
	}
}

// The list is the entry's own and reads through the same right the entry does.
func TestTheListAnswersTheEntrysReminders(t *testing.T) {
	h := newReminderHarness(t, reminderProfiles())
	h.withItem(nil)
	h.withReminder("ABS:2026-09-01T08:00:00Z", nil)

	out, err := h.list.Descriptor().Handler.Invoke(
		t.Context(), actor(), usecase.Input{"item_id": remindedItem.String()})
	if err != nil {
		t.Fatalf("listing failed: %v", err)
	}

	rows, ok := out["data"].([]usecase.Output)
	if !ok || len(rows) != 1 {
		t.Fatalf("the answer is %+v", out)
	}
	if rows[0]["offset_spec"] != "ABS:2026-09-01T08:00:00Z" || rows[0]["state"] != "PENDING" {
		t.Errorf("the row is %+v", rows[0])
	}
}

// The two channel sets are one set. The domain models may not import one another (ADR-0001), so
// this is where the copy is held to its original - and where a channel added to one and forgotten
// in the other is caught.
func TestTheReminderChannelsAreTheNotificationChannels(t *testing.T) {
	carried := domain.ReminderChannels()
	sent := notification.Channels()

	if len(carried) != len(sent) {
		t.Fatalf("a reminder carries %v while a notification is sent on %v", carried, sent)
	}
	for i, channel := range carried {
		if string(channel) != string(sent[i]) {
			t.Errorf("channel %d is %q here and %q there", i, channel, sent[i])
		}
	}
}
