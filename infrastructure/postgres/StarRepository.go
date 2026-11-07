// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// StarRepository keeps a person's stars in account_star, inside the caller's transaction (rule 3).
type StarRepository struct{}

var _ repository.Stars = StarRepository{}

// NewStarRepository answers the repository.
func NewStarRepository() StarRepository { return StarRepository{} }

// starRow is a member of the stored list.
type starRow struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Find answers the account's list, empty at version 0 when it has none.
func (StarRepository) Find(ctx context.Context, accountID shared.ID) (identity.StarList, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.StarList{}, err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return identity.StarList{}, err
	}
	row, err := queries.FindAccountStars(ctx, id)
	if err != nil {
		if IsNoRows(err) {
			return identity.StarList{Stars: []identity.Star{}}, nil
		}
		return identity.StarList{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the stars of %s: %w", accountID, err))
	}
	var rows []starRow
	if err := json.Unmarshal(row.Stars, &rows); err != nil {
		return identity.StarList{}, shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("decoding the stars of %s: %w", accountID, err))
	}
	stars := make([]identity.Star, 0, len(rows))
	for _, r := range rows {
		stars = append(stars, identity.Star{Kind: identity.StarKind(r.Kind), ID: shared.ID(r.ID)})
	}
	return identity.StarList{Stars: stars, Version: int(row.Version)}, nil
}

// Replace writes the whole list at the version expected.
func (StarRepository) Replace(
	ctx context.Context, accountID shared.ID, stars []identity.Star, expected int, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(accountID)
	if err != nil {
		return err
	}
	rows := make([]starRow, 0, len(stars))
	for _, star := range stars {
		rows = append(rows, starRow{Kind: string(star.Kind), ID: star.ID.String()})
	}
	document, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("encoding the stars of %s: %w", accountID, err)
	}
	var affected int64
	if expected == 0 {
		affected, err = queries.InsertAccountStars(ctx, sqlc.InsertAccountStarsParams{
			AccountID: id, Stars: document, At: timestampOf(at),
		})
	} else {
		affected, err = queries.UpdateAccountStars(ctx, sqlc.UpdateAccountStarsParams{
			Stars: document, At: timestampOf(at), AccountID: id, ExpectedVersion: int32(expected), //nolint:gosec // bounded by the column
		})
	}
	if err != nil {
		return shared.ErrUnavailable.WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the stars of %s: %w", accountID, err))
	}
	if affected == 0 {
		return shared.ErrVersionConflict.WithDetail("stars.version_conflict")
	}
	return nil
}
