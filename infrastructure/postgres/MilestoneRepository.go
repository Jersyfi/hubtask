// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// MilestoneRepository records and reads when a collection or hub was last finished
// (db/queries/Milestones.sql).
type MilestoneRepository struct{}

func NewMilestoneRepository() MilestoneRepository { return MilestoneRepository{} }

var _ repository.Milestones = MilestoneRepository{}

func milestoneFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// LockHub takes the hub's milestone lock for the rest of the transaction.
func (MilestoneRepository) LockHub(ctx context.Context, hubID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(hubID)
	if err != nil {
		return err
	}
	if err := queries.LockHubMilestone(ctx, id); err != nil {
		return milestoneFailed("locking a hub's milestone", err)
	}
	return nil
}

// RecordCollection records the collection's milestone when nothing in it is open.
func (MilestoneRepository) RecordCollection(ctx context.Context, collectionID shared.ID, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(collectionID)
	if err != nil {
		return false, err
	}
	rows, err := queries.RecordCollectionMilestone(ctx, sqlc.RecordCollectionMilestoneParams{
		CollectionID: id, ReachedAt: timestampOf(at),
	})
	if err != nil {
		return false, milestoneFailed("recording a collection's milestone", err)
	}
	return rows > 0, nil
}

// RecordHub records the hub's milestone when nothing in any of its collections is open.
func (MilestoneRepository) RecordHub(ctx context.Context, hubID shared.ID, at time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(hubID)
	if err != nil {
		return false, err
	}
	rows, err := queries.RecordHubMilestone(ctx, sqlc.RecordHubMilestoneParams{HubID: id, ReachedAt: timestampOf(at)})
	if err != nil {
		return false, milestoneFailed("recording a hub's milestone", err)
	}
	return rows > 0, nil
}

// Of answers the milestones of the given containers.
func (MilestoneRepository) Of(ctx context.Context, containerIDs []shared.ID) (map[shared.ID]time.Time, error) {
	found := make(map[shared.ID]time.Time, len(containerIDs))
	if len(containerIDs) == 0 {
		return found, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	ids, err := uuidsOf(containerIDs)
	if err != nil {
		return nil, err
	}
	rows, err := queries.MilestonesOf(ctx, ids)
	if err != nil {
		return nil, milestoneFailed("reading milestones", err)
	}
	for _, row := range rows {
		id, err := idFrom(row.ContainerID)
		if err != nil {
			return nil, err
		}
		found[id] = timeFrom(row.ReachedAt)
	}
	return found, nil
}
