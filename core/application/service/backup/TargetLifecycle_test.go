// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"errors"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// deletion wires a writer that can see both the targets and the schedules, which is the one
// operation that has to.
func deletion(h *harness, schedules *scheduleStore) Writer {
	writer := h.writer()
	writer.Schedules = schedules
	return writer
}

// A target a schedule still names is refused, with the schedules named. Deleting it silently
// would disarm a backup that runs every night, and the disarming would be discovered by whoever
// needed the archive.
func TestATargetASchedulePointsAtIsNotRemoved(t *testing.T) {
	h := newHarness()
	h.targets.stored = append(h.targets.stored, domain.Target{ID: targetID, Name: "Off-site bucket"})
	schedules := newSchedules()
	seededSchedule(t, schedules)

	err := (DeleteBackupTarget{Writer: deletion(h, schedules)}).
		Execute(t.Context(), caller(), targetID)

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error %v is not a domain error", err)
	}
	if domainErr.DetailCode != domain.CodeTargetInUse {
		t.Errorf("detail code %q", domainErr.DetailCode)
	}
	// The identifiers, not only a count: an operator told "one schedule" still has to find it.
	if !strings.Contains(domainErr.Params["schedule_ids"], scheduleID.String()) {
		t.Errorf("the refusal names %q", domainErr.Params["schedule_ids"])
	}
	if len(h.targets.removed) != 0 {
		t.Error("the target was removed despite the refusal")
	}
	if len(h.audit.entries) != 0 {
		t.Error("a refused removal was recorded as one that happened")
	}
}

// With nothing pointing at it the target goes, and the archives at it are not this operation's
// business at all - which is why nothing here opens the store.
func TestRemovingAnUnreferencedTargetTouchesNothingAtIt(t *testing.T) {
	h := newHarness()
	h.targets.stored = append(h.targets.stored, domain.Target{ID: targetID, Name: "Off-site bucket"})
	before := len(h.opener.opened)

	if err := (DeleteBackupTarget{Writer: deletion(h, newSchedules())}).
		Execute(t.Context(), caller(), targetID); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if len(h.targets.removed) != 1 || h.targets.removed[0] != targetID {
		t.Errorf("removed %v", h.targets.removed)
	}
	if len(h.opener.opened) != before {
		t.Error("removing a target opened the store it names")
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != TargetRemovedAction {
		t.Errorf("the trail says %v", h.audit.entries)
	}
}

// Removing one that is not there is not a failure.
func TestRemovingATargetThatIsNotThereIsNotAnError(t *testing.T) {
	h := newHarness()

	if err := (DeleteBackupTarget{Writer: deletion(h, newSchedules())}).
		Execute(t.Context(), caller(), targetID); err != nil {
		t.Fatalf("removing an absent target: %v", err)
	}
	if len(h.audit.entries) != 0 {
		t.Error("removing nothing was recorded as a removal")
	}
}
