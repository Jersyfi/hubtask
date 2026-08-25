// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

var dueItem = shared.MustParseID("0192f000-0000-7000-8000-0000000000f5")

// dueDateProfiles is the shared fixture with DUE_DATE put back where the capability matrix grants
// it: every type carries a due date by default (domain-model.md §2). systemProfiles is
// deliberately narrower, which is exactly what the refusal test needs - the negative case is a
// tenant-narrowed profile rather than a type.
func dueDateProfiles() []domain.CapabilityProfile {
	rows := systemProfiles()
	for i, row := range rows {
		rows[i].Capabilities = append(row.Capabilities, domain.CapabilityDueDate)
	}
	return rows
}

type dueDateHarness struct {
	set        SetDueDate
	clear      ClearDueDate
	items      *items
	containers *containers
	events     *events
	changes    *changes
	audit      *sink
	history    *journal
	authorizer *authorizer
	uow        *unitOfWork
}

func newDueDateHarness(t *testing.T, rows []domain.CapabilityProfile) *dueDateHarness {
	t.Helper()

	h := &dueDateHarness{
		items:      &items{stored: map[shared.ID]domain.WorkItem{}},
		containers: &containers{stored: map[shared.ID]domain.Container{}},
		events:     &events{}, changes: &changes{}, audit: &sink{}, history: &journal{},
		authorizer: &authorizer{}, uow: &unitOfWork{},
	}

	writer := DueDateWriter{
		Items: h.items, Containers: h.containers,
		Profiles: &profiles{rows: rows}, Authorizer: h.authorizer,
		Events: h.events, Changes: h.changes, Audit: h.audit,
		Activity:   ActivityJournal{Entries: h.history, IDs: &ids{}},
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	h.set = SetDueDate{Due: writer}
	h.clear = ClearDueDate{Due: writer}

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

func (h *dueDateHarness) withItem(itemType domain.ItemType) domain.WorkItem {
	item := domain.WorkItem{
		ID: dueItem, TenantID: tenantID, CollectionID: collectionID, Type: itemType,
		Path: domain.RootPath(dueItem), Depth: 1, Title: "Buy oat milk",
		OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	h.items.stored[dueItem] = item
	return item
}

var berlinFriday = time.Date(2026, 9, 4, 15, 0, 0, 0, time.UTC)

func dueCmd(t *testing.T) DueDateCommand {
	t.Helper()
	due, err := domain.NewDueDate(&berlinFriday, false, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the fixture due date does not build: %v", err)
	}
	return DueDateCommand{ItemID: dueItem, Due: due}
}

// One change owes four things, and this is the test that says so.
func TestSettingADueDateWritesTheRowTheEventTheChangesAndTheEntry(t *testing.T) {
	h := newDueDateHarness(t, dueDateProfiles())
	h.withItem(domain.ItemTask)
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	item, err := h.set.Execute(ctx, actor(), dueCmd(t))
	if err != nil {
		t.Fatalf("setting the due date failed: %v", err)
	}

	if item.Due == nil || !item.Due.At.Equal(berlinFriday) || item.Due.TimeZone != "Europe/Berlin" {
		t.Fatalf("the entry carries %+v", item.Due)
	}
	if item.Version != 2 {
		t.Errorf("version %d, want the write to have spent one", item.Version)
	}

	t.Run("the row is written against the version that was read", func(t *testing.T) {
		if len(h.items.dueDates) != 1 {
			t.Fatalf("%d writes, want 1", len(h.items.dueDates))
		}
		if h.items.dueDates[0].expectedVersion != 1 {
			t.Errorf("written against version %d", h.items.dueDates[0].expectedVersion)
		}
	})

	t.Run("the event carries the new side and no old one", func(t *testing.T) {
		if len(h.events.appended) != 1 {
			t.Fatalf("%d events, want 1", len(h.events.appended))
		}
		announcement := h.events.appended[0]
		if announcement.Type != event.ItemDueChanged {
			t.Errorf("event type %s", announcement.Type)
		}
		if announcement.Payload["time_zone"] != "Europe/Berlin" {
			t.Errorf("the event names the zone as %v", announcement.Payload["time_zone"])
		}
		if _, was := announcement.Payload["old_due_at"]; was {
			t.Error("a set claims there was an old due date")
		}
		if _, snapshot := announcement.Payload["title"]; snapshot {
			t.Error("the event carries an entry snapshot")
		}
	})

	// The scalar rule of offline-sync.md §4.2: one entry per field that moved, each with its own
	// clock reading. The flag did not move - false before, false now - so no entry names it.
	t.Run("each moved field is its own change", func(t *testing.T) {
		if len(h.changes.recorded) != 2 {
			t.Fatalf("%d changes, want one for the date and one for the zone", len(h.changes.recorded))
		}
		payloads := map[string]string{}
		for _, change := range h.changes.recorded {
			if len(change.Payload) != 1 {
				t.Fatalf("a change carries %d fields: %+v", len(change.Payload), change.Payload)
			}
			if change.HLC.IsZero() {
				t.Error("a change carries no clock reading")
			}
			for field, value := range change.Payload {
				payloads[field] = value.(string)
			}
		}
		if payloads[domain.FieldDueAt] == "" || payloads[domain.FieldDueTimeZone] != "Europe/Berlin" {
			t.Errorf("the changes carry %+v", payloads)
		}
	})

	t.Run("the audit entry keeps both sides in clear text", func(t *testing.T) {
		if len(h.audit.entries) != 1 {
			t.Fatalf("%d audit entries, want 1", len(h.audit.entries))
		}
		entry := h.audit.entries[0]
		if entry.Action != ItemDueSetAction || entry.TargetID != dueItem {
			t.Errorf("unexpected entry: %+v", entry)
		}
		recorded, _ := entry.Changes[domain.FieldDueAt].(map[string]any)
		if recorded == nil || recorded["to"] == "" {
			t.Errorf("the new date is not in the trail: %+v", entry.Changes)
		}
	})

	t.Run("the history records the set with its values", func(t *testing.T) {
		if len(h.history.entries) != 1 {
			t.Fatalf("%d history entries, want 1", len(h.history.entries))
		}
		step := h.history.entries[0]
		if step.Verb != activity.ItemDueSet {
			t.Errorf("verb %s", step.Verb)
		}
		field, _ := step.ChangeSet[domain.FieldDueAt].(map[string]any)
		if field == nil || field["to"] == "" {
			t.Errorf("the step does not name the new date: %+v", step.ChangeSet)
		}
	})
}

// The acceptance's own case, at this level: the merge rule is per field, so moving the date alone
// records the date alone - a second device moving the zone concurrently keeps its zone.
func TestMovingTheDateAloneRecordsTheDateAlone(t *testing.T) {
	h := newDueDateHarness(t, dueDateProfiles())
	item := h.withItem(domain.ItemTask)
	item.Due = &domain.DueDate{At: berlinFriday.Add(-72 * time.Hour), TimeZone: "Europe/Berlin"}
	h.items.stored[dueItem] = item

	if _, err := h.set.Execute(context.Background(), actor(), dueCmd(t)); err != nil {
		t.Fatalf("moving the due date failed: %v", err)
	}

	if len(h.changes.recorded) != 1 {
		t.Fatalf("%d changes, want the moved date alone", len(h.changes.recorded))
	}
	if _, named := h.changes.recorded[0].Payload[domain.FieldDueAt]; !named {
		t.Errorf("the change carries %+v", h.changes.recorded[0].Payload)
	}

	announcement := h.events.appended[0]
	if _, was := announcement.Payload["old_due_at"]; !was {
		t.Error("a move does not name the date it moved from")
	}
	if h.history.entries[0].Verb != activity.ItemDueSet {
		t.Errorf("a move is recorded as %s", h.history.entries[0].Verb)
	}
}

// Clearing takes the trio off together, and every record says so: the event names the date that
// was there, the change log clears each stored field by name, and the history reads "removed".
func TestClearingWritesTheTrioWholeAndAnnouncesTheDateThatWas(t *testing.T) {
	h := newDueDateHarness(t, dueDateProfiles())
	item := h.withItem(domain.ItemTask)
	item.Due = &domain.DueDate{At: berlinFriday, DateOnly: true, TimeZone: "Europe/Berlin"}
	h.items.stored[dueItem] = item

	cleared, err := h.clear.Execute(context.Background(), actor(), DueDateCommand{ItemID: dueItem})
	if err != nil {
		t.Fatalf("clearing failed: %v", err)
	}
	if cleared.Due != nil {
		t.Fatalf("the entry still carries %+v", cleared.Due)
	}

	announcement := h.events.appended[0]
	if announcement.Type != event.ItemDueChanged {
		t.Errorf("event type %s", announcement.Type)
	}
	if _, is := announcement.Payload["new_due_at"]; is {
		t.Error("a clearing claims there is a new due date")
	}
	if _, was := announcement.Payload["old_due_at"]; !was {
		t.Error("a clearing does not name the date it removed")
	}

	// All three stored fields moved, and each clears by name: an absent field would read as "not
	// touched", and a device that read it that way would keep a due date somebody removed.
	if len(h.changes.recorded) != 3 {
		t.Fatalf("%d changes, want the three cleared fields", len(h.changes.recorded))
	}
	for _, change := range h.changes.recorded {
		for field, value := range change.Payload {
			if want := map[string]string{
				domain.FieldDueAt: "", domain.FieldDueTimeZone: "", domain.FieldDueDateOnly: "false",
			}[field]; value != want {
				t.Errorf("%s cleared to %v rather than %q", field, value, want)
			}
		}
	}

	if h.audit.entries[0].Action != ItemDueClearedAction {
		t.Errorf("audit action %s", h.audit.entries[0].Action)
	}
	if h.history.entries[0].Verb != activity.ItemDueCleared {
		t.Errorf("verb %s", h.history.entries[0].Verb)
	}
}

// Idempotence, in both directions: the state asked for is the state that is there, so nothing is
// written, no version is spent and nothing is announced.
func TestAskingForTheStateThatIsThereWritesNothing(t *testing.T) {
	h := newDueDateHarness(t, dueDateProfiles())
	item := h.withItem(domain.ItemTask)
	item.Due = dueCmd(t).Due
	h.items.stored[dueItem] = item

	repeated, err := h.set.Execute(context.Background(), actor(), dueCmd(t))
	if err != nil {
		t.Fatalf("the repeat failed: %v", err)
	}
	if repeated.Version != 1 {
		t.Errorf("the repeat spent a version: %d", repeated.Version)
	}

	bare := h.withItem(domain.ItemTask)
	if _, err := h.clear.Execute(context.Background(), actor(),
		DueDateCommand{ItemID: bare.ID}); err != nil {
		t.Fatalf("clearing nothing failed: %v", err)
	}

	if len(h.items.dueDates)+len(h.events.appended)+len(h.changes.recorded) != 0 {
		t.Errorf("a no-op wrote: %d writes, %d events, %d changes",
			len(h.items.dueDates), len(h.events.appended), len(h.changes.recorded))
	}
	if len(h.audit.entries)+len(h.history.entries) != 0 {
		t.Errorf("a no-op recorded: %d audit entries, %d history entries",
			len(h.audit.entries), len(h.history.entries))
	}
}

// The If-Match is honoured even when the change would be a no-op: the state the caller was
// reasoning about is not the state that is there.
func TestAStaleVersionIsRefusedEvenWhenTheDueDateWouldNotChange(t *testing.T) {
	h := newDueDateHarness(t, dueDateProfiles())
	item := h.withItem(domain.ItemTask)
	item.Due = dueCmd(t).Due
	item.Version = 4
	h.items.stored[dueItem] = item

	cmd := dueCmd(t)
	cmd.ExpectedVersion = 2

	_, err := h.set.Execute(context.Background(), actor(), cmd)
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale If-Match answered %v", err)
	}
}

// The capability matrix, narrowed: every system profile carries DUE_DATE, so the refusal case is
// a tenant that narrowed it away - and the answer names the capability rather than failing silently.
func TestADueDateOnANarrowedTypeIsRefused(t *testing.T) {
	h := newDueDateHarness(t, systemProfiles())
	h.withItem(domain.ItemTask)

	_, err := h.set.Execute(context.Background(), actor(), dueCmd(t))
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Fatalf("a narrowed profile answered %v", err)
	}
	if len(h.items.dueDates) != 0 {
		t.Error("the refusal still wrote the row")
	}

	// Clearing asks for the state the entry is already in, and succeeds.
	if _, err := h.clear.Execute(context.Background(), actor(),
		DueDateCommand{ItemID: dueItem}); err != nil {
		t.Errorf("clearing on the narrowed profile answered %v", err)
	}
}

// The permission question is the entry's: moving a deadline is writing an entry.
func TestSettingADueDateAsksForThePermissionToWriteItems(t *testing.T) {
	h := newDueDateHarness(t, dueDateProfiles())
	h.withItem(domain.ItemTask)

	if _, err := h.set.Execute(context.Background(), actor(), dueCmd(t)); err != nil {
		t.Fatalf("setting the due date failed: %v", err)
	}
	if len(h.authorizer.requests) != 1 {
		t.Fatalf("%d permission questions, want 1", len(h.authorizer.requests))
	}
	if h.authorizer.requests[0].Permission != service.PermissionWriteItems {
		t.Errorf("asked for %v", h.authorizer.requests[0].Permission)
	}

	h.authorizer.err = shared.ErrForbidden
	if _, err := h.set.Execute(context.Background(), actor(), dueCmd(t)); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a refusal answered %v", err)
	}
}

