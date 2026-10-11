// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// HubLockRepository takes a hub's row for the writes that must not interleave on a private hub.
type HubLockRepository struct{}

func NewHubLockRepository() HubLockRepository { return HubLockRepository{} }

var _ repository.HubLocks = HubLockRepository{}

// LockHubOf locks and answers the hub the identifier sits under. The tenant is the transaction's:
// row level security hides another tenant's rows, which then answer as not found (ADR-0010).
func (r HubLockRepository) LockHubOf(ctx context.Context, id shared.ID) (repository.Hub, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Hub{}, false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return repository.Hub{}, false, err
	}

	row, err := queries.LockHubOf(ctx, key)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return repository.Hub{}, false, nil
	case err != nil:
		return repository.Hub{}, false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("locking the hub of %s: %w", id, err))
	}
	hubID, err := idFrom(row.ID)
	if err != nil {
		return repository.Hub{}, false, err
	}
	return repository.Hub{ID: hubID, Private: row.Private}, true, nil
}

// GroupHoldsRoleUnder answers whether a group holds a role on the hub or anywhere below it.
func (r HubLockRepository) GroupHoldsRoleUnder(ctx context.Context, hubID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(hubID)
	if err != nil {
		return false, err
	}
	held, err := queries.GroupHoldsRoleUnderHub(ctx, key)
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the groups under %s: %w", hubID, err))
	}
	return held, nil
}
