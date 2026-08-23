// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"strconv"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// AutoAssignPolicy is a collection's rule for handing out what is created in it
// (domain-model.md §3.6).
//
// Its own row rather than a value inside the container's `policies` document, because two of the
// strategies are stateful in a way a document is not: ROUND_ROBIN advances a cursor that has to be
// locked against a concurrent create, and LEAST_LOADED is a query. The `autoAssign` key of the
// policies document is this row's representation in the contract; the row is the storage, so
// there is exactly one place a strategy and its state can disagree with themselves - nowhere.
type AutoAssignPolicy struct {
	ID       shared.ID
	TenantID shared.ID
	// ScopeType and ScopeID are where the policy applies. The schema also allows HUB, for the
	// milestone that gives hubs policies; C-02 writes and resolves COLLECTION only, because
	// UpdateContainerPolicies - the one writer of policy documents - refuses a hub today.
	ScopeType  AutoAssignScope
	ScopeID    shared.ID
	Strategy   AutoAssignStrategy
	Candidates []AutoAssignCandidate
	State      AutoAssignState
	// Enabled is the difference between the two ways a policy is reached (C-02): an enabled
	// policy applies itself to everything created in the collection, a disabled one waits to be
	// asked for with `auto_assign` on the create path. Removing the policy is deleting the row,
	// not disabling it.
	Enabled bool
	Version int
}

// AutoAssignScope is where a policy hangs.
type AutoAssignScope string

const (
	AutoAssignScopeCollection AutoAssignScope = "COLLECTION"
	AutoAssignScopeHub        AutoAssignScope = "HUB"
)

// AutoAssignStrategy is how a candidate is picked (domain-model.md §3.6).
type AutoAssignStrategy string

const (
	// AssignFixed always picks the one configured account. The fixed thing is a person; "always
	// the same group" is RANDOM_GROUP_MEMBER with a single group, because an assignee is an
	// account and a group can only ever reach the field through one of its members.
	AssignFixed AutoAssignStrategy = "FIXED"
	// AssignRandomMember draws uniformly from the candidate accounts.
	AssignRandomMember AutoAssignStrategy = "RANDOM_MEMBER"
	// AssignRandomGroupMember draws a group, then one of its members - so a small team and a
	// large one carry the same share of the work, not the same share per person.
	AssignRandomGroupMember AutoAssignStrategy = "RANDOM_GROUP_MEMBER"
	// AssignRoundRobin walks the candidate list in order, remembering where it stands in State.
	AssignRoundRobin AutoAssignStrategy = "ROUND_ROBIN"
	// AssignLeastLoaded picks the candidate with the fewest open entries.
	AssignLeastLoaded AutoAssignStrategy = "LEAST_LOADED"
)

// autoAssignStrategies is the closed set, in the order domain-model.md §3.6 lists them.
var autoAssignStrategies = [...]AutoAssignStrategy{
	AssignFixed, AssignRandomMember, AssignRandomGroupMember, AssignRoundRobin, AssignLeastLoaded,
}

// AutoAssignStrategies returns every defined strategy, for the same reason CompletionPolicies
// does: a client's form is configured from the installation, not from a copy of this list.
func AutoAssignStrategies() []AutoAssignStrategy { return autoAssignStrategies[:] }

// Valid reports whether the strategy is one of the defined ones.
func (s AutoAssignStrategy) Valid() bool {
	for _, known := range autoAssignStrategies {
		if known == s {
			return true
		}
	}
	return false
}

// ParseAutoAssignStrategy reads a stored or submitted strategy. No default: a policy without a
// strategy is not a policy, so the empty value is as unknown as a misspelled one.
func ParseAutoAssignStrategy(value string) (AutoAssignStrategy, error) {
	strategy := AutoAssignStrategy(value)
	if !strategy.Valid() {
		return "", shared.ErrValidation.
			WithDetail("containers.auto_assign_strategy_unknown").
			WithParams(map[string]string{"value": value}).
			WithFields(shared.FieldError{
				Path: "/policies/auto_assign/strategy", Code: "containers.auto_assign_strategy_unknown",
			})
	}
	return strategy, nil
}

