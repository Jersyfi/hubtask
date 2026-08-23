// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// AutoAssignPolicyRepository stores the assignment policy per scope (C-02).
//
// The candidates and the rotation's state travel as JSONB, in the shape the column has carried
// since 0001_init: `[{"kind": "ACCOUNT", "id": "…"}]` and `{"cursor": 0}`. The shapes are named
// here once, beside the only code that reads or writes them.
type AutoAssignPolicyRepository struct{}

var _ repository.AutoAssignPolicies = AutoAssignPolicyRepository{}

// storedCandidate is one entry of the candidates column.
type storedCandidate struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// storedState is the state column. Only the rotation writes into it today; an absent key is a
// rotation standing at the head, which is what a fresh policy's `{}` decodes to.
type storedState struct {
	Cursor int `json:"cursor"`
}

// FindForScope reads the policy at one scope, or ErrNotFound when none is configured there.
func (r AutoAssignPolicyRepository) FindForScope(
	ctx context.Context, scope work.AutoAssignScope, scopeID shared.ID,
) (work.AutoAssignPolicy, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.AutoAssignPolicy{}, err
	}
	id, err := uuidOf(scopeID)
	if err != nil {
		return work.AutoAssignPolicy{}, err
	}

	row, err := queries.FindAutoAssignPolicy(ctx, sqlc.FindAutoAssignPolicyParams{
		ScopeType: string(scope), ScopeID: id,
	})
	if err != nil {
		if IsNoRows(err) {
			return work.AutoAssignPolicy{}, shared.ErrNotFound
		}
		return work.AutoAssignPolicy{}, policyReadError(scopeID, err)
	}
	return policyOf(
		row.ID, row.TenantID, row.ScopeType, row.ScopeID,
		row.Strategy, row.Candidates, row.State, row.Enabled, row.Version,
	)
}

// Lock reads the same row and holds it for the rest of the transaction (the rotation's
// concurrency control - see the port).
func (r AutoAssignPolicyRepository) Lock(
	ctx context.Context, scope work.AutoAssignScope, scopeID shared.ID,
) (work.AutoAssignPolicy, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.AutoAssignPolicy{}, err
	}
	id, err := uuidOf(scopeID)
	if err != nil {
		return work.AutoAssignPolicy{}, err
	}

	row, err := queries.LockAutoAssignPolicy(ctx, sqlc.LockAutoAssignPolicyParams{
		ScopeType: string(scope), ScopeID: id,
	})
	if err != nil {
		if IsNoRows(err) {
			return work.AutoAssignPolicy{}, shared.ErrNotFound
		}
		return work.AutoAssignPolicy{}, policyReadError(scopeID, err)
	}
	return policyOf(
		row.ID, row.TenantID, row.ScopeType, row.ScopeID,
		row.Strategy, row.Candidates, row.State, row.Enabled, row.Version,
	)
}

