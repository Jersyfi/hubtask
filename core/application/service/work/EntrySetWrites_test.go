// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-WRK-24 check 3, the sets and the threads: what a List collection hides is neither added nor
// taken away, and nothing of it is lost.

func TestHiddenLabelsAreNeitherAddedNorRemoved(t *testing.T) {
	h := newItemLabelHarness(t)
	h.withItem(domain.ItemTask)
	h.itemLabels.carried[labelledItem] = append(h.itemLabels.carried[labelledItem], urgentLabel)
	gate := gateOver(h.items, h.containers)
	h.add.Writer.Gate, h.remove.Writer.Gate = gate, gate
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	_, err := h.add.Execute(t.Context(), actor(), labelCmd())
	refusedFor(t, err, "items.capability_hidden")
	_, err = h.remove.Execute(t.Context(), actor(), labelCmd())
	refusedFor(t, err, "items.capability_hidden")
	if len(h.itemLabels.carried[labelledItem]) != 1 {
		t.Fatalf("the hidden label is gone: %v", h.itemLabels.carried[labelledItem])
	}

	widen(h.containers, collectionID)
	if _, err := h.remove.Execute(t.Context(), actor(), labelCmd()); err != nil {
		t.Fatalf("widened, removing was refused: %v", err)
	}
}

func TestHiddenMembersAreKeptUnlessTheyLostSightOfTheCollection(t *testing.T) {
	h := newItemMemberHarness(t)
	h.withItem(domain.ItemTask)
	gate := gateOver(h.items, h.containers)
	h.add.Writer.Gate, h.remove.Writer.Gate = gate, gate
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	_, err := h.add.Execute(t.Context(), actor(), memberCmd())
	refusedFor(t, err, "items.capability_hidden")
	_, err = h.remove.Execute(t.Context(), actor(), memberCmd())
	refusedFor(t, err, "items.capability_hidden")

	// Somebody who can no longer see the collection may still be taken off: an entry on somebody
	// who cannot see it is one nobody can act on.
	cmd := memberCmd()
	cmd.AccountID = strangerID
	if _, err := h.remove.Execute(t.Context(), actor(), cmd); err != nil {
		t.Fatalf("removing a member who lost sight was refused: %v", err)
	}
}

func TestAHiddenAttachmentIsNeitherAddedNorRemoved(t *testing.T) {
	h := newAttachmentHarness(t)
	h.withItem(domain.ItemTask)
	gate := gateOver(h.items, h.containers)
	h.attach.Writer.Gate, h.detach.Writer.Gate = gate, gate
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	_, err := h.attach.Execute(t.Context(), actor(), attachCmd())
	refusedFor(t, err, "items.capability_hidden")
	_, err = h.detach.Execute(t.Context(), actor(), attachCmd())
	refusedFor(t, err, "items.capability_hidden")
}

func TestAHiddenThreadIsKeptButModerationStillRemoves(t *testing.T) {
	h := newCommentHarness(t)
	h.withItem(domain.ItemTask)
	h.writer.Gate = gateOver(h.items, h.containers)
	narrowTo(t, h.containers, collectionID, domain.PresetList)
	own := h.withComment("0192f000-0000-7000-8000-0000000000b1", accountID, "Mine")
	theirs := h.withComment("0192f000-0000-7000-8000-0000000000b2", otherAccount, "Theirs")

	_, err := AddComment{Writer: h.writer}.Execute(t.Context(), actor(), addCmd("Another"))
	refusedFor(t, err, "items.capability_hidden")

	h.withModeration(identity.RoleAdmin, true)
	_, err = EditComment{Writer: h.writer}.Execute(t.Context(), actor(), editCmd(own.ID, "Changed"))
	refusedFor(t, err, "items.capability_hidden")
	_, err = DeleteComment{Writer: h.writer}.Execute(t.Context(), actor(), editCmd(own.ID, ""))
	refusedFor(t, err, "items.capability_hidden")

	// An administrator removing somebody else's comment is the lifecycle, which no structure stops.
	if _, err := (DeleteComment{Writer: h.writer}).Execute(t.Context(), actor(), editCmd(theirs.ID, "")); err != nil {
		t.Fatalf("moderation was refused: %v", err)
	}
}

func TestHiddenRemindersAreNeitherSetNorDeleted(t *testing.T) {
	h := newReminderHarness(t, reminderProfiles())
	h.withItem(remindedDue(t))
	h.withReminder("REL:-PT1H", remindedDue(t))
	gate := gateOver(h.items, h.containers)
	h.create.Writer.Gate, h.remove.Writer.Gate = gate, gate
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	_, err := h.create.Execute(t.Context(), actor(), CreateReminderCommand{ItemID: remindedItem, OffsetSpec: "REL:-PT2H"})
	refusedFor(t, err, "items.capability_hidden")
	err = h.remove.Execute(t.Context(), actor(), ChangeReminderCommand{ItemID: remindedItem, ReminderID: reminderID})
	refusedFor(t, err, "items.capability_hidden")
	if _, held := h.reminders.stored[reminderID]; !held {
		t.Fatal("the hidden reminder is gone")
	}
}

func TestAHiddenSeriesIsNeitherSetRemovedNorSkipped(t *testing.T) {
	h := newRecurrenceHarness(t)
	h.withItem(domain.ItemTask, seriesDate(t))
	gate := gateOver(h.items, h.containers)
	h.set.Writer.Gate, h.remove.Writer.Gate, h.skip.Writer.Gate = gate, gate, gate
	narrowTo(t, h.containers, collectionID, domain.PresetList)

	_, err := h.set.Execute(t.Context(), actor(), weeklyCommand())
	refusedFor(t, err, "items.capability_hidden")
	err = h.remove.Execute(t.Context(), actor(), RemoveRecurrenceCommand{ItemID: recurringItem})
	refusedFor(t, err, "items.capability_hidden")
	_, err = h.skip.Execute(t.Context(), actor(), SkipOccurrenceCommand{ItemID: recurringItem})
	refusedFor(t, err, "items.capability_hidden")
}
