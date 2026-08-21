// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	urgent  = shared.MustParseID("0192f000-0000-7000-8000-0000000000d1")
	blocked = shared.MustParseID("0192f000-0000-7000-8000-0000000000d2")
	waiting = shared.MustParseID("0192f000-0000-7000-8000-0000000000d3")

	syncBase = time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
)

// tag is a clock reading of one device, `at` milliseconds after the base. The device is part of the
// reading rather than beside it, which is what lets two devices stamp the same millisecond and
// still be ordered the same way everywhere.
func tag(t *testing.T, device string, at int) shared.HLC {
	t.Helper()
	hlc, err := shared.NewHLC(syncBase.Add(time.Duration(at)*time.Millisecond), 0, device)
	if err != nil {
		t.Fatalf("the clock reading was refused: %v", err)
	}
	return hlc
}

func TestSetElementIsPresent(t *testing.T) {
	for _, c := range []struct {
		name    string
		element work.SetElement
		want    bool
	}{
		{name: "never touched", element: work.SetElement{ElementID: urgent}},
		{name: "added", element: work.SetElement{ElementID: urgent, AddedAt: tag(t, "a", 1)}, want: true},
		{
			name:    "removed after it was added",
			element: work.SetElement{ElementID: urgent, AddedAt: tag(t, "a", 1), RemovedAt: tag(t, "a", 2)},
		},
		{
			name:    "added again after it was removed",
			element: work.SetElement{ElementID: urgent, AddedAt: tag(t, "a", 3), RemovedAt: tag(t, "a", 2)},
			want:    true,
		},
		{
			// A device may remove something this replica never saw added. The removal is then a
			// fact about an element that is already absent, not a reason to consider it present.
			name:    "removed without ever having been added",
			element: work.SetElement{ElementID: urgent, RemovedAt: tag(t, "b", 2)},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := c.element.IsPresent(); got != c.want {
				t.Errorf("present %v, want %v", got, c.want)
			}
		})
	}
}

// The acceptance criterion of B-09, and the reason labels are an OR-set rather than an array under
// last writer wins: two devices working offline converge on the union of what they added, minus the
// removals that genuinely undo an addition. Neither device's change disappears because the other
// one happened to write later.
func TestTwoOfflineDevicesConvergeOnTheUnionMinusTheRemovals(t *testing.T) {
	// Both devices start from the same state: the item carries "blocked".
	starting := []work.SetElement{{ElementID: blocked, AddedAt: tag(t, "server", 0)}}

	// Phone adds "urgent". Laptop, at the same time and without seeing that, removes "blocked" and
	// adds "waiting".
	phone := append(slices.Clone(starting),
		work.SetElement{ElementID: urgent, AddedAt: tag(t, "phone", 10)})
	laptop := append(slices.Clone(starting),
		work.SetElement{ElementID: waiting, AddedAt: tag(t, "laptop", 11)})
	laptop[0] = work.SetElement{
		ElementID: blocked, AddedAt: tag(t, "server", 0), RemovedAt: tag(t, "laptop", 12),
	}

	merged := work.MergeSetElements(phone, laptop)
	present := work.PresentElements(merged)

	if len(present) != 2 || !slices.Contains(present, urgent) || !slices.Contains(present, waiting) {
		t.Fatalf("the merged set is %v, want urgent and waiting", present)
	}
	if slices.Contains(present, blocked) {
		t.Error("the explicit removal was lost")
	}

	// The other direction has to give the same answer, or the two devices never agree.
	if other := work.MergeSetElements(laptop, phone); !slices.Equal(
		work.PresentElements(other), present) {
		t.Errorf("merging the other way round gives %v, want %v",
			work.PresentElements(other), present)
	}

	// And merging a device's own result back in changes nothing, which is what lets a client push
	// the same batch twice after a lost response.
	if again := work.MergeSetElements(merged, phone); !slices.Equal(
		work.PresentElements(again), present) {
		t.Errorf("merging again gives %v, want %v", work.PresentElements(again), present)
	}
}

// A re-add after a removal wins when it is genuinely later - which is the case the tags exist for.
// Keeping only the winner of the pair would make this indistinguishable from an element that had
// never been added at all.
func TestAConcurrentReAddBeatsAnEarlierRemoval(t *testing.T) {
	removed := []work.SetElement{{
		ElementID: urgent, AddedAt: tag(t, "server", 0), RemovedAt: tag(t, "laptop", 5),
	}}
	readded := []work.SetElement{{ElementID: urgent, AddedAt: tag(t, "phone", 9)}}

	merged := work.MergeSetElements(removed, readded)

	if len(merged) != 1 || !merged[0].IsPresent() {
		t.Fatalf("the re-add was discarded: %+v", merged)
	}
	if merged[0].RemovedAt.IsZero() {
		t.Error("the removal tag was erased - a third replica could no longer merge against it")
	}
}

// Two devices stamping the same millisecond with the same counter are ordered by their identifier,
// so both replicas reach the same result. Without that, the two of them would disagree about a tie
// - the one failure a merge rule must not have.
func TestATieIsBrokenTheSameWayOnBothSides(t *testing.T) {
	mine := []work.SetElement{{ElementID: urgent, AddedAt: tag(t, "aaa", 7)}}
	theirs := []work.SetElement{{ElementID: urgent, RemovedAt: tag(t, "zzz", 7)}}

	forwards := work.MergeSetElements(mine, theirs)
	backwards := work.MergeSetElements(theirs, mine)

	if forwards[0].IsPresent() != backwards[0].IsPresent() {
		t.Errorf("the two replicas disagree: %v and %v",
			forwards[0].IsPresent(), backwards[0].IsPresent())
	}
	if forwards[0].IsPresent() {
		t.Error("the removal from the higher device identifier lost the tie")
	}
}

// The result is ordered, whatever order the two sides arrived in: a map's iteration order is
// deliberately random in Go, and a merge that came back differently each time could not be
// compared, logged, or tested.
func TestTheMergedSetIsOrdered(t *testing.T) {
	mine := []work.SetElement{
		{ElementID: waiting, AddedAt: tag(t, "a", 1)},
		{ElementID: urgent, AddedAt: tag(t, "a", 2)},
	}
	theirs := []work.SetElement{{ElementID: blocked, AddedAt: tag(t, "b", 3)}}

	merged := work.MergeSetElements(mine, theirs)

	if !slices.IsSortedFunc(merged, func(a, b work.SetElement) int {
		return strings.Compare(a.ElementID.String(), b.ElementID.String())
	}) {
		t.Errorf("the merged set is not ordered: %+v", merged)
	}
}
