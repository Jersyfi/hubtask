// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// UsageRepository reads the use for the control plane (multi-tenancy.md §4.3), through
// `admin_usage()`, which answers nothing inside a workspace's scope. Its cursor is the installation's
// signed one, as every list's (api-guidelines.md §4).
type UsageRepository struct{ cursors security.CursorCodec }

// NewUsageRepository answers the repository.
func NewUsageRepository(cursors security.CursorCodec) UsageRepository {
	return UsageRepository{cursors: cursors}
}

// CursorAfter is the cursor of a page whose last workspace was last.
func (r UsageRepository) CursorAfter(last shared.ID) string {
	return r.cursors.Encode(security.At("", last))
}

var _ repository.UsageLedger = UsageRepository{}

// Read answers the rows of at most query.Size workspaces.
func (r UsageRepository) Read(ctx context.Context, query repository.UsageQuery) ([]repository.UsageRow, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	params := sqlc.AdminUsageParams{
		FromDay:  pgtype.Date{Time: query.From, Valid: true},
		ToDay:    pgtype.Date{Time: query.To, Valid: true},
		PageSize: int32(min(query.Size, 1000)), //nolint:gosec // G115: bounded above
	}
	if query.Metric != "" {
		params.WantMetric = &query.Metric
	}
	if !query.Tenant.IsZero() {
		if params.WantTenant, err = uuidOf(query.Tenant); err != nil {
			return nil, err
		}
	}
	if query.Cursor != "" {
		after, err := r.cursors.Decode(query.Cursor)
		if err != nil {
			return nil, err
		}
		if params.AfterTenant, err = uuidOf(after.ID); err != nil {
			return nil, err
		}
	}
	rows, err := queries.AdminUsage(ctx, params)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the use: %w", err))
	}
	read := make([]repository.UsageRow, 0, len(rows))
	for _, row := range rows {
		tenant, err := idFrom(row.TenantID)
		if err != nil {
			return nil, err
		}
		usage := repository.UsageRow{
			Tenant: tenant, Day: row.Period.Time.UTC().Truncate(24 * time.Hour), Metric: row.Metric,
			Value: row.Value, Purged: row.Purged,
		}
		if row.HasClosing {
			closing := row.Closing
			usage.Closing = &closing
		}
		read = append(read, usage)
	}
	return read, nil
}
