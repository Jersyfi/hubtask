// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// WorkspaceBrandingRepository stores the workspace's own pictures, one row each (migration 0449),
// and what its header shows, the settings' `header_shows`. No statement names a tenant: row level
// security binds the transaction to one, and a picture's key is (tenant, media) - another
// workspace's object is not a picture this one can name.
type WorkspaceBrandingRepository struct{}

var _ repository.WorkspaceBranding = WorkspaceBrandingRepository{}

func brandingFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// Lock reads the branding and holds the workspace's row.
func (WorkspaceBrandingRepository) Lock(ctx context.Context) (identity.Branding, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.Branding{}, err
	}
	header, err := queries.LockWorkspaceHeader(ctx)
	if IsNoRows(err) {
		return identity.Branding{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	if err != nil {
		return identity.Branding{}, brandingFailed("reading the workspace's header", err)
	}
	return brandingOf(ctx, queries, header)
}

// Set writes the three pictures and the choice: a zero picture removes its row.
func (WorkspaceBrandingRepository) Set(ctx context.Context, branding identity.Branding, now time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	for _, picture := range identity.Pictures() {
		id := branding.Of(picture)
		if id.IsZero() {
			if err := queries.RemoveWorkspacePicture(ctx, string(picture)); err != nil {
				return brandingFailed("removing a picture of the workspace", err)
			}
			continue
		}
		key, err := uuidOf(id)
		if err != nil {
			return err
		}
		err = queries.SetWorkspacePicture(ctx, sqlc.SetWorkspacePictureParams{Picture: string(picture), MediaID: key})
		if isForeignKeyViolation(err) {
			return shared.ErrNotFound.WithDetail("media.not_found")
		}
		if err != nil {
			return brandingFailed("writing a picture of the workspace", err)
		}
	}
	header := branding.Header
	if header == "" {
		header = identity.HeaderProduct
	}
	changed, err := queries.SetWorkspaceHeader(ctx, sqlc.SetWorkspaceHeaderParams{
		HeaderShows: string(header), Now: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return brandingFailed("writing the workspace's header", err)
	}
	if changed == 0 {
		return shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	return nil
}

// brandingOf reads the workspace's pictures beside the choice it read with the workspace's row,
// settled so that a picture the media's cascade took leaves no logo alone without a picture.
func brandingOf(ctx context.Context, queries *sqlc.Queries, header string) (identity.Branding, error) {
	rows, err := queries.ListWorkspacePictures(ctx)
	if err != nil {
		return identity.Branding{}, brandingFailed("reading the workspace's pictures", err)
	}
	branding := identity.Branding{Header: identity.HeaderShows(header)}
	for _, row := range rows {
		picture, err := identity.ParsePicture(row.Picture)
		if err != nil {
			continue
		}
		id, err := idFrom(row.MediaID)
		if err != nil {
			return identity.Branding{}, err
		}
		branding = branding.WithPicture(picture, id)
	}
	return branding.Settled(), nil
}
