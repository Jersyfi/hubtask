// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	hubID        = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	collectionID = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	otherHubID   = shared.MustParseID("0192f000-0000-7000-8000-00000000001b")
	taskID       = shared.MustParseID("0192f000-0000-7000-8000-000000000001")
	packageID    = shared.MustParseID("0192f000-0000-7000-8000-000000000002")
	accountID    = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	holdID       = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
)

// A work package three levels down, as a purge would present it: the containers above it, and the
// entries above it including itself.
func target() lifecycle.Target {
	return lifecycle.Target{
		ItemID:          packageID,
		ContainerIDs:    []shared.ID{hubID, collectionID},
		AncestorItemIDs: []shared.ID{taskID, packageID},
	}
}

func hold(scope lifecycle.HoldScope, id shared.ID) lifecycle.LegalHold {
	return lifecycle.LegalHold{ID: holdID, Scope: scope, ScopeID: id, Reason: "Pending litigation"}
}

// The precedence rule of data-retention.md §4: a legal hold is checked first and overrides
// everything. What it covers is a path rather than a row - a hold on a hub has to reach an entry
// three levels below it, or it would be a hold in name only.
func TestAHoldReachesEverythingBelowWhatItNames(t *testing.T) {
	for _, c := range []struct {
		name    string
		holds   lifecycle.Holds
		blocked bool
	}{
		{"nothing held", nil, false},
		{"the whole tenant", lifecycle.Holds{hold(lifecycle.HoldTenant, "")}, true},
		{"the hub above it", lifecycle.Holds{hold(lifecycle.HoldContainer, hubID)}, true},
		{"its own collection", lifecycle.Holds{hold(lifecycle.HoldContainer, collectionID)}, true},
		{"another hub", lifecycle.Holds{hold(lifecycle.HoldContainer, otherHubID)}, false},
		{"the entry above it", lifecycle.Holds{hold(lifecycle.HoldItem, taskID)}, true},
		{"the entry itself", lifecycle.Holds{hold(lifecycle.HoldItem, packageID)}, true},
		{"a sibling entry", lifecycle.Holds{hold(lifecycle.HoldItem, shared.MustParseID("0192f000-0000-7000-8000-000000000003"))}, false},
		// A hold on a person is about their own data - their profile, their trail - and is answered
		// where that is erased. It does not freeze every entry they ever touched.
		{"an account", lifecycle.Holds{hold(lifecycle.HoldAccount, accountID)}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			blocking, blocked := c.holds.Blocking(target())

			if blocked != c.blocked {
				t.Errorf("blocked %v, want %v", blocked, c.blocked)
			}
			if blocked && blocking.ID != holdID {
				t.Errorf("the blocking hold is %q, want the one that was placed", blocking.ID)
			}
		})
	}
}

// The hold itself is returned rather than a boolean, because a blocked run has to be able to say
// why: "a hold placed on the hub" is an answer an operator can act on where "blocked" is not.
func TestTheBlockingHoldIsNamed(t *testing.T) {
	held := hold(lifecycle.HoldContainer, hubID)
	holds := lifecycle.Holds{hold(lifecycle.HoldContainer, otherHubID), held}

	blocking, blocked := holds.Blocking(target())
	if !blocked {
		t.Fatal("a hold on the hub did not block")
	}
	if blocking.Scope != lifecycle.HoldContainer || blocking.ScopeID != hubID {
		t.Errorf("the answer names %s/%s, want the hold on the hub", blocking.Scope, blocking.ScopeID)
	}
	if blocking.Reason == "" {
		t.Error("the answer carries no reason for an operator to read")
	}
}

// A container's own purge asks the same question with no entry in the target.
func TestAContainerTargetIsJudgedByItsOwnPath(t *testing.T) {
	container := lifecycle.Target{ContainerIDs: []shared.ID{hubID, collectionID}}

	if _, blocked := (lifecycle.Holds{hold(lifecycle.HoldContainer, hubID)}).Blocking(container); !blocked {
		t.Error("a hold on the hub did not block its collection")
	}
	// An item hold names nothing in a container target, and an empty identifier must not match the
	// empty ItemID - which is what a naive equality would do.
	if _, blocked := (lifecycle.Holds{hold(lifecycle.HoldItem, "")}).Blocking(container); blocked {
		t.Error("a hold naming no entry blocked a container")
	}
}
