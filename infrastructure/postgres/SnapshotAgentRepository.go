// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// The walk's registry rows (offline-sync.md §3.1), read as the registry's own repository maps them.

// AgentModels pages the registry's models.
func (SnapshotRepository) AgentModels(ctx context.Context, after shared.ID, batch int) ([]domain.Model, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotAgentModels(ctx, sqlc.SnapshotAgentModelsParams{After: key, Batch: int32(batch)}) //nolint:gosec // bounded by the contract's page maximum
	if err != nil {
		return nil, snapshotFailed("agent models", err)
	}
	return mapRows(rows, func(r sqlc.SnapshotAgentModelsRow) (domain.Model, error) { return modelOf(sqlc.FindAgentModelRow(r)) })
}

// Agents pages the registry's agents.
func (SnapshotRepository) Agents(ctx context.Context, after shared.ID, batch int) ([]domain.Agent, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotAgents(ctx, sqlc.SnapshotAgentsParams{After: key, Batch: int32(batch)}) //nolint:gosec // bounded by the contract's page maximum
	if err != nil {
		return nil, snapshotFailed("agents", err)
	}
	return mapRows(rows, func(r sqlc.SnapshotAgentsRow) (domain.Agent, error) { return agentOf(sqlc.FindAgentRow(r)) })
}

// AgentRequests pages the held acts with their agent's owner.
func (SnapshotRepository) AgentRequests(ctx context.Context, after shared.ID, batch int) ([]repository.HeldActOf, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotAgentRequests(ctx, sqlc.SnapshotAgentRequestsParams{After: key, Batch: int32(batch)}) //nolint:gosec // bounded by the contract's page maximum
	if err != nil {
		return nil, snapshotFailed("held acts", err)
	}
	return mapRows(rows, func(r sqlc.SnapshotAgentRequestsRow) (repository.HeldActOf, error) {
		request, err := requestOf(sqlc.FindAgentRequestRow{
			ID: r.ID, AgentID: r.AgentID, UseCase: r.UseCase, Class: r.Class, Input: r.Input, Status: r.Status,
			DecidedBy: r.DecidedBy, DecidedAt: r.DecidedAt, Code: r.Code, CreatedAt: r.CreatedAt, Version: r.Version,
		})
		if err != nil {
			return repository.HeldActOf{}, err
		}
		owner, err := optionalID(r.OwnerID)
		return repository.HeldActOf{Request: request, OwnerID: owner}, err
	})
}

// AgentCases pages the evaluation cases.
func (SnapshotRepository) AgentCases(ctx context.Context, after shared.ID, batch int) ([]domain.Case, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotAgentCases(ctx, sqlc.SnapshotAgentCasesParams{After: key, Batch: int32(batch)}) //nolint:gosec // bounded by the contract's page maximum
	if err != nil {
		return nil, snapshotFailed("evaluation cases", err)
	}
	return mapRows(rows, func(r sqlc.SnapshotAgentCasesRow) (domain.Case, error) {
		keys, err := ids(r.ID, r.AgentID, r.CreatedBy)
		if err != nil {
			return domain.Case{}, err
		}
		var expected map[string]any
		if err := json.Unmarshal(r.Expected, &expected); err != nil {
			return domain.Case{}, shared.ErrInternal.WithDetail("agents.stored_shape").WithCause(err)
		}
		return domain.Case{
			ID: keys[0], AgentID: keys[1], Title: r.Title, Notes: r.Notes, Expected: expected, CreatedBy: keys[2],
			CreatedAt: timeFrom(r.CreatedAt),
		}, nil
	})
}

// AgentRuns pages the evaluation runs with their agent's owner.
func (SnapshotRepository) AgentRuns(ctx context.Context, after shared.ID, batch int) ([]repository.RunOf, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotAgentRuns(ctx, sqlc.SnapshotAgentRunsParams{After: key, Batch: int32(batch)}) //nolint:gosec // bounded by the contract's page maximum
	if err != nil {
		return nil, snapshotFailed("evaluation runs", err)
	}
	return mapRows(rows, func(r sqlc.SnapshotAgentRunsRow) (repository.RunOf, error) {
		keys, err := ids(r.ID, r.AgentID, r.RunBy, r.OwnerID)
		if err != nil {
			return repository.RunOf{}, err
		}
		return repository.RunOf{Run: domain.Run{
			ID: keys[0], AgentID: keys[1], ModelRevision: int(r.ModelRevision), Cases: int(r.Cases),
			Passed: int(r.Passed), By: keys[2], At: timeFrom(r.At),
		}, OwnerID: keys[3]}, nil
	})
}
