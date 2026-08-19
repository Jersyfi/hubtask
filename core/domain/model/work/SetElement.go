// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// SetName is which set of an item an element belongs to. The values are the ones the column's
// CHECK constraint allows (db/schema.sql, set_element).
type SetName string

const (
	SetLabels   SetName = "labels"
	SetMembers  SetName = "members"
	SetWatchers SetName = "watchers"
)

// SetElement is one element of a set an item carries, with the tags that decide whether it is in
// it (offline-sync.md §4.2, §10).
//
// Labels and members are sets, and a set is the one kind of field last writer wins gets wrong. Two
// devices offline: one adds "urgent", the other removes "blocked". LWW over the whole array makes
// the later of the two arrays the truth, and one of the two changes disappears - not as a conflict
// a person could see, but as a label that quietly is not there any more. So each element carries
// its own pair of tags instead, and the merge decides per element.
//
// Both tags are kept rather than only the winner. A removal that erased the addition would make a
// concurrent re-add on another device indistinguishable from an element that had never been added,
// and the two have to merge differently.
type SetElement struct {
	ElementID shared.ID
	// AddedAt is the clock reading of the last addition, zero if the element has never been added
	// on this replica.
	AddedAt shared.HLC
	// RemovedAt is the clock reading of the last removal, zero if it has never been removed.
	RemovedAt shared.HLC
}

// IsPresent reports whether the element is in the set: added, and not removed since.
//
// An element that was never added is not in the set whatever its removal tag says - a device may
// well remove something it saw and this replica did not, and the removal is then a fact about an
// element that is already absent.
func (e SetElement) IsPresent() bool {
	if e.AddedAt.IsZero() {
		return false
	}
	return e.RemovedAt.IsZero() || e.AddedAt.After(e.RemovedAt)
}

// MergeSetElements merges two replicas of one set and returns the result, ordered by element.
//
// This is the merge rule offline-sync.md §4.2 states for sets, written once: the union of the
// elements either side knows about, each element's tags being the later of the two readings. What
// falls out is "the union minus the removals that are genuinely later than the additions they
// undo" - so a label added on one device survives a different label being removed on another,
// which is the failure this rule exists to prevent.
//
// It is commutative and idempotent by construction: the result depends only on the tags, and
// HLC.Compare orders any two of them the same way on every replica - including the tie, which is
// broken by the device identifier rather than by whichever side happened to be scanned first.
//
// Nothing calls it from a request path yet. The transport that will - `POST /sync:push`, which
// merges a device's tags into the server's - arrives with the sync task; this is the rule it
// applies, defined and tested where it belongs rather than inside the endpoint that will need it.
func MergeSetElements(mine, theirs []SetElement) []SetElement {
	merged := make(map[shared.ID]SetElement, len(mine)+len(theirs))

	for _, side := range [][]SetElement{mine, theirs} {
		for _, element := range side {
			known, seen := merged[element.ElementID]
			if !seen {
				merged[element.ElementID] = element
				continue
			}
			merged[element.ElementID] = SetElement{
				ElementID: element.ElementID,
				AddedAt:   laterOf(known.AddedAt, element.AddedAt),
				RemovedAt: laterOf(known.RemovedAt, element.RemovedAt),
			}
		}
	}

	result := make([]SetElement, 0, len(merged))
	for _, element := range merged {
		result = append(result, element)
	}
	// Sorted, because a map's iteration order is deliberately random in Go and a merge result that
	// came back in a different order each time would be impossible to compare, to log, or to test.
	slices.SortFunc(result, func(a, b SetElement) int {
		return strings.Compare(a.ElementID.String(), b.ElementID.String())
	})
	return result
}

// PresentElements is the membership a merged set describes: the elements that are in it.
func PresentElements(elements []SetElement) []shared.ID {
	present := make([]shared.ID, 0, len(elements))
	for _, element := range elements {
		if element.IsPresent() {
			present = append(present, element.ElementID)
		}
	}
	return present
}

// laterOf is the winner of two clock readings, and the zero reading loses to everything. A zero on
// one side means that replica never saw the event at all, which is not the same as having seen it
// at the beginning of time.
func laterOf(a, b shared.HLC) shared.HLC {
	switch {
	case a.IsZero():
		return b
	case b.IsZero():
		return a
	case b.After(a):
		return b
	}
	return a
}
