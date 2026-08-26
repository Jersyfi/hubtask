// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// LegalHoldRepository places and lifts the instructions that override every deletion (E-08).
//
// Its own type rather than four more methods on LifecycleRepository, and for the same reason the
// ports are apart: the deletion paths take the reading half, and a repository that carried both
// would let a purge reach the statement that lifts the hold stopping it.
type LegalHoldRepository struct{}

func NewLegalHoldRepository() LegalHoldRepository { return LegalHoldRepository{} }

var _ repository.HoldWriter = LegalHoldRepository{}

// Place writes a legal hold (E-08).
func (r LegalHoldRepository) Place(ctx context.Context, hold domain.LegalHold) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(hold.ID)
	if err != nil {
		return err
	}
	scopeID, err := optionalUUID(hold.ScopeID)
	if err != nil {
		return err
	}
	placedBy, err := uuidOf(hold.PlacedBy)
	if err != nil {
		return err
	}

	err = queries.InsertLegalHold(ctx, sqlc.InsertLegalHoldParams{
		ID: id, ScopeKind: string(hold.Scope), ScopeID: scopeID,
		Reason: hold.Reason, PlacedBy: placedBy, PlacedAt: timestampOf(hold.PlacedAt),
	})
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("placing the legal hold: %w", err))
	}
	return nil
}

// Find answers one hold, released or not.
func (r LegalHoldRepository) Find(ctx context.Context, id shared.ID) (domain.LegalHold, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.LegalHold{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.LegalHold{}, err
	}

	row, err := queries.FindLegalHold(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return domain.LegalHold{}, shared.ErrNotFound.WithDetail(domain.CodeHoldNotFound).
				WithParams(map[string]string{"hold_id": id.String()})
		}
		return domain.LegalHold{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the legal hold: %w", err))
	}
	return holdFrom(sqlc.ListLegalHoldsRow(row))
}

// List answers the tenant's holds, newest first.
func (r LegalHoldRepository) List(
	ctx context.Context, includeReleased bool,
) ([]domain.LegalHold, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListLegalHolds(ctx, includeReleased)
	if err != nil {
		return nil, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the legal holds: %w", err))
	}

	holds := make([]domain.LegalHold, 0, len(rows))
	for _, row := range rows {
		hold, err := holdFrom(row)
		if err != nil {
			return nil, err
		}
		holds = append(holds, hold)
	}
	return holds, nil
}

// Release lifts one, and answers false for a hold that was already lifted.
func (r LegalHoldRepository) Release(
	ctx context.Context, hold domain.LegalHold,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(hold.ID)
	if err != nil {
		return false, err
	}
	releasedBy, err := uuidOf(hold.ReleasedBy)
	if err != nil {
		return false, err
	}

	affected, err := queries.ReleaseLegalHold(ctx, sqlc.ReleaseLegalHoldParams{
		ID: id, ReleasedBy: releasedBy, ReleasedAt: timestampOf(hold.ReleasedAt),
		ReleasedReason: optionalText(hold.ReleasedReason),
	})
	if err != nil {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("lifting the legal hold: %w", err))
	}
	return affected > 0, nil
}

func holdFrom(row sqlc.ListLegalHoldsRow) (domain.LegalHold, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.LegalHold{}, err
	}
	scopeID, err := optionalID(row.ScopeID)
	if err != nil {
		return domain.LegalHold{}, err
	}
	placedBy, err := optionalID(row.PlacedBy)
	if err != nil {
		return domain.LegalHold{}, err
	}
	releasedBy, err := optionalID(row.ReleasedBy)
	if err != nil {
		return domain.LegalHold{}, err
	}

	return domain.LegalHold{
		ID: id, Scope: domain.HoldScope(row.ScopeKind), ScopeID: scopeID,
		Reason: row.Reason, PlacedBy: placedBy, PlacedAt: timeFrom(row.PlacedAt),
		ReleasedBy: releasedBy, ReleasedAt: timeFrom(row.ReleasedAt),
		ReleasedReason: stringFrom(row.ReleasedReason),
	}, nil
}
