// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// DeskLogoRepository stores a collection's logo on its container row (migration 0320).
type DeskLogoRepository struct{}

var _ repository.DeskLogos = DeskLogoRepository{}

func deskLogoFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// Find answers the collection's logo.
func (DeskLogoRepository) Find(ctx context.Context, collectionID shared.ID) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	key, err := uuidOf(collectionID)
	if err != nil {
		return "", err
	}
	logo, err := queries.FindDeskLogo(ctx, key)
	if IsNoRows(err) {
		return "", shared.ErrNotFound.WithDetail("containers.not_found")
	}
	if err != nil {
		return "", deskLogoFailed("reading the desk's logo", err)
	}
	return optionalID(logo)
}

// Set writes the collection's logo, or clears it for zero.
func (DeskLogoRepository) Set(ctx context.Context, collectionID, mediaID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(collectionID)
	if err != nil {
		return err
	}
	logo, err := optionalUUID(mediaID)
	if err != nil {
		return err
	}
	changed, err := queries.SetDeskLogo(ctx, sqlc.SetDeskLogoParams{LogoMediaID: logo, CollectionID: key})
	if err != nil {
		return deskLogoFailed("writing the desk's logo", err)
	}
	if changed == 0 {
		return shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return nil
}
