// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// MovePreviewRepository counts what a collection's move leaves at the hub it leaves, inside the
// caller's transaction (rule 3).
type MovePreviewRepository struct{}

var _ repository.MoveLeftovers = MovePreviewRepository{}

// KeptReleaseEntries counts the collection's entries held by a release of the hub.
func (MovePreviewRepository) KeptReleaseEntries(ctx context.Context, collectionID, hubID shared.ID) (int, error) {
	queries, collection, hub, err := leftoverArgs(ctx, collectionID, hubID)
	if err != nil {
		return 0, err
	}
	count, err := queries.MoveKeptReleaseEntries(ctx, sqlc.MoveKeptReleaseEntriesParams{CollectionID: collection, HubID: hub})
	return leftovers("release entries", count, err)
}

// KeptSLAClocks counts the collection's running or paused clocks measured by a policy of the hub.
func (MovePreviewRepository) KeptSLAClocks(ctx context.Context, collectionID, hubID shared.ID) (int, error) {
	queries, collection, hub, err := leftoverArgs(ctx, collectionID, hubID)
	if err != nil {
		return 0, err
	}
	count, err := queries.MoveKeptSLAClocks(ctx, sqlc.MoveKeptSLAClocksParams{CollectionID: collection, HubID: hub})
	return leftovers("clocks", count, err)
}

func leftoverArgs(ctx context.Context, collectionID, hubID shared.ID) (*sqlc.Queries, pgtype.UUID, pgtype.UUID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, pgtype.UUID{}, pgtype.UUID{}, err
	}
	collection, err := uuidOf(collectionID)
	if err != nil {
		return nil, pgtype.UUID{}, pgtype.UUID{}, err
	}
	hub, err := uuidOf(hubID)
	return queries, collection, hub, err
}

func leftovers(what string, count int32, err error) (int, error) {
	if err != nil {
		return 0, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the %s a move leaves: %w", what, err))
	}
	return int(count), nil
}