// The three channels reach the same command: the untyped input parses into the same trio the
// typed command carries, and the refusals come back with the field the client sent.
func TestTheDueDateChannelsReachTheSameCommand(t *testing.T) {
	h := newDueDateHarness(t, dueDateProfiles())
	h.withItem(domain.ItemTask)

	out, err := h.set.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{
		"item_id":       dueItem.String(),
		"due_at":        "2026-09-04T17:00:00+02:00",
		"due_date_only": false,
		"due_time_zone": "Europe/Berlin",
	})
	if err != nil {
		t.Fatalf("the untyped door failed: %v", err)
	}
	if out.String("due_time_zone") != "Europe/Berlin" {
		t.Errorf("the output names the zone as %v", out["due_time_zone"])
	}
	if at, is := out["due_at"].(time.Time); !is || !at.Equal(berlinFriday) {
		t.Errorf("the output carries %v, want %v", out["due_at"], berlinFriday)
	}

	t.Run("an unparseable instant is refused by name", func(t *testing.T) {
		_, err := h.set.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{
			"item_id": dueItem.String(), "due_at": "tomorrow",
		})
		refusal := shared.AsError(err)
		if refusal == nil || refusal.DetailCode != "items.due_at_malformed" {
			t.Fatalf("an unparseable instant answered %v", err)
		}
		if len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/due_at" {
			t.Errorf("the refusal does not point at /due_at: %v", refusal.Fields)
		}
	})

	t.Run("an invalid zone is refused by name", func(t *testing.T) {
		_, err := h.set.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{
			"item_id": dueItem.String(), "due_at": "2026-09-04T17:00:00Z", "due_time_zone": "CEST",
		})
		refusal := shared.AsError(err)
		if refusal == nil || refusal.DetailCode != "items.due_time_zone_invalid" {
			t.Fatalf("an invalid zone answered %v", err)
		}
	})

	t.Run("clearing needs nothing beyond the entry", func(t *testing.T) {
		if _, err := h.clear.Descriptor().Handler.Invoke(context.Background(), actor(),
			usecase.Input{"item_id": dueItem.String()}); err != nil {
			t.Fatalf("the untyped clearing failed: %v", err)
		}
	})
}
