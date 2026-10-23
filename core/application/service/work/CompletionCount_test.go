// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-WRK-25 check 4's ranking: how often an entry went from open to done. A completion counts, a
// repeat counts nothing (UC-WRK-04 check 5), a reopen keeps the count, and an entry the roll-up
// completes counts like one completed by hand.

func countOf(h *completionHarness, id domain.WorkItem) int {
	return h.items.stored[id.ID].CompletionCount
}

func TestACompletionCountsAndARepeatDoesNot(t *testing.T) {
	h := newCompletionHarness(domain.CompletionManual)
	task := h.items.stored[taskID]

	complete := func() {
		t.Helper()
		if _, err := (CompleteWorkItem{Completion: h.writer}).Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: taskID}); err != nil {
			t.Fatalf("completing: %v", err)
		}
	}
	reopen := func() {
		t.Helper()
		if _, err := (ReopenWorkItem{Completion: h.writer}).Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: taskID}); err != nil {
			t.Fatalf("reopening: %v", err)
		}
	}

	complete()
	complete()
	if got := countOf(h, task); got != 1 {
		t.Errorf("after a completion and its repeat the count is %d, want 1", got)
	}
	reopen()
	if got := countOf(h, task); got != 1 {
		t.Errorf("a reopen changed the count to %d", got)
	}
	complete()
	if got := countOf(h, task); got != 2 {
		t.Errorf("after the second completion the count is %d, want 2", got)
	}
	if out := ItemOutput(h.items.stored[taskID]); out["completion_count"] != 2 {
		t.Errorf("the answer carries %#v", out["completion_count"])
	}
}

func TestAnEntryTheRollUpCompletesCounts(t *testing.T) {
	h := newCompletionHarness(domain.CompletionRollup)
	h.alreadyDone(otherActID)
	h.summarise(packageID, 2, 2)

	if _, err := (CompleteWorkItem{Completion: h.writer}).Execute(t.Context(), actorFixture(), CompletionCommand{ItemID: activityID}); err != nil {
		t.Fatalf("completing: %v", err)
	}
	if got := h.items.stored[packageID].CompletionCount; got != 1 {
		t.Errorf("the work package the roll-up completed counts %d", got)
	}
}
