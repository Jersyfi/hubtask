// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The dispatch D-01 decided: the contract has carried the due fields on the create and update
// schemas since 0.1.0, so both doors serve them - into the writer the SetDueDate pair owns, so a
// due date has one validation, one event and one history whichever way it arrives.

func TestACreateDeclaringTheScheduleDispatchesIntoTheDueWriter(t *testing.T) {
	h := newItemHarness()
	h.profiles.rows = dueDateProfiles()

	out, err := h.handler.Descriptor().Handler.Invoke(context.Background(), itemActor(),
		map[string]any{
			"type": "TASK", "title": "Buy milk",
			"collection_id": collectionID.String(),
			"start_at":      "2026-08-30T08:00:00Z",
			"due_at":        "2026-09-04T17:00:00+02:00",
			"due_time_zone": "Europe/Berlin",
		})
	if err != nil {
		t.Fatalf("the create failed: %v", err)
	}

	if at, is := out["start_at"].(time.Time); !is ||
		!at.Equal(time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("the start came back as %v", out["start_at"])
	}
	if at, is := out["due_at"].(time.Time); !is ||
		!at.Equal(time.Date(2026, 9, 4, 15, 0, 0, 0, time.UTC)) {
		t.Errorf("the due date came back as %v", out["due_at"])
	}
	if out.String("due_time_zone") != "Europe/Berlin" {
		t.Errorf("the zone came back as %v", out["due_time_zone"])
	}
	// The insert and the due write: born at 1, the due date spends the second.
	if out.Int("version") != 2 {
		t.Errorf("version %d, want 2", out.Int("version"))
	}

	// Two announcements: the entry exists, and it is due - the scheduler subscribes to the
	// second, not to every create.
	kinds := map[event.Type]int{}
	for _, announcement := range h.events.appended {
		kinds[announcement.Type]++
	}
	if kinds[event.ItemCreated] != 1 || kinds[event.ItemDueChanged] != 1 {
		t.Errorf("the create announced %v", kinds)
	}
}

func TestACreateWithAQualifierAndNoDateIsRefusedWhole(t *testing.T) {
	h := newItemHarness()
	h.profiles.rows = dueDateProfiles()

	_, err := h.handler.Descriptor().Handler.Invoke(context.Background(), itemActor(),
		map[string]any{
			"type": "TASK", "title": "Buy milk",
			"collection_id": collectionID.String(),
			"due_time_zone": "Europe/Berlin",
		})
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "items.due_time_zone_without_date" {
		t.Fatalf("a qualifier without its date answered %v", err)
	}
	if len(h.items.stored) != 0 {
		t.Error("the refusal still created the entry")
	}
}

func TestAPatchMovingTheDueDateDispatchesIntoTheWriter(t *testing.T) {
	h := newUpdateHarness()
	withDue := h.items.stored[packageID]
	withDue.Due = &domain.DueDate{
		At: time.Date(2026, 9, 1, 17, 0, 0, 0, time.UTC), TimeZone: "Europe/Berlin",
	}
	h.items.stored[packageID] = withDue

	out, err := h.handler.Descriptor().Handler.Invoke(context.Background(), actorFixture(),
		map[string]any{
			"item_id": packageID.String(),
			"due_at":  "2026-09-04T17:00:00+02:00",
		})
	if err != nil {
		t.Fatalf("the patch failed: %v", err)
	}

	if at, is := out["due_at"].(time.Time); !is ||
		!at.Equal(time.Date(2026, 9, 4, 15, 0, 0, 0, time.UTC)) {
		t.Errorf("the due date came back as %v", out["due_at"])
	}
	// The zone the patch did not touch survives the move: the merge starts from what is stored.
	if out.String("due_time_zone") != "Europe/Berlin" {
		t.Errorf("the zone came back as %v", out["due_time_zone"])
	}

	// The due writer announced it, not the update: a rule reacting to a rename must not fire
	// when a deadline moves, and the scheduler must not parse a change set.
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ItemDueChanged {
		t.Fatalf("the patch announced %+v", h.events.appended)
	}
	if len(h.items.attributes) != 0 {
		t.Error("a due-only patch wrote the attributes")
	}
	if len(h.history.entries) != 1 || h.history.entries[0].Verb != activity.ItemDueSet {
		t.Errorf("the history reads %+v", h.history.entries)
	}
	// One field moved, one entry (offline-sync.md §4.2).
	if len(h.changes.recorded) != 1 {
		t.Errorf("%d changes recorded", len(h.changes.recorded))
	}
}

