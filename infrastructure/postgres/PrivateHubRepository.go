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
	_ repository.PrivateHubs = PrivateHubRepository{}
	_ identityrepo.People    = PrivateHubRepository{}
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
