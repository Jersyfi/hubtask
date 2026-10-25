// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ItemTallyRepository counts what a page of entries holds - parts, comments, attachments - in one
// statement (TallyItems in db/queries/Structure.sql). It reads within the caller's unit of work, as
// every repository here does, so the tenant's row-level security applies to the counts too.
type ItemTallyRepository struct{}

func NewItemTallyRepository() ItemTallyRepository { return ItemTallyRepository{} }

var _ repository.ItemTallies = ItemTallyRepository{}

// TallyFor answers every entry asked about that the reader can see.
func (r ItemTallyRepository) TallyFor(
	ctx context.Context, itemIDs []shared.ID,
) (map[shared.ID]repository.ItemTally, error) {
	tallies := map[shared.ID]repository.ItemTally{}
	if len(itemIDs) == 0 {
		return tallies, nil
	}

	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	keys := make([]pgtype.UUID, 0, len(itemIDs))
	for _, id := range itemIDs {
		key, err := uuidOf(id)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	rows, err := queries.TallyItems(ctx, keys)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting what %d entries hold: %w", len(itemIDs), err))
	}

	for _, row := range rows {
		itemID, err := idFrom(row.ItemID)
		if err != nil {
			return nil, err
		}
		tallies[itemID] = repository.ItemTally{
			Parts:       int(row.Parts),
			PartsDone:   int(row.PartsDone),
			Comments:    int(row.Comments),
			Attachments: int(row.Attachments),
		}
	}
	return tallies, nil
}
