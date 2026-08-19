// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// LifecycleRepository is the end of data's life: the instructions not to delete, and the record of
// what was deleted anyway (ADR-0020).
//
// Nothing here names a tenant. The transaction the caller opened decided that, and row level
// security applies it to every statement below - which matters more here than anywhere else, because
// a legal hold read across the wrong boundary would not be a wrong answer but somebody else's
// obligation ignored.
type LifecycleRepository struct{}

func NewLifecycleRepository() LifecycleRepository { return LifecycleRepository{} }

var (
	_ repository.LegalHolds = LifecycleRepository{}
	_ repository.Removals   = LifecycleRepository{}
)

// Active returns the holds in force for this tenant.
func (r LifecycleRepository) Active(ctx context.Context) (domain.Holds, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ActiveLegalHolds(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the legal holds: %w", err))
	}

	holds := make(domain.Holds, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		// A tenant-wide hold names nothing, which is a NULL rather than a missing row.
		scopeID, err := optionalID(row.ScopeID)
		if err != nil {
			return nil, err
		}
		holds = append(holds, domain.LegalHold{
			ID: id, Scope: domain.HoldScope(row.ScopeKind), ScopeID: scopeID,
			Reason: row.Reason, PlacedAt: timeFrom(row.PlacedAt),
		})
	}
	return holds, nil
}

// Record writes a journal entry and a tombstone for each removal.
//
// Two statements per table rather than one per row: a retention run removes in batches of a
// thousand, and a statement per row would make the transaction as long as the batch. The removals
// are grouped by table here rather than at every call site, so that a purge of a hub - containers
// and entries in one act - stays a single call.
func (r LifecycleRepository) Record(
	ctx context.Context, removals []domain.Removal, deletedAt, purgeAfter time.Time,
) error {
	if len(removals) == 0 {
		return nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	byEntity, reasons, order, err := groupRemovals(removals)
	if err != nil {
		return err
	}

	for _, entity := range order {
		ids := byEntity[entity]
		err := queries.RecordDeletions(ctx, sqlc.RecordDeletionsParams{
			Entity: entity, DeletedAt: timestampOf(deletedAt),
			Reason: string(reasons[entity]), EntityIds: ids,
		})
		if err != nil {
			return shared.ErrUnavailable.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("journalling %d removals from %s: %w", len(ids), entity, err))
		}

		err = queries.RecordTombstones(ctx, sqlc.RecordTombstonesParams{
			Entity: entity, DeletedAt: timestampOf(deletedAt),
			PurgeAfter: timestampOf(purgeAfter), EntityIds: ids,
		})
		if err != nil {
			return shared.ErrUnavailable.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("marking %d removals from %s: %w", len(ids), entity, err))
		}
	}
	return nil
}

// groupRemovals sorts the removals by the table they came from, keeping the order they arrived in.
//
// The order is kept because a purge works a subtree from the bottom up (data-retention.md §4.6) and
// the containers have to be journalled in the order the caller decided, not in map order - which in
// Go is deliberately not an order at all.
//
// One reason per table, and a second reason for the same table is a defect: a single call is one
// act, and an act with two reasons is two acts written as one.
func groupRemovals(
	removals []domain.Removal,
) (map[string][]pgtype.UUID, map[string]domain.DeletionReason, []string, error) {
	byEntity := map[string][]pgtype.UUID{}
	reasons := map[string]domain.DeletionReason{}
	order := make([]string, 0, 2)

	for _, removal := range removals {
		if removal.Entity == "" || removal.EntityID.IsZero() || removal.Reason == "" {
			return nil, nil, nil, shared.ErrInternal.WithDetail("lifecycle.removal_incomplete")
		}
		id, err := uuidOf(removal.EntityID)
		if err != nil {
			return nil, nil, nil, err
		}

		if _, seen := reasons[removal.Entity]; !seen {
			reasons[removal.Entity] = removal.Reason
			order = append(order, removal.Entity)
		}
		if reasons[removal.Entity] != removal.Reason {
			return nil, nil, nil, shared.ErrInternal.WithDetail("lifecycle.removal_reasons_mixed")
		}
		byEntity[removal.Entity] = append(byEntity[removal.Entity], id)
	}
	return byEntity, reasons, order, nil
}
