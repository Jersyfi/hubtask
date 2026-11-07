// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// The agents' rows the retention sweep removes (AGENT_LEDGER, AGENT_REQUEST, AGENT_JOB): one
// remover per kind, each bounded to the transaction's tenant by row level security.

// AgentLedgerExpiry removes the ledger's acts past the period.
type AgentLedgerExpiry struct{}

// AgentRequestExpiry removes held acts past the period.
type AgentRequestExpiry struct{}

// AgentJobExpiry removes ended jobs past the period.
type AgentJobExpiry struct{}

func agentExpiryFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

func (AgentLedgerExpiry) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.DeleteExpiredAgentActions(ctx, sqlc.DeleteExpiredAgentActionsParams{Cutoff: timestampOf(cutoff), Batch: boundedInt32(batch)})
	if err != nil {
		return 0, agentExpiryFailed("removing expired agent acts", err)
	}
	return int(n), nil
}

func (AgentLedgerExpiry) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.CountExpiredAgentActions(ctx, sqlc.CountExpiredAgentActionsParams{Cutoff: timestampOf(cutoff), Ceiling: boundedInt32(ceiling)})
	if err != nil {
		return 0, agentExpiryFailed("counting expired agent acts", err)
	}
	return int(n), nil
}

func (AgentRequestExpiry) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.DeleteExpiredAgentRequests(ctx, sqlc.DeleteExpiredAgentRequestsParams{Cutoff: timestampOf(cutoff), Batch: boundedInt32(batch)})
	if err != nil {
		return 0, agentExpiryFailed("removing expired held agent acts", err)
	}
	return int(n), nil
}

func (AgentRequestExpiry) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.CountExpiredAgentRequests(ctx, sqlc.CountExpiredAgentRequestsParams{Cutoff: timestampOf(cutoff), Ceiling: boundedInt32(ceiling)})
	if err != nil {
		return 0, agentExpiryFailed("counting expired held agent acts", err)
	}
	return int(n), nil
}

func (AgentJobExpiry) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.DeleteExpiredAgentJobs(ctx, sqlc.DeleteExpiredAgentJobsParams{Cutoff: timestampOf(cutoff), Batch: boundedInt32(batch)})
	if err != nil {
		return 0, agentExpiryFailed("removing expired agent jobs", err)
	}
	return int(n), nil
}

func (AgentJobExpiry) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.CountExpiredAgentJobs(ctx, sqlc.CountExpiredAgentJobsParams{Cutoff: timestampOf(cutoff), Ceiling: boundedInt32(ceiling)})
	if err != nil {
		return 0, agentExpiryFailed("counting expired agent jobs", err)
	}
	return int(n), nil
}
