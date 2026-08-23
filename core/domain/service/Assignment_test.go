// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

const (
	ada   = shared.ID("00000000-0000-7000-8000-00000000000a")
	ben   = shared.ID("00000000-0000-7000-8000-00000000000b")
	chris = shared.ID("00000000-0000-7000-8000-00000000000c")
	dana  = shared.ID("00000000-0000-7000-8000-00000000000d")
)

// eligible builds the account-strategy material: all listed accounts eligible, positions matching
// the configured order.
func eligible(accounts ...shared.ID) service.AssignmentSelection {
	positions := make([]int, len(accounts))
	for i := range accounts {
		positions[i] = i
	}
	return service.AssignmentSelection{
		Accounts: accounts, Positions: positions, CandidateCount: len(accounts),
	}
}

// The acceptance criterion of C-02: every strategy as a table, with an injected random source and
// no infrastructure in it.
func TestEveryStrategyPicksDeterministically(t *testing.T) {
	cases := []struct {
		name      string
		strategy  work.AutoAssignStrategy
		selection service.AssignmentSelection
		want      shared.ID
	}{
		{
			name:      "FIXED picks the one configured account",
			strategy:  work.AssignFixed,
			selection: eligible(ada),
			want:      ada,
		},
		{
			name:      "RANDOM_MEMBER follows the drawn index",
			strategy:  work.AssignRandomMember,
			selection: withRandom(eligible(ada, ben, chris), clock.NewScripted(2)),
			want:      chris,
		},
		{
			name:     "RANDOM_GROUP_MEMBER draws the group first and the member second",
			strategy: work.AssignRandomGroupMember,
			selection: service.AssignmentSelection{
				Groups: []service.AssignmentGroup{
					{GroupID: "g1", Members: []shared.ID{ada, ben}},
					{GroupID: "g2", Members: []shared.ID{chris, dana}},
				},
				Random: clock.NewScripted(1, 0),
			},
			want: chris,
		},
		{
			name:      "ROUND_ROBIN picks whoever the cursor stands on",
			strategy:  work.AssignRoundRobin,
			selection: withCursor(eligible(ada, ben, chris), 1),
			want:      ben,
		},
		{
			name:     "LEAST_LOADED picks the lowest count",
			strategy: work.AssignLeastLoaded,
			selection: withLoad(eligible(ada, ben, chris),
				map[shared.ID]int{ada: 3, ben: 1, chris: 2}),
			want: ben,
		},
		{
			name:     "LEAST_LOADED breaks a tie by the policy's order",
			strategy: work.AssignLeastLoaded,
			selection: withLoad(eligible(ada, ben, chris),
				map[shared.ID]int{ada: 2, ben: 1, chris: 1}),
			want: ben,
		},
		{
			name:     "LEAST_LOADED counts an absent account as unloaded",
			strategy: work.AssignLeastLoaded,
			selection: withLoad(eligible(ada, ben),
				map[shared.ID]int{ada: 1}),
			want: ben,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			strategy, err := service.StrategyFor(tc.strategy)
			if err != nil {
				t.Fatalf("StrategyFor(%s): %v", tc.strategy, err)
			}
			choice, ok := strategy.Choose(tc.selection)
			if !ok {
				t.Fatalf("Choose refused a selection with candidates")
			}
			if choice.AccountID != tc.want {
				t.Fatalf("chose %s, want %s", choice.AccountID, tc.want)
			}
		})
	}
}

func TestAnEmptySelectionIsNobodyForEveryStrategy(t *testing.T) {
	for _, strategy := range work.AutoAssignStrategies() {
		t.Run(string(strategy), func(t *testing.T) {
			chooser, err := service.StrategyFor(strategy)
			if err != nil {
				t.Fatalf("StrategyFor(%s): %v", strategy, err)
			}
			// A random source that would panic if drawn from: an empty selection must be decided
			// before any draw.
			if _, ok := chooser.Choose(service.AssignmentSelection{}); ok {
				t.Fatalf("%s chose somebody from an empty selection", strategy)
			}
		})
	}
}

func TestTheRotationSkipsTheIneligibleWithoutLosingTheirPlace(t *testing.T) {
	strategy, err := service.StrategyFor(work.AssignRoundRobin)
	if err != nil {
		t.Fatal(err)
	}

	// Three configured candidates; the cursor stands on ben, but ben has lost access: only ada
	// and chris (positions 0 and 2) are eligible.
	selection := service.AssignmentSelection{
		Accounts:       []shared.ID{ada, chris},
		Positions:      []int{0, 2},
		CandidateCount: 3,
		Cursor:         1,
	}

	choice, ok := strategy.Choose(selection)
	if !ok {
		t.Fatal("the rotation found nobody although two candidates are eligible")
	}
	if choice.AccountID != chris {
		t.Fatalf("chose %s, want %s - the walk starts at the cursor, not at the head", choice.AccountID, chris)
	}
	if !choice.Advanced || choice.NextCursor != 0 {
		t.Fatalf("cursor after the pick: advanced=%v next=%d, want advanced past chris to 0",
			choice.Advanced, choice.NextCursor)
	}

	// The next turn, with ben back: the cursor stands at 0, so ada is picked - ben lost the one
	// turn they were ineligible for, not their place in the order.
	next := service.AssignmentSelection{
		Accounts:       []shared.ID{ada, ben, chris},
		Positions:      []int{0, 1, 2},
		CandidateCount: 3,
		Cursor:         choice.NextCursor,
	}
	second, ok := strategy.Choose(next)
	if !ok || second.AccountID != ada {
		t.Fatalf("second turn chose %s, want %s", second.AccountID, ada)
	}
}

func TestACursorPastTheListIsFoldedNotTrusted(t *testing.T) {
	strategy, err := service.StrategyFor(work.AssignRoundRobin)
	if err != nil {
		t.Fatal(err)
	}

	// The policy was rewritten from five candidates to two while the cursor stood at 4.
	selection := withCursor(eligible(ada, ben), 4)
	choice, ok := strategy.Choose(selection)
	if !ok {
		t.Fatal("a stale cursor must not empty the rotation")
	}
	if choice.AccountID != ada || choice.NextCursor != 1 {
		t.Fatalf("chose %s with next=%d, want %s with next=1", choice.AccountID, choice.NextCursor, ada)
	}
}

func TestOnlyTheRotationAdvancesState(t *testing.T) {
	for _, strategy := range []work.AutoAssignStrategy{
		work.AssignFixed, work.AssignRandomMember, work.AssignLeastLoaded,
	} {
		chooser, err := service.StrategyFor(strategy)
		if err != nil {
			t.Fatal(err)
		}
		selection := withRandom(eligible(ada, ben), clock.NewScripted(0))
		choice, ok := chooser.Choose(selection)
		if !ok {
			t.Fatalf("%s found nobody", strategy)
		}
		if choice.Advanced {
			t.Fatalf("%s advanced state it does not keep", strategy)
		}
	}
}

func TestAnUnknownStrategyIsAnInternalError(t *testing.T) {
	if _, err := service.StrategyFor(work.AutoAssignStrategy("BOGUS")); err == nil {
		t.Fatal("an unknown strategy value must not resolve")
	}
}

func withRandom(s service.AssignmentSelection, r *clock.Scripted) service.AssignmentSelection {
	s.Random = r
	return s
}

func withCursor(s service.AssignmentSelection, cursor int) service.AssignmentSelection {
	s.Cursor = cursor
	return s
}

func withLoad(s service.AssignmentSelection, load map[shared.ID]int) service.AssignmentSelection {
	s.OpenItems = load
	return s
}
