// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PrivateHubRepository answers what an administrator may know of private hubs, and how many
// people the workspace has.
type PrivateHubRepository struct{}

func NewPrivateHubRepository() PrivateHubRepository { return PrivateHubRepository{} }

var (
	_ repository.PrivateHubs  = PrivateHubRepository{}
	_ identityrepo.People     = PrivateHubRepository{}
	_ repository.OrphanedHubs = PrivateHubRepository{}
)

// ListPrivateHubs lists the private hubs of the transaction's tenant; row level security bounds it
// (ADR-0010).
func (r PrivateHubRepository) ListPrivateHubs(ctx context.Context) ([]repository.PrivateHubSummary, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListPrivateHubs(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the private hubs: %w", err))
	}

	hubs := make([]repository.PrivateHubSummary, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		owners := make([]shared.ID, 0, len(row.Owners))
		for _, owner := range row.Owners {
			ownerID, err := idFrom(owner)
			if err != nil {
				return nil, err
			}
			owners = append(owners, ownerID)
		}
		hubs = append(hubs, repository.PrivateHubSummary{
			ID: id, Owners: owners,
			Collections: int(row.Collections), Entries: int(row.Entries),
			AttachmentBytes: row.AttachmentBytes,
			CreatedAt:       timeFrom(row.CreatedAt),
			TrashedAt:       optionalTime(row.DeletedAt),
		})
	}
	return hubs, nil
}

// CountPeople counts the persons of the transaction's tenant that can still act.
func (r PrivateHubRepository) CountPeople(ctx context.Context) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountPeople(ctx)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the people: %w", err))
	}
	return int(count), nil
}

// Orphaned answers the private hubs without a member among those the named rows sit under; nil
// names every private hub of the transaction's tenant.
func (r PrivateHubRepository) Orphaned(ctx context.Context, named []shared.ID) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := uuidsOf(named)
	if err != nil {
		return nil, err
	}
	rows, err := queries.OrphanedPrivateHubs(ctx, keys)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the private hubs without a member: %w", err))
	}
	return idsFrom(rows)
}

// Lock takes the hubs' rows for the rest of the transaction.
func (r PrivateHubRepository) Lock(ctx context.Context, hubIDs []shared.ID) error {
	if len(hubIDs) == 0 {
		return nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	keys, err := uuidsOf(hubIDs)
	if err != nil {
		return err
	}
	if err := queries.LockContainers(ctx, keys); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("locking the private hubs: %w", err))
	}
	return nil
}

// HeldBy answers the private hubs the account is a member of.
func (r PrivateHubRepository) HeldBy(ctx context.Context, accountID shared.ID) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PrivateHubsHeldBy(ctx, key)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the private hubs of an account: %w", err))
	}
	return idsFrom(rows)
}

// WorkspaceOwners answers the persons holding OWNER on the workspace itself.
func (r PrivateHubRepository) WorkspaceOwners(ctx context.Context) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.WorkspaceOwners(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the workspace's owners: %w", err))
	}
	return idsFrom(rows)
}
