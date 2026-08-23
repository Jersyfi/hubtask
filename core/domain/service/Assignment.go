// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package service

import (
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// AssignmentStrategy is the port arc42 §5.3 names: given the material a policy's pool resolves
// to, pick who gets the entry. Five implementations, one per value of work.AutoAssignStrategy,
// and extension is a sixth - no strategy touches WorkItem (domain-model.md §3.6).
//
// The application resolves the material - which candidates may actually receive the entry, how
// loaded they are, where the rotation stands - and the strategy only chooses. That split is what
// makes every strategy a table test: nothing in here reads a database or the wall clock, and the
// one source of chance is the RandomSource port.
type AssignmentStrategy interface {
	// Choose picks from the selection, or reports that nobody is eligible. A choice that
	// advanced the rotation says so, and the caller persists the new cursor in the same
	// transaction that assigns - hopeful writes are how two creates hand one turn to two people.
	Choose(in AssignmentSelection) (AssignmentChoice, bool)
}

// StrategyFor returns the implementation of one strategy value. An unknown value is an internal
// error rather than a validation error: the policy was validated when it was written, so a value
// no strategy answers to is a row this code no longer understands.
func StrategyFor(strategy work.AutoAssignStrategy) (AssignmentStrategy, error) {
	chosen, known := strategies[strategy]
	if !known {
		return nil, shared.ErrInternal.
			WithDetail("items.auto_assign_strategy_unhandled").
			WithParams(map[string]string{"strategy": string(strategy)})
	}
	return chosen, nil
}

var strategies = map[work.AutoAssignStrategy]AssignmentStrategy{
	work.AssignFixed:             fixedStrategy{},
	work.AssignRandomMember:      randomMemberStrategy{},
	work.AssignRandomGroupMember: randomGroupMemberStrategy{},
	work.AssignRoundRobin:        roundRobinStrategy{},
	work.AssignLeastLoaded:       leastLoadedStrategy{},
}

// AssignmentSelection is the material one choice is made from.
//
// Accounts is the policy's candidate accounts filtered to the eligible ones, in the policy's
// order; Positions maps each of them back to its index in the configured list, which is what the
// rotation walks - a candidate skipped while ineligible loses that one turn, not their place.
// Groups is the material of RANDOM_GROUP_MEMBER instead: each candidate group that still has an
// eligible member, with those members. OpenItems is the material of LEAST_LOADED: open entries
// per eligible account, an absent key meaning none.
type AssignmentSelection struct {
	Accounts  []shared.ID
	Positions []int
	// CandidateCount is the length of the configured candidate list, which the rotation's cursor
	// wraps around - the eligible subset may be shorter.
	CandidateCount int
	Cursor         int
	Groups         []AssignmentGroup
	OpenItems      map[shared.ID]int
	Random         clock.RandomSource
}

// AssignmentGroup is one candidate group with the members a draw may land on.
type AssignmentGroup struct {
	GroupID shared.ID
	Members []shared.ID
}

// AssignmentChoice is who was picked, and - when the strategy keeps state - where the rotation
// now stands.
type AssignmentChoice struct {
	AccountID shared.ID
	// NextCursor is the round-robin state to persist, meaningful only when Advanced.
	NextCursor int
	Advanced   bool
}

// fixedStrategy always picks the one configured account. Eligibility still applies: a fixed
// assignee who can no longer see the collection is nobody, not an assignment to a dead reference.
type fixedStrategy struct{}

func (fixedStrategy) Choose(in AssignmentSelection) (AssignmentChoice, bool) {
	if len(in.Accounts) == 0 {
		return AssignmentChoice{}, false
	}
	return AssignmentChoice{AccountID: in.Accounts[0]}, true
}

// randomMemberStrategy draws uniformly from the eligible accounts.
type randomMemberStrategy struct{}

func (randomMemberStrategy) Choose(in AssignmentSelection) (AssignmentChoice, bool) {
	if len(in.Accounts) == 0 {
		return AssignmentChoice{}, false
	}
	return AssignmentChoice{AccountID: in.Accounts[in.Random.IntN(len(in.Accounts))]}, true
}

// randomGroupMemberStrategy draws a group first and a member second, so that each group carries
// an equal share of the work regardless of its size. A group with no eligible member is out of
// the first draw entirely - a draw that could land on an empty group would either error or
// silently redraw, and both are worse than not offering the group.
type randomGroupMemberStrategy struct{}

func (randomGroupMemberStrategy) Choose(in AssignmentSelection) (AssignmentChoice, bool) {
	if len(in.Groups) == 0 {
		return AssignmentChoice{}, false
	}
	group := in.Groups[in.Random.IntN(len(in.Groups))]
	return AssignmentChoice{AccountID: group.Members[in.Random.IntN(len(group.Members))]}, true
}

// roundRobinStrategy hands out turns in the configured order. From the cursor, it walks the
// configured list circularly and picks the first candidate that is eligible; the cursor then
// stands one past the pick.
type roundRobinStrategy struct{}

func (roundRobinStrategy) Choose(in AssignmentSelection) (AssignmentChoice, bool) {
	if len(in.Accounts) == 0 || in.CandidateCount == 0 {
		return AssignmentChoice{}, false
	}

	// The cursor as stored may point past the list when the policy was rewritten with fewer
	// candidates; folding it here rather than on write keeps the stored state exactly what the
	// last assignment left.
	start := in.Cursor % in.CandidateCount

	// One circle over the configured positions, starting at the cursor. The eligible subset is
	// searched through Positions so the walk is over the configuration, not over eligibility.
	for offset := 0; offset < in.CandidateCount; offset++ {
		position := (start + offset) % in.CandidateCount
		for i, candidate := range in.Positions {
			if candidate == position {
				return AssignmentChoice{
					AccountID:  in.Accounts[i],
					NextCursor: (position + 1) % in.CandidateCount,
					Advanced:   true,
				}, true
			}
		}
	}
	return AssignmentChoice{}, false
}

// leastLoadedStrategy picks the eligible account with the fewest open entries. A tie goes to the
// earlier position in the policy's list - deterministic, so the strategy stays a pure function
// over counts (domain-model.md §3.6), and stable, so two ties in a row do not oscillate.
type leastLoadedStrategy struct{}

func (leastLoadedStrategy) Choose(in AssignmentSelection) (AssignmentChoice, bool) {
	if len(in.Accounts) == 0 {
		return AssignmentChoice{}, false
	}

	chosen := in.Accounts[0]
	lowest := in.OpenItems[chosen]
	for _, account := range in.Accounts[1:] {
		if load := in.OpenItems[account]; load < lowest {
			chosen, lowest = account, load
		}
	}
	return AssignmentChoice{AccountID: chosen}, true
}
