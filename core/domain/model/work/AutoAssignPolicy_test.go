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

func TestTheAutoAssignKeyParsesFromAnyChannel(t *testing.T) {
	parsed, err := work.ParseAutoAssignDefinition(map[string]any{
		"strategy": "ROUND_ROBIN",
		"candidates": []any{
			map[string]any{"kind": "ACCOUNT", "id": "00000000-0000-7000-8000-00000000000a"},
			map[string]any{"kind": "ACCOUNT", "id": "00000000-0000-7000-8000-00000000000b"},
		},
	})
	if err != nil {
		t.Fatalf("parsing a valid document: %v", err)
	}
	if parsed.Strategy != work.AssignRoundRobin || len(parsed.Candidates) != 2 {
		t.Fatalf("parsed %+v", parsed)
	}
	if !parsed.Enabled {
		t.Error("enabled defaults to true: a policy somebody configures should apply itself")
	}

	disabled, err := work.ParseAutoAssignDefinition(map[string]any{
		"strategy":   "FIXED",
		"candidates": []any{map[string]any{"kind": "ACCOUNT", "id": "00000000-0000-7000-8000-00000000000a"}},
		"enabled":    false,
	})
	if err != nil || disabled.Enabled {
		t.Fatalf("enabled: false did not survive the parse: %+v, %v", disabled, err)
	}
}

func TestTheAbsentAndTheNullKeyAreNoPolicy(t *testing.T) {
	if parsed, err := work.ParseAutoAssignDefinition(nil); err != nil || parsed != nil {
		t.Fatalf("null parsed to %+v, %v - want no policy and no error", parsed, err)
	}
}

func TestAMalformedAutoAssignKeyIsRefusedByName(t *testing.T) {
	cases := []struct {
		name  string
		value any
		code  string
	}{
		{"not an object", "ROUND_ROBIN", "usecase.field_type_invalid"},
		{"no strategy", map[string]any{"candidates": []any{}}, "containers.auto_assign_strategy_unknown"},
		{
			"no candidates list",
			map[string]any{"strategy": "FIXED"},
			"containers.auto_assign_candidates_required",
		},
		{
			"candidate not an object",
			map[string]any{"strategy": "FIXED", "candidates": []any{"ada"}},
			"usecase.field_type_invalid",
		},
		{
			"candidate id malformed",
			map[string]any{"strategy": "FIXED", "candidates": []any{
				map[string]any{"kind": "ACCOUNT", "id": "not-a-uuid"},
			}},
			"containers.auto_assign_candidate_id_required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := work.ParseAutoAssignDefinition(tc.value)
			if err == nil {
				t.Fatal("want a refusal, got none")
			}
			if got := shared.AsError(err).DetailCode; got != tc.code {
				t.Fatalf("want %s, got %s", tc.code, got)
			}
		})
	}
}

func TestReplacingThePoliciesMovesTheAutoAssignKey(t *testing.T) {
	collection := work.Container{
		ID: "c1", Type: work.ContainerCollection, CompletionPolicy: work.CompletionManual,
	}
	rotation := &work.AutoAssignDefinition{
		Strategy:   work.AssignRoundRobin,
		Candidates: []work.AutoAssignCandidate{account("a"), account("b")},
		Enabled:    true,
	}

	configured, changes, err := collection.WithPolicies(work.ContainerPolicies{
		CompletionPolicy: work.CompletionManual, AutoAssign: rotation,
	}, collection.UpdatedAt.Add(1))
	if err != nil {
		t.Fatalf("setting the key: %v", err)
	}
	if len(changes) != 1 || changes[0].Field != work.FieldAutoAssign ||
		changes[0].From != "" || changes[0].To != "ROUND_ROBIN" {
		t.Fatalf("changes %+v, want one auto_assign change to ROUND_ROBIN", changes)
	}
	if configured.AutoAssign == nil || configured.AutoAssign.Strategy != work.AssignRoundRobin {
		t.Fatalf("the container does not carry the key: %+v", configured.AutoAssign)
	}

	// The same document again is no change at all - and the version stays unspent upstream.
	_, changes, err = configured.WithPolicies(work.ContainerPolicies{
		CompletionPolicy: work.CompletionManual, AutoAssign: rotation,
	}, configured.UpdatedAt.Add(1))
	if err != nil || len(changes) != 0 {
		t.Fatalf("an identical document moved something: %+v, %v", changes, err)
	}

	// The key left out is the key removed: this is a PUT.
	removed, changes, err := configured.WithPolicies(work.ContainerPolicies{
		CompletionPolicy: work.CompletionManual,
	}, configured.UpdatedAt.Add(1))
	if err != nil {
		t.Fatalf("removing the key: %v", err)
	}
	if len(changes) != 1 || changes[0].From != "ROUND_ROBIN" || changes[0].To != "" {
		t.Fatalf("changes %+v, want one auto_assign change to absent", changes)
	}
	if removed.AutoAssign != nil {
		t.Fatal("the key survived its removal")
	}
}

func TestAnInvalidAutoAssignKeyNeverReachesTheDocument(t *testing.T) {
	collection := work.Container{ID: "c1", Type: work.ContainerCollection}

	_, _, err := collection.WithPolicies(work.ContainerPolicies{
		AutoAssign: &work.AutoAssignDefinition{
			Strategy:   work.AssignFixed,
			Candidates: []work.AutoAssignCandidate{account("a"), account("b")},
		},
	}, collection.UpdatedAt.Add(1))
	if err == nil {
		t.Fatal("FIXED with two candidates got through WithPolicies")
	}
	if got := shared.AsError(err).DetailCode; got != "containers.auto_assign_single_candidate_required" {
		t.Fatalf("refused as %s", got)
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
