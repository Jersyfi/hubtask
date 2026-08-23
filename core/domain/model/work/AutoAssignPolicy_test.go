// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

func account(id string) work.AutoAssignCandidate {
	return work.AutoAssignCandidate{Kind: work.CandidateAccount, ID: shared.ID(id)}
}

func group(id string) work.AutoAssignCandidate {
	return work.AutoAssignCandidate{Kind: work.CandidateGroup, ID: shared.ID(id)}
}

func TestWhatEachStrategyDemandsOfItsPool(t *testing.T) {
	cases := []struct {
		name       string
		strategy   work.AutoAssignStrategy
		candidates []work.AutoAssignCandidate
		wantCode   string
	}{
		{
			name:     "FIXED takes exactly one account",
			strategy: work.AssignFixed, candidates: []work.AutoAssignCandidate{account("a")},
		},
		{
			name:       "FIXED with a list is refused",
			strategy:   work.AssignFixed,
			candidates: []work.AutoAssignCandidate{account("a"), account("b")},
			wantCode:   "containers.auto_assign_single_candidate_required",
		},
		{
			name:     "FIXED with a group is refused - an assignee is an account",
			strategy: work.AssignFixed, candidates: []work.AutoAssignCandidate{group("g")},
			wantCode: "containers.auto_assign_candidate_kind_invalid",
		},
		{
			name:       "RANDOM_MEMBER takes accounts",
			strategy:   work.AssignRandomMember,
			candidates: []work.AutoAssignCandidate{account("a"), account("b")},
		},
		{
			name:       "RANDOM_MEMBER refuses a group in the list",
			strategy:   work.AssignRandomMember,
			candidates: []work.AutoAssignCandidate{account("a"), group("g")},
			wantCode:   "containers.auto_assign_candidate_kind_invalid",
		},
		{
			name:       "RANDOM_GROUP_MEMBER takes groups",
			strategy:   work.AssignRandomGroupMember,
			candidates: []work.AutoAssignCandidate{group("g1"), group("g2")},
		},
		{
			name:       "RANDOM_GROUP_MEMBER refuses an account in the list",
			strategy:   work.AssignRandomGroupMember,
			candidates: []work.AutoAssignCandidate{account("a")},
			wantCode:   "containers.auto_assign_candidate_kind_invalid",
		},
		{
			name:       "ROUND_ROBIN takes accounts",
			strategy:   work.AssignRoundRobin,
			candidates: []work.AutoAssignCandidate{account("a"), account("b")},
		},
		{
			name:       "LEAST_LOADED takes accounts",
			strategy:   work.AssignLeastLoaded,
			candidates: []work.AutoAssignCandidate{account("a"), account("b")},
		},
		{
			name:     "no candidates is no policy",
			strategy: work.AssignRandomMember,
			wantCode: "containers.auto_assign_candidates_required",
		},
		{
			name:       "an unknown strategy is refused by name",
			strategy:   work.AutoAssignStrategy("LOTTERY"),
			candidates: []work.AutoAssignCandidate{account("a")},
			wantCode:   "containers.auto_assign_strategy_unknown",
		},
		{
			name:       "a candidate without an identifier is refused",
			strategy:   work.AssignRandomMember,
			candidates: []work.AutoAssignCandidate{{Kind: work.CandidateAccount}},
			wantCode:   "containers.auto_assign_candidate_id_required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := work.AutoAssignPolicy{Strategy: tc.strategy, Candidates: tc.candidates}
			err := policy.Validate()

			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("valid policy refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want refusal %s, got none", tc.wantCode)
			}
			if got := shared.AsError(err).DetailCode; got != tc.wantCode {
				t.Fatalf("want %s, got %s", tc.wantCode, got)
			}
		})
	}
}

func TestStrategyAndCandidateKindParseByName(t *testing.T) {
	if _, err := work.ParseAutoAssignStrategy("ROUND_ROBIN"); err != nil {
		t.Fatalf("known strategy refused: %v", err)
	}
	if _, err := work.ParseAutoAssignStrategy(""); err == nil {
		t.Fatal("an empty strategy is not a policy and must not parse")
	}
	if _, err := work.ParseCandidateKind("GROUP"); err != nil {
		t.Fatalf("known kind refused: %v", err)
	}
	if _, err := work.ParseCandidateKind("TEAM"); err == nil {
		t.Fatal("an unknown kind must not parse")
	}
}
