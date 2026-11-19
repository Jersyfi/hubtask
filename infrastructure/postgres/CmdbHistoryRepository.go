// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cmdbrepo "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// CmdbHistoryRepository keeps an inventory object's own history (migration 0435).
type CmdbHistoryRepository struct {
	cursors security.CursorCodec
}

// NewCmdbHistoryRepository builds the repository with the codec its pages' cursors are signed by.
func NewCmdbHistoryRepository(cursors security.CursorCodec) CmdbHistoryRepository {
	return CmdbHistoryRepository{cursors: cursors}
}

var _ cmdbrepo.History = CmdbHistoryRepository{}

// movedJSON is one field of the change set as the contract spells it (schema CiHistoryStep).
type movedJSON struct {
	From    *string `json:"from,omitempty"`
	To      *string `json:"to,omitempty"`
	Changed bool    `json:"changed,omitempty"`
}

// AppendCIHistory writes one step in the caller's transaction.
func (r CmdbHistoryRepository) AppendCIHistory(ctx context.Context, step cmdb.Step) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(step.ID)
	if err != nil {
		return err
	}
	ci, err := uuidOf(step.CIID)
	if err != nil {
		return err
	}
	actor, err := optionalUUID(step.ActorID)
	if err != nil {
		return err
	}
	set := make(map[string]movedJSON, len(step.Changes))
	for field, moved := range step.Changes {
		var out movedJSON
		if moved.From != "" {
			out.From = &moved.From
		}
		if moved.To != "" {
			out.To = &moved.To
		}
		out.Changed = moved.Changed
		set[field] = out
	}
	changeSet, err := json.Marshal(set)
	if err != nil {
		return shared.ErrInternal.WithDetail("cmdb.history_step_invalid").WithCause(err)
	}
	if err := queries.AppendCiHistory(ctx, sqlc.AppendCiHistoryParams{
		ID: id, CiID: ci, ActorType: string(step.ActorKind), ActorID: actor, Verb: string(step.Verb),
		ChangeSet: changeSet, OccurredAt: timestampOf(step.At),
	}); err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the history of %s: %w", step.CIID, err))
	}
	return nil
}

// CIHistory returns one page of an object's history, newest first.
func (r CmdbHistoryRepository) CIHistory(ctx context.Context, ciID shared.ID, page workrepo.Page) (cmdbrepo.HistoryPage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return cmdbrepo.HistoryPage{}, err
	}
	ci, err := uuidOf(ciID)
	if err != nil {
		return cmdbrepo.HistoryPage{}, err
	}
	boundary, err := activityCursor(r.cursors, page.Cursor)
	if err != nil {
		return cmdbrepo.HistoryPage{}, err
	}
	rows, err := queries.ListCiHistory(ctx, sqlc.ListCiHistoryParams{
		CiID: ci, CursorOccurredAt: boundary.occurredAt, CursorID: boundary.id, PageSize: pageProbe(page.Size),
	})
	if err != nil {
		return cmdbrepo.HistoryPage{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the history of %s: %w", ciID, err))
	}
	steps := make([]cmdb.Step, 0, len(rows))
	for _, row := range rows {
		step, err := stepFrom(row)
		if err != nil {
			return cmdbrepo.HistoryPage{}, err
		}
		steps = append(steps, step)
	}
	kept, info := pageOf(steps, page.Size, r.cursors, func(step cmdb.Step) security.Position {
		return security.At(step.At.UTC().Format(time.RFC3339Nano), step.ID)
	})
	return cmdbrepo.HistoryPage{Steps: kept, Info: workrepo.PageInfo{NextCursor: info.NextCursor, HasMore: info.HasMore}}, nil
}

func stepFrom(row sqlc.ListCiHistoryRow) (cmdb.Step, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return cmdb.Step{}, err
	}
	ci, err := idFrom(row.CiID)
	if err != nil {
		return cmdb.Step{}, err
	}
	actor, err := optionalID(row.ActorID)
	if err != nil {
		return cmdb.Step{}, err
	}
	var set map[string]movedJSON
	if err := json.Unmarshal(row.ChangeSet, &set); err != nil {
		return cmdb.Step{}, shared.ErrInternal.WithDetail("postgres.row_incoherent").WithCause(err)
	}
	changes := make(map[string]cmdb.Moved, len(set))
	for field, moved := range set {
		var out cmdb.Moved
		if moved.From != nil {
			out.From = *moved.From
		}
		if moved.To != nil {
			out.To = *moved.To
		}
		out.Changed = moved.Changed
		changes[field] = out
	}
	return cmdb.Step{
		ID: id, CIID: ci, ActorKind: shared.ActorKind(row.ActorType), ActorID: actor,
		Verb: cmdb.Verb(row.Verb), Changes: changes, At: timeFrom(row.OccurredAt),
	}, nil
}