// Upsert writes the whole definition, creating or replacing the scope's row.
func (r AutoAssignPolicyRepository) Upsert(
	ctx context.Context, policy work.AutoAssignPolicy,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(policy.ID)
	if err != nil {
		return err
	}
	scopeID, err := uuidOf(policy.ScopeID)
	if err != nil {
		return err
	}

	stored := make([]storedCandidate, 0, len(policy.Candidates))
	for _, candidate := range policy.Candidates {
		stored = append(stored, storedCandidate{
			Kind: string(candidate.Kind), ID: candidate.ID.String(),
		})
	}
	candidates, err := json.Marshal(stored)
	if err != nil {
		return shared.ErrInternal.
			WithDetail("postgres.payload_unserialisable").
			WithCause(fmt.Errorf("marshalling the candidates of %s: %w", policy.ScopeID, err))
	}

	err = queries.UpsertAutoAssignPolicy(ctx, sqlc.UpsertAutoAssignPolicyParams{
		ID:         id,
		ScopeType:  string(policy.ScopeType),
		ScopeID:    scopeID,
		Strategy:   string(policy.Strategy),
		Candidates: candidates,
		Enabled:    policy.Enabled,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the assignment policy of %s: %w", policy.ScopeID, err))
	}
	return nil
}

// Delete removes the scope's policy. Idempotent: removing what is not there is the state the
// caller asked for.
func (r AutoAssignPolicyRepository) Delete(
	ctx context.Context, scope work.AutoAssignScope, scopeID shared.ID,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(scopeID)
	if err != nil {
		return err
	}

	_, err = queries.DeleteAutoAssignPolicy(ctx, sqlc.DeleteAutoAssignPolicyParams{
		ScopeType: string(scope), ScopeID: id,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("removing the assignment policy of %s: %w", scopeID, err))
	}
	return nil
}

// SaveState persists the advanced rotation, under the lock Lock took.
func (r AutoAssignPolicyRepository) SaveState(
	ctx context.Context, policy work.AutoAssignPolicy,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(policy.ID)
	if err != nil {
		return err
	}
	state, err := json.Marshal(storedState{Cursor: policy.State.Cursor})
	if err != nil {
		return shared.ErrInternal.
			WithDetail("postgres.payload_unserialisable").
			WithCause(fmt.Errorf("marshalling the rotation state of %s: %w", policy.ID, err))
	}

	affected, err := queries.SaveAutoAssignPolicyState(ctx, sqlc.SaveAutoAssignPolicyStateParams{
		State: state, ID: id,
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the rotation state of %s: %w", policy.ID, err))
	}
	if affected == 0 {
		// The caller holds the row locked, so a state write that matched nothing is this code
		// disagreeing with itself, not a request that can be fixed.
		return shared.ErrInternal.
			WithDetail("postgres.row_vanished").
			WithCause(fmt.Errorf("the locked assignment policy %s was not there to write", policy.ID))
	}
	return nil
}

// policyOf maps a stored row onto the domain's policy. One mapper for both selects - sqlc gives
// each statement a row type of its own, and two mappings would eventually disagree about a field.
func policyOf(
	id, tenantID pgtype.UUID, scopeType string, scopeID pgtype.UUID,
	strategy string, candidates, state []byte, enabled bool, version int32,
) (work.AutoAssignPolicy, error) {
	policyID, err := idFrom(id)
	if err != nil {
		return work.AutoAssignPolicy{}, err
	}
	tenant, err := idFrom(tenantID)
	if err != nil {
		return work.AutoAssignPolicy{}, err
	}
	scope, err := idFrom(scopeID)
	if err != nil {
		return work.AutoAssignPolicy{}, err
	}
	parsedStrategy, err := work.ParseAutoAssignStrategy(strategy)
	if err != nil {
		return work.AutoAssignPolicy{}, err
	}

	pool, err := candidatesFrom(candidates, policyID)
	if err != nil {
		return work.AutoAssignPolicy{}, err
	}

	var rotation storedState
	if err := json.Unmarshal(state, &rotation); err != nil {
		return work.AutoAssignPolicy{}, policyUnreadable(policyID, "state", err)
	}

	return work.AutoAssignPolicy{
		ID:         policyID,
		TenantID:   tenant,
		ScopeType:  work.AutoAssignScope(scopeType),
		ScopeID:    scope,
		Strategy:   parsedStrategy,
		Candidates: pool,
		State:      work.AutoAssignState{Cursor: rotation.Cursor},
		Enabled:    enabled,
		Version:    int(version),
	}, nil
}

// candidatesFrom reads the candidates column. The identifier in the error is whatever the caller
// has to name the row by - the policy for a policy read, the container for the joined read.
func candidatesFrom(raw []byte, owner shared.ID) ([]work.AutoAssignCandidate, error) {
	var storedCandidates []storedCandidate
	if err := json.Unmarshal(raw, &storedCandidates); err != nil {
		return nil, policyUnreadable(owner, "candidates", err)
	}
	pool := make([]work.AutoAssignCandidate, 0, len(storedCandidates))
	for _, candidate := range storedCandidates {
		kind, err := work.ParseCandidateKind(candidate.Kind)
		if err != nil {
			return nil, err
		}
		pool = append(pool, work.AutoAssignCandidate{Kind: kind, ID: shared.ID(candidate.ID)})
	}
	return pool, nil
}

// autoAssignDefinitionFrom reads the policy columns a container row carries from the LEFT JOIN:
// all NULL - read off the strategy - means no policy, which the domain spells nil.
func autoAssignDefinitionFrom(
	containerID shared.ID, strategy *string, candidates []byte, enabled *bool,
) (*work.AutoAssignDefinition, error) {
	if strategy == nil {
		return nil, nil
	}
	parsed, err := work.ParseAutoAssignStrategy(*strategy)
	if err != nil {
		return nil, err
	}
	pool, err := candidatesFrom(candidates, containerID)
	if err != nil {
		return nil, err
	}
	return &work.AutoAssignDefinition{
		Strategy:   parsed,
		Candidates: pool,
		Enabled:    enabled != nil && *enabled,
	}, nil
}

func policyReadError(scopeID shared.ID, err error) error {
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("reading the assignment policy of %s: %w", scopeID, err))
}

func policyUnreadable(policyID shared.ID, column string, err error) error {
	return shared.ErrInternal.
		WithDetail("postgres.payload_unreadable").
		WithCause(errors.Join(err, fmt.Errorf("the %s of assignment policy %s", column, policyID)))
}