func TestAPatchTouchingTitleAndDueSpendsBothWritesInOrder(t *testing.T) {
	h := newUpdateHarness()

	out, err := h.handler.Descriptor().Handler.Invoke(context.Background(), actorFixture(),
		map[string]any{
			"item_id":          packageID.String(),
			"title":            "Order the longer cable",
			"due_at":           "2026-09-04T17:00:00Z",
			"expected_version": 4,
		})
	if err != nil {
		t.Fatalf("the patch failed: %v", err)
	}

	// The attribute write goes against the version the caller matched, the due write against the
	// one that write produced - one request, spending what it spends in sequence.
	if h.written(t).expectedVersion != 4 {
		t.Errorf("the attributes wrote against %d", h.written(t).expectedVersion)
	}
	if len(h.items.dueDates) != 1 || h.items.dueDates[0].expectedVersion != 5 {
		t.Fatalf("the due write went against %+v", h.items.dueDates)
	}
	if out.Int("version") != 6 {
		t.Errorf("version %d, want 6", out.Int("version"))
	}

	kinds := map[event.Type]int{}
	for _, announcement := range h.events.appended {
		kinds[announcement.Type]++
	}
	if kinds[event.ItemUpdated] != 1 || kinds[event.ItemDueChanged] != 1 {
		t.Errorf("the patch announced %v", kinds)
	}
}

func TestAPatchNullingTheDueDateClearsTheTrioWhole(t *testing.T) {
	h := newUpdateHarness()
	withDue := h.items.stored[packageID]
	withDue.Due = &domain.DueDate{
		At: time.Date(2026, 9, 1, 17, 0, 0, 0, time.UTC), DateOnly: true, TimeZone: "Europe/Berlin",
	}
	h.items.stored[packageID] = withDue

	// Null reaches the catalogue as the empty string, exactly as the controller spells it.
	out, err := h.handler.Descriptor().Handler.Invoke(context.Background(), actorFixture(),
		map[string]any{"item_id": packageID.String(), "due_at": ""})
	if err != nil {
		t.Fatalf("the patch failed: %v", err)
	}

	if out["due_at"] != nil || out["due_time_zone"] != nil {
		t.Errorf("the trio survived the null: %v / %v", out["due_at"], out["due_time_zone"])
	}
	if h.history.entries[0].Verb != activity.ItemDueCleared {
		t.Errorf("the history reads %s", h.history.entries[0].Verb)
	}
	// All three stored fields clear by name, each its own entry.
	if len(h.changes.recorded) != 3 {
		t.Errorf("%d changes recorded, want the three cleared fields", len(h.changes.recorded))
	}
}

func TestAPatchWithAQualifierAndNoStoredDateIsRefusedWhole(t *testing.T) {
	h := newUpdateHarness()

	_, err := h.handler.Descriptor().Handler.Invoke(context.Background(), actorFixture(),
		map[string]any{
			"item_id":       packageID.String(),
			"title":         "Order the longer cable",
			"due_time_zone": "Europe/Berlin",
		})
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "items.due_time_zone_without_date" {
		t.Fatalf("a qualifier without its date answered %v", err)
	}
	// The refusal takes the whole patch with it: a merge patch is one request, and half of one
	// applied is a state nobody asked for.
	if len(h.items.attributes) != 0 {
		t.Error("the refused patch still wrote the title")
	}
	if h.items.stored[packageID].Title != "Order the cable" {
		t.Errorf("the title moved to %q", h.items.stored[packageID].Title)
	}
}

func TestAPatchMovesAndClearsTheStart(t *testing.T) {
	h := newUpdateHarness()

	out, err := h.handler.Descriptor().Handler.Invoke(context.Background(), actorFixture(),
		map[string]any{"item_id": packageID.String(), "start_at": "2026-08-30T08:00:00Z"})
	if err != nil {
		t.Fatalf("setting the start failed: %v", err)
	}
	if at, is := out["start_at"].(time.Time); !is ||
		!at.Equal(time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("the start came back as %v", out["start_at"])
	}
	// A plain attribute: the update announced it, no due event fired.
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.ItemUpdated {
		t.Fatalf("the start announced %+v", h.events.appended)
	}

	out, err = h.handler.Descriptor().Handler.Invoke(context.Background(), actorFixture(),
		map[string]any{"item_id": packageID.String(), "start_at": ""})
	if err != nil {
		t.Fatalf("clearing the start failed: %v", err)
	}
	if out["start_at"] != nil {
		t.Errorf("the start survived its clearing: %v", out["start_at"])
	}
}
