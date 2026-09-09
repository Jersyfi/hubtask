// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// WorkspaceSettingsRepository is the tenant's own row as the people inside it read and change
// it (F4-01). Neither statement names a tenant: row level security has bound the transaction to
// one, and that is what makes another workspace invisible rather than forbidden.
//
// Named for what it serves rather than for the row, because `WorkspaceRepository` is taken by
// the restore's one-column reader (E-06) and two types with one name would have to live in two
// packages to be told apart.
type WorkspaceSettingsRepository struct{}

func NewWorkspaceSettingsRepository() WorkspaceSettingsRepository {
	return WorkspaceSettingsRepository{}
}

var _ repository.Workspaces = WorkspaceSettingsRepository{}

// settingsDocument is the modelled half of `tenant.settings`, and this file is the only place
// that knows its shape (`TenantPolicy`'s discipline, and `quotasDocument`'s beside it). The
// unmodelled keys never travel through here at all - the write merges rather than replaces.
type settingsDocument struct {
	RequireAdminTotp bool `json:"require_admin_totp"`
}

// Find answers the workspace the transaction is bound to.
func (WorkspaceSettingsRepository) Find(ctx context.Context) (identity.Workspace, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return identity.Workspace{}, err
	}

	row, err := queries.FindWorkspace(ctx)
	if err != nil {
		if IsNoRows(err) {
			return identity.Workspace{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
		}
		return identity.Workspace{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the workspace: %w", err))
	}

	id, err := idFrom(row.ID)
	if err != nil {
		return identity.Workspace{}, err
	}

	var settings settingsDocument
	if len(row.Settings) > 0 {
		if err := json.Unmarshal(row.Settings, &settings); err != nil {
			// The same fail-closed answer `RequireAdminTotp` gives: a settings document this
			// build cannot read must not be reported as one with enforcement switched off.
			return identity.Workspace{}, shared.ErrInternal.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("parsing the workspace settings: %w", err))
		}
	}

	return identity.Workspace{
		Tenant: identity.Tenant{
			ID: id, Slug: row.Slug, DisplayName: row.DisplayName,
			Status:        identity.TenantStatus(row.Status),
			DefaultLocale: row.DefaultLocale, DefaultTimeZone: row.DefaultTimeZone,
			CreatedAt: timeFrom(row.CreatedAt),
		},
		Settings:  identity.WorkspaceSettings{RequireAdminTotp: settings.RequireAdminTotp},
		UpdatedAt: timeFrom(row.UpdatedAt),
		Version:   int(row.Version),
	}, nil
}

// Update writes the three columns and merges the modelled settings keys, guarded on the version.
func (WorkspaceSettingsRepository) Update(
	ctx context.Context, changed identity.Workspace, expectedVersion int, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	payload, err := json.Marshal(settingsDocument{
		RequireAdminTotp: changed.Settings.RequireAdminTotp,
	})
	if err != nil {
		return false, shared.Internalf("postgres: encoding the workspace settings: %w", err)
	}

	rows, err := queries.UpdateWorkspace(ctx, sqlc.UpdateWorkspaceParams{
		DisplayName:     changed.DisplayName,
		DefaultLocale:   changed.DefaultLocale,
		DefaultTimeZone: changed.DefaultTimeZone,
		Settings:        payload,
		Now:             pgtype.Timestamptz{Time: now, Valid: true},
		//nolint:gosec // G115: a row version, bounded far below either type's range
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("writing the workspace: %w", err))
	}
	return rows > 0, nil
}