// AutoAssignCandidate is one entry of the pool: an account, or - for RANDOM_GROUP_MEMBER - a
// group whose members are resolved when the draw happens, not when the policy is written.
type AutoAssignCandidate struct {
	Kind CandidateKind
	ID   shared.ID
}

// CandidateKind says what a candidate identifier names.
type CandidateKind string

const (
	CandidateAccount CandidateKind = "ACCOUNT"
	CandidateGroup   CandidateKind = "GROUP"
)

// ParseCandidateKind reads a stored or submitted candidate kind.
func ParseCandidateKind(value string) (CandidateKind, error) {
	kind := CandidateKind(value)
	if kind != CandidateAccount && kind != CandidateGroup {
		return "", shared.ErrValidation.
			WithDetail("containers.auto_assign_candidate_kind_unknown").
			WithParams(map[string]string{"value": value}).
			WithFields(shared.FieldError{
				Path: "/policies/auto_assign/candidates",
				Code: "containers.auto_assign_candidate_kind_unknown",
			})
	}
	return kind, nil
}

// AutoAssignState is what ROUND_ROBIN remembers between assignments: the index into the
// configured candidate list whose turn is next. Explicitly visible (domain-model.md §3.6) - the
// state is data in the policy row, so a test and an operator read the same thing.
//
// The cursor indexes the configured list rather than the currently eligible subset, so that a
// candidate who is skipped while ineligible has lost nothing but that one turn: the rotation's
// order does not depend on who happened to be eligible when.
type AutoAssignState struct {
	Cursor int
}

// Validate checks what every strategy demands of its pool. Called where the policy document is
// written, so that a policy that cannot choose anybody is refused before it is stored rather
// than discovered by the create that consults it.
func (p AutoAssignPolicy) Validate() error {
	if !p.Strategy.Valid() {
		return shared.ErrValidation.
			WithDetail("containers.auto_assign_strategy_unknown").
			WithParams(map[string]string{"value": string(p.Strategy)}).
			WithFields(shared.FieldError{
				Path: "/policies/auto_assign/strategy", Code: "containers.auto_assign_strategy_unknown",
			})
	}
	if len(p.Candidates) == 0 {
		return shared.ErrValidation.
			WithDetail("containers.auto_assign_candidates_required").
			WithFields(shared.FieldError{
				Path: "/policies/auto_assign/candidates",
				Code: "containers.auto_assign_candidates_required",
			})
	}
	if p.Strategy == AssignFixed && len(p.Candidates) != 1 {
		// One candidate is the strategy: FIXED with a list would leave the extra names as dead
		// configuration that reads as if it did something.
		return shared.ErrValidation.
			WithDetail("containers.auto_assign_single_candidate_required").
			WithParams(map[string]string{"count": strconv.Itoa(len(p.Candidates))}).
			WithFields(shared.FieldError{
				Path: "/policies/auto_assign/candidates",
				Code: "containers.auto_assign_single_candidate_required",
			})
	}

	for _, candidate := range p.Candidates {
		if candidate.ID.IsZero() {
			return shared.ErrValidation.
				WithDetail("containers.auto_assign_candidate_id_required").
				WithFields(shared.FieldError{
					Path: "/policies/auto_assign/candidates",
					Code: "containers.auto_assign_candidate_id_required",
				})
		}
		if candidate.Kind != p.Strategy.candidateKind() {
			// Each strategy draws from one kind of pool: groups exactly where §3.6 names them,
			// accounts everywhere else. A mixed list would make "no eligible candidate" ambiguous
			// about which half it means.
			return shared.ErrValidation.
				WithDetail("containers.auto_assign_candidate_kind_invalid").
				WithParams(map[string]string{
					"strategy": string(p.Strategy), "kind": string(candidate.Kind),
				}).
				WithFields(shared.FieldError{
					Path: "/policies/auto_assign/candidates",
					Code: "containers.auto_assign_candidate_kind_invalid",
				})
		}
	}
	return nil
}

// candidateKind is which kind of pool the strategy draws from.
func (s AutoAssignStrategy) candidateKind() CandidateKind {
	if s == AssignRandomGroupMember {
		return CandidateGroup
	}
	return CandidateAccount
}
