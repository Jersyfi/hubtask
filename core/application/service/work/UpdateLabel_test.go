// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

type labelWriterHarness struct {
	*labelHarness
	update UpdateLabel
	remove DeleteLabel
}

func newLabelWriterHarness() *labelWriterHarness {
	base := newLabelHarness()
	writer := LabelWriter{
		Labels: base.labels, Containers: base.containers, Authorizer: base.authorizer,
		Events: base.events, Changes: base.changes, Audit: base.audit, UnitOfWork: base.uow,
		Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
	}
	return &labelWriterHarness{
		labelHarness: base,
		update:       UpdateLabel{Writer: writer},
		remove:       DeleteLabel{Writer: writer},
	}
}

// One change owes four things, and this is the test that says so.
func TestUpdatingALabelWritesTheRowTheEventTheChangeAndTheEntry(t *testing.T) {
	h := newLabelWriterHarness()
	h.withLabel(urgentLabel, "Urgent")
	ctx := correlation.ContextWithRequestID(context.Background(), "01J9REQUEST")

	colour := "accent.amber"
	label, err := h.update.Execute(ctx, actor(), UpdateLabelCommand{
		LabelID: urgentLabel, Attributes: domain.LabelAttributes{ColorToken: &colour},
	})
	if err != nil {
		t.Fatalf("updating the label failed: %v", err)
	}

	if label.ColorToken != "accent.amber" || label.Version != 2 {
		t.Errorf("unexpected label: %+v", label)
	}
	if len(h.labels.written) != 1 || h.labels.written[0].method != "attributes" {
		t.Fatalf("unexpected writes: %+v", h.labels.written)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.LabelUpdated {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}
	if len(h.changes.recorded) != 1 || h.changes.recorded[0].Op != changelog.Upsert {
		t.Fatalf("unexpected changes: %+v", h.changes.recorded)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != LabelUpdatedAction {
		t.Fatalf("unexpected audit entries: %+v", h.audit.entries)
	}

	changeSet, _ := h.events.appended[0].Payload["change_set"].(map[string]any)
	if changeSet == nil || changeSet[domain.FieldColorToken] == nil {
		t.Errorf("the change set does not name the field that moved: %+v", changeSet)
	}
}

// One entry per field, each with its own clock reading: a device that renamed a label while another
// recoloured it has to keep both changes (offline-sync.md §4.2).
func TestEachLabelFieldThatMovesGetsItsOwnEntry(t *testing.T) {
	h := newLabelWriterHarness()
	h.withLabel(urgentLabel, "Urgent")

	name, description := "Blocked", "Waiting on somebody else"
	if _, err := h.update.Execute(context.Background(), actor(), UpdateLabelCommand{
		LabelID:    urgentLabel,
		Attributes: domain.LabelAttributes{Name: &name, Description: &description},
	}); err != nil {
		t.Fatalf("updating the label failed: %v", err)
	}

	if len(h.changes.recorded) != 2 {
		t.Fatalf("%d change log entries, want one per field", len(h.changes.recorded))
	}
	if h.changes.recorded[0].HLC.Compare(h.changes.recorded[1].HLC) == 0 {
		t.Error("the two entries share a clock reading, so a merge would decide them together")
	}
}

// A cleared description travels as null: the API spells "not set" that way.
func TestAClearedDescriptionTravelsAsNull(t *testing.T) {
	h := newLabelWriterHarness()
	label := h.withLabel(urgentLabel, "Urgent")
	label.Description = "Needs a decision today"
	h.labels.stored[urgentLabel] = label

	empty := ""
	if _, err := h.update.Execute(context.Background(), actor(), UpdateLabelCommand{
		LabelID: urgentLabel, Attributes: domain.LabelAttributes{Description: &empty},
	}); err != nil {
		t.Fatalf("clearing the description failed: %v", err)
	}

	if value := h.changes.recorded[0].Payload[domain.FieldDescription]; value != nil {
		t.Errorf("the cleared description travels as %v, want null", value)
	}
}

// A label is rendered as a chip and nothing else, so the colour has no "set it to nothing".
func TestALabelsColourCannotBeCleared(t *testing.T) {
	h := newLabelWriterHarness()
	h.withLabel(urgentLabel, "Urgent")

	empty := ""
	_, err := h.update.Execute(context.Background(), actor(), UpdateLabelCommand{
		LabelID: urgentLabel, Attributes: domain.LabelAttributes{ColorToken: &empty},
	})
	if shared.AsError(err).DetailCode != "labels.color_token_empty" {
		t.Fatalf("detail code %s", shared.AsError(err).DetailCode)
	}
}

func TestAnUpdateToALabelThatChangesNothingWritesNothing(t *testing.T) {
	h := newLabelWriterHarness()
	h.withLabel(urgentLabel, "Urgent")

	name := "Urgent"
	label, err := h.update.Execute(context.Background(), actor(), UpdateLabelCommand{
		LabelID: urgentLabel, Attributes: domain.LabelAttributes{Name: &name},
	})
	if err != nil {
		t.Fatalf("the update failed: %v", err)
	}
	if label.Version != 1 {
		t.Errorf("the version was spent: %d", label.Version)
	}
	if len(h.labels.written) != 0 || len(h.events.appended) != 0 {
		t.Error("an update that moved nothing wrote something")
	}
}

func TestALabelUpdateHonoursTheExpectedVersion(t *testing.T) {
	h := newLabelWriterHarness()
	label := h.withLabel(urgentLabel, "Urgent")
	label.Version = 4
	h.labels.stored[urgentLabel] = label

	name := "Blocked"
	t.Run("the version reaches the repository", func(t *testing.T) {
		if _, err := h.update.Execute(context.Background(), actor(), UpdateLabelCommand{
			LabelID:    urgentLabel,
			Attributes: domain.LabelAttributes{Name: &name}, ExpectedVersion: 4,
		}); err != nil {
			t.Fatalf("the update failed: %v", err)
		}
		if len(h.labels.written) != 1 || h.labels.written[0].expected != 4 {
			t.Fatalf("the write did not carry the version: %+v", h.labels.written)
		}
	})

	t.Run("a stale version is refused", func(t *testing.T) {
		_, err := h.update.Execute(context.Background(), actor(), UpdateLabelCommand{
			LabelID:    urgentLabel,
			Attributes: domain.LabelAttributes{Name: &name}, ExpectedVersion: 1,
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Fatalf("a stale version was accepted: %v", err)
		}
	})
}

// A deletion is one change log entry with no payload: there is nothing left to describe, and a
// tombstone with content would be a copy of the deleted object living on in the log.
func TestDeletingALabelRecordsATombstone(t *testing.T) {
	h := newLabelWriterHarness()
	h.withLabel(urgentLabel, "Urgent")

	label, err := h.remove.Execute(context.Background(), actor(), DeleteLabelCommand{
		LabelID: urgentLabel,
	})
	if err != nil {
		t.Fatalf("deleting the label failed: %v", err)
	}

	if !label.IsDeleted() {
		t.Error("the label is still in the vocabulary")
	}
	if len(h.labels.written) != 1 || h.labels.written[0].method != "deleted" {
		t.Fatalf("unexpected writes: %+v", h.labels.written)
	}
	if len(h.events.appended) != 1 || h.events.appended[0].Type != event.LabelDeleted {
		t.Fatalf("unexpected events: %+v", h.events.appended)
	}

	if len(h.changes.recorded) != 1 {
		t.Fatalf("%d changes, want 1", len(h.changes.recorded))
	}
	change := h.changes.recorded[0]
	if change.Op != changelog.Delete || change.Payload != nil {
		t.Errorf("the change is not a tombstone: %+v", change)
	}
	if change.ContainerID != hubID {
		t.Errorf("the change is filed under %s, want the hub", change.ContainerID)
	}

	// A deletion stamp is a timestamp this server produced, and "when did this label go" is
	// precisely what an auditor asks.
	recorded, _ := h.audit.entries[0].Changes[domain.FieldDeletedAt].(map[string]any)
	if recorded == nil || recorded["to"] == nil {
		t.Errorf("the stamp is not readable in the trail: %+v", h.audit.entries[0].Changes)
	}
}

// Deleting a label that is already gone succeeds and announces nothing, which is what makes a
// retry after a lost response harmless.
func TestDeletingALabelTwiceAnnouncesNothing(t *testing.T) {
	h := newLabelWriterHarness()
	h.withLabel(urgentLabel, "Urgent")

	if _, err := h.remove.Execute(context.Background(), actor(), DeleteLabelCommand{
		LabelID: urgentLabel,
	}); err != nil {
		t.Fatalf("the first deletion failed: %v", err)
	}
	if _, err := h.remove.Execute(context.Background(), actor(), DeleteLabelCommand{
		LabelID: urgentLabel,
	}); err != nil {
		t.Fatalf("the second deletion failed: %v", err)
	}

	if len(h.events.appended) != 1 || len(h.audit.entries) != 1 {
		t.Error("the second deletion announced something")
	}
}

// A deleted label is out of the vocabulary, and an update to one says so with a conflict.
func TestADeletedLabelCannotBeUpdated(t *testing.T) {
	h := newLabelWriterHarness()
	label := h.withLabel(urgentLabel, "Urgent")
	deleted, _, err := label.Deleted(now)
	if err != nil {
		t.Fatalf("deleting the label: %v", err)
	}
	h.labels.stored[urgentLabel] = deleted

	name := "Blocked"
	_, err = h.update.Execute(context.Background(), actor(), UpdateLabelCommand{
		LabelID: urgentLabel, Attributes: domain.LabelAttributes{Name: &name},
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a deleted label was renamed: %v", err)
	}
}

func TestALabelChangeIsRefusedOnAnArchivedCollection(t *testing.T) {
	h := newLabelWriterHarness()
	h.withLabel(urgentLabel, "Urgent")

	collection := h.containers.stored[collectionID]
	archivedAt := now
	collection.ArchivedAt = &archivedAt
	h.containers.stored[collectionID] = collection

	_, err := h.remove.Execute(context.Background(), actor(), DeleteLabelCommand{
		LabelID: urgentLabel,
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("a label in an archived collection was deleted: %v", err)
	}
}

func TestALabelChangeNeedsALabelThatExists(t *testing.T) {
	h := newLabelWriterHarness()

	t.Run("no label named", func(t *testing.T) {
		_, err := h.remove.Execute(context.Background(), actor(), DeleteLabelCommand{})
		if shared.AsError(err).DetailCode != "labels.label_id_required" {
			t.Fatalf("detail code %s", shared.AsError(err).DetailCode)
		}
	})

	t.Run("a label nobody has", func(t *testing.T) {
		_, err := h.remove.Execute(context.Background(), actor(), DeleteLabelCommand{
			LabelID: urgentLabel,
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("an unknown label was accepted: %v", err)
		}
	})
}

func TestARefusedLabelChangeWritesNothing(t *testing.T) {
	h := newLabelWriterHarness()
	h.withLabel(urgentLabel, "Urgent")
	h.authorizer.err = shared.ErrForbidden

	_, err := h.remove.Execute(context.Background(), actor(), DeleteLabelCommand{
		LabelID: urgentLabel,
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("the refusal did not come back: %v", err)
	}
	if len(h.labels.written) != 0 || h.uow.writes != 0 {
		t.Error("a refused change wrote something")
	}
}

func TestTheLabelChangeDescriptorsCarryWhatTheChannelsNeed(t *testing.T) {
	update := UpdateLabel{}.Descriptor()
	if update.Name != UpdateLabelName || !update.Audit.Required {
		t.Errorf("unexpected descriptor: %+v", update)
	}

	remove := DeleteLabel{}.Descriptor()
	if remove.Audit.Action != LabelDeletedAction {
		t.Errorf("the deletion writes the wrong audit action: %s", remove.Audit.Action)
	}
}

// The untyped input becomes the typed command in one place, for all three channels.
func TestTheLabelUpdateReadsAbsenceAndEmptinessApart(t *testing.T) {
	h := newLabelWriterHarness()
	label := h.withLabel(urgentLabel, "Urgent")
	label.Description = "Needs a decision today"
	h.labels.stored[urgentLabel] = label

	t.Run("a field nobody sent is left alone", func(t *testing.T) {
		out, err := h.update.invoke(context.Background(), actor(), map[string]any{
			"label_id": urgentLabel.String(), "name": "Blocked",
		})
		if err != nil {
			t.Fatalf("the update failed: %v", err)
		}
		if out["description"] != "Needs a decision today" {
			t.Errorf("the description moved: %v", out["description"])
		}
	})

	t.Run("an empty description clears it", func(t *testing.T) {
		out, err := h.update.invoke(context.Background(), actor(), map[string]any{
			"label_id": urgentLabel.String(), "description": "",
		})
		if err != nil {
			t.Fatalf("the update failed: %v", err)
		}
		if out["description"] != nil {
			t.Errorf("the description survived: %v", out["description"])
		}
	})

	t.Run("an update that asks for nothing is refused", func(t *testing.T) {
		_, err := h.update.invoke(context.Background(), actor(), map[string]any{
			"label_id": urgentLabel.String(),
		})
		if shared.AsError(err).DetailCode != "labels.update_empty" {
			t.Fatalf("detail code %s", shared.AsError(err).DetailCode)
		}
	})
}
