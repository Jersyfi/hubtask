// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// AdminTenantRepository is the control plane's view of the tenant row (H-06).
//
// The listing is the one deliberate exception to "no statement reaches beyond its transaction's
// tenant": it goes through the SECURITY DEFINER enumerator migration 0067 pins down, under the
// installation scope. Everything else here is bounded the way every repository is.
type AdminTenantRepository struct{}

func NewAdminTenantRepository() AdminTenantRepository { return AdminTenantRepository{} }

var _ repository.Tenants = AdminTenantRepository{}

// List reads through admin_tenants(), the one legitimate enumerator (0.6.0 decision 6).
func (AdminTenantRepository) List(ctx context.Context) ([]repository.TenantRecord, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.AdminTenants(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the tenants: %w", err))
	}

	records := make([]repository.TenantRecord, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		records = append(records, repository.TenantRecord{
			ID: id, Slug: row.Slug, DisplayName: row.DisplayName,
			Status:        identity.TenantStatus(row.Status),
			DefaultLocale: row.DefaultLocale, DefaultTimeZone: row.DefaultTimeZone,
			CreatedAt: timeFrom(row.CreatedAt), PurgeAfter: timeFrom(row.PurgeAfter),
		})
	}
	return records, nil
}

// Insert writes the row inside the new tenant's own scope; the identifier comes from
// current_tenant_id(), so the row and the transaction cannot disagree.
func (AdminTenantRepository) Insert(ctx context.Context, record repository.TenantRecord) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	err = queries.InsertTenant(ctx, sqlc.InsertTenantParams{
		Slug: record.Slug, DisplayName: record.DisplayName,
		DefaultLocale: record.DefaultLocale, DefaultTimeZone: record.DefaultTimeZone,
		Settings: []byte("{}"),
		Now:      pgtype.Timestamptz{Time: record.CreatedAt, Valid: true},
	})
	if err != nil {
		if isUniqueViolation(err) {
			return shared.ErrConflict.
				WithDetail("admin.slug_taken").
				WithParams(map[string]string{"slug": record.Slug})
		}
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("inserting the tenant: %w", err))
	}
	return nil
}

// Find answers the transaction's own tenant row.
func (AdminTenantRepository) Find(ctx context.Context) (repository.TenantRecord, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.TenantRecord{}, err
	}

	row, err := queries.FindTenantForAdmin(ctx)
	if err != nil {
		if IsNoRows(err) {
			return repository.TenantRecord{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
		}
		return repository.TenantRecord{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the tenant: %w", err))
	}

	id, err := idFrom(row.ID)
	if err != nil {
		return repository.TenantRecord{}, err
	}
	return repository.TenantRecord{
		ID: id, Slug: row.Slug, DisplayName: row.DisplayName,
		Status:        identity.TenantStatus(row.Status),
		DefaultLocale: row.DefaultLocale, DefaultTimeZone: row.DefaultTimeZone,
		CreatedAt: timeFrom(row.CreatedAt), PurgeAfter: timeFrom(row.PurgeAfter),
		Version: int(row.Version),
	}, nil
}

// SetStatus moves one guarded edge on the transaction's own tenant.
func (AdminTenantRepository) SetStatus(
	ctx context.Context, from, to identity.TenantStatus, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	changed, err := queries.SetTenantStatus(ctx, sqlc.SetTenantStatusParams{
		NextStatus:     sqlc.TenantStatus(to),
		ExpectedStatus: sqlc.TenantStatus(from),
		Now:            pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("moving the tenant status: %w", err))
	}
	return changed > 0, nil
}

// InstanceJournal writes the installation's own evidence (audit.md §6). The table carries no
// row-level-security policy, so the write lands inside whatever transaction the act runs in -
// including the one that ends the tenant it names.
type InstanceJournal struct{}

func NewInstanceJournal() InstanceJournal { return InstanceJournal{} }

var _ repository.Journal = InstanceJournal{}

// Record appends one entry.
func (InstanceJournal) Record(ctx context.Context, entry repository.InstanceEvent) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(entry.ID)
	if err != nil {
		return err
	}
	details := entry.Details
	if details == nil {
		details = map[string]any{}
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return shared.ErrInternal.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("encoding the journal details: %w", err))
	}

	params := sqlc.InsertInstanceEventParams{
		ID: id, OccurredAt: pgtype.Timestamptz{Time: entry.OccurredAt, Valid: true},
		Action: entry.Action, Details: payload,
	}
	if !entry.TenantID.IsZero() {
		tenantID, err := uuidOf(entry.TenantID)
		if err != nil {
			return err
		}
		params.TenantID = tenantID
	}
	if entry.TenantSlug != "" {
		params.TenantSlug = &entry.TenantSlug
	}
	if entry.ActorLabel != "" {
		params.ActorLabel = &entry.ActorLabel
	}

	if err := queries.InsertInstanceEvent(ctx, params); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("recording the instance event: %w", err))
	}
	return nil
}

// RequestDeletion moves either living status to PENDING_DELETION and stamps the grace deadline.
func (AdminTenantRepository) RequestDeletion(
	ctx context.Context, purgeAfter, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	changed, err := queries.RequestTenantDeletion(ctx, sqlc.RequestTenantDeletionParams{
		PurgeAfter: pgtype.Timestamptz{Time: purgeAfter, Valid: true},
		Now:        pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("requesting the tenant deletion: %w", err))
	}
	return changed > 0, nil
}

// AutomationSwitch throws §5's one switch: every enabled rule of the transaction's tenant, off.
type AutomationSwitch struct{}

func NewAutomationSwitch() AutomationSwitch { return AutomationSwitch{} }

var _ repository.Automations = AutomationSwitch{}

// DisableAll is bounded by row level security to the tenant of the transaction.
func (AutomationSwitch) DisableAll(ctx context.Context, now time.Time) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	disabled, err := queries.DisableAllAutomationRules(ctx, pgtype.Timestamptz{Time: now, Valid: true})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("disabling the automation rules: %w", err))
	}
	return int(disabled), nil
}
