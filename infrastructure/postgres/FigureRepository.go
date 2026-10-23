// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// FigureRepository counts collections' figures (db/queries/Figures.sql). One statement per kind on
// the page, never one per collection; row level security keeps each inside the transaction's
// workspace.
type FigureRepository struct{}

func NewFigureRepository() FigureRepository { return FigureRepository{} }

var _ repository.Figures = FigureRepository{}

// figureBatch is one statement's parameters: the collections asking for a kind, in one order.
type figureBatch struct {
	ids     []pgtype.UUID
	offered []string
	dated   []string
	keys    []string
}

func (b *figureBatch) add(request work.FigureRequest) error {
	id, err := uuidOf(request.CollectionID)
	if err != nil {
		return err
	}
	b.ids = append(b.ids, id)
	b.offered = append(b.offered, work.JoinTypes(request.Offered))
	b.dated = append(b.dated, work.JoinTypes(request.Dated))
	b.keys = append(b.keys, request.FieldKey)
	return nil
}

// Count answers what each request's kind needs.
func (r FigureRepository) Count(
	ctx context.Context, requests []work.FigureRequest, window work.FigureWindow,
) (map[shared.ID]work.FigureCounts, error) {
	counted := make(map[shared.ID]work.FigureCounts, len(requests))
	if len(requests) == 0 {
		return counted, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	batches := map[work.Figure]*figureBatch{}
	for _, request := range requests {
		kind := request.Kind
		if kind == work.FigureFieldSum {
			kind = work.FigureOpen // one statement answers both
		}
		if batches[kind] == nil {
			batches[kind] = &figureBatch{}
		}
		if err := batches[kind].add(request); err != nil {
			return nil, err
		}
	}

	update := func(id pgtype.UUID, change func(*work.FigureCounts)) error {
		collectionID, err := idFrom(id)
		if err != nil {
			return err
		}
		counts := counted[collectionID]
		change(&counts)
		counted[collectionID] = counts
		return nil
	}
	failed := func(kind string, err error) error {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the %s figure of %d collections: %w", kind, len(requests), err))
	}

	if batch := batches[work.FigureOpen]; batch != nil {
		rows, err := queries.CountFigureOpen(ctx, sqlc.CountFigureOpenParams{CollectionIds: batch.ids, FieldKeys: batch.keys})
		if err != nil {
			return nil, failed("open", err)
		}
		for _, row := range rows {
			err := update(row.CollectionID, func(c *work.FigureCounts) {
				c.Open, c.Summed = int(row.Open), int(row.Summed)
				if row.Live {
					sum := row.Sum
					c.Sum = &sum
				}
			})
			if err != nil {
				return nil, err
			}
		}
	}
	if batch := batches[work.FigureDue]; batch != nil {
		rows, err := queries.CountFigureDue(ctx, sqlc.CountFigureDueParams{
			CollectionIds: batch.ids, Offered: batch.offered, Dated: batch.dated,
			Today: window.Today, DayEnd: timestampOf(window.DayEnd),
		})
		if err != nil {
			return nil, failed("due", err)
		}
		for _, row := range rows {
			if err := update(row.CollectionID, func(c *work.FigureCounts) { c.Due = int(row.Due) }); err != nil {
				return nil, err
			}
		}
	}
	if batch := batches[work.FigureRhythm]; batch != nil {
		rows, err := queries.CountFigureDays(ctx, sqlc.CountFigureDaysParams{
			Zone: window.Location.String(), CollectionIds: batch.ids, Offered: batch.offered,
			WeekStart: timestampOf(window.WeekStart), Now: timestampOf(window.Now),
		})
		if err != nil {
			return nil, failed("rhythm", err)
		}
		for _, row := range rows {
			if err := update(row.CollectionID, func(c *work.FigureCounts) { c.Days = row.Days }); err != nil {
				return nil, err
			}
		}
	}
	if batch := batches[work.FigureProgress]; batch != nil {
		rows, err := queries.CountFigureProgress(ctx, sqlc.CountFigureProgressParams{
			CollectionIds: batch.ids, Offered: batch.offered,
		})
		if err != nil {
			return nil, failed("progress", err)
		}
		for _, row := range rows {
			err := update(row.CollectionID, func(c *work.FigureCounts) { c.Done, c.Total = int(row.Done), int(row.Total) })
			if err != nil {
				return nil, err
			}
		}
	}
	if batch := batches[work.FigureFlow]; batch != nil {
		rows, err := queries.CountFigureFlow(ctx, sqlc.CountFigureFlowParams{
			CollectionIds: batch.ids, Offered: batch.offered,
			Since: timestampOf(window.FlowSince()), Now: timestampOf(window.Now),
		})
		if err != nil {
			return nil, failed("flow", err)
		}
		for _, row := range rows {
			err := update(row.CollectionID, func(c *work.FigureCounts) {
				c.Completed, c.OverLimit = int(row.Completed), int(row.OverLimit)
				if row.Timed > 0 {
					average := row.AverageSeconds
					c.AverageSeconds = &average
				}
			})
			if err != nil {
				return nil, err
			}
		}
	}
	return counted, nil
}
