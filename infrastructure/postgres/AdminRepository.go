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
	"github.com/Jersyfi/hubtask/infrastructure/security"
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
			PasswordOpening: identity.PasswordOpening{
				Until:     timeFrom(row.PasswordOpenedUntil),
				Requester: row.PasswordOpenedRequester, Reason: row.PasswordOpenedReason,
			},
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
		PasswordOpening: identity.PasswordOpening{
			Until:     timeFrom(row.PasswordOpenedUntil),
			Requester: stringFrom(row.PasswordOpenedRequester),
			Reason:    stringFrom(row.PasswordOpenedReason),
		},
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
type InstanceJournal struct{ cursors security.CursorCodec }

// NewInstanceJournal takes the codec the read needs; a writer-only wiring may leave it zero, and
// only `Page` would notice.
func NewInstanceJournal(cursors security.CursorCodec) InstanceJournal {
	return InstanceJournal{cursors: cursors}
}

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

// OpenPassword writes an operator's opening on the transaction's own tenant (ADR-0078 §3).
func (AdminTenantRepository) OpenPassword(
	ctx context.Context, opening identity.PasswordOpening, now time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	changed, err := queries.OpenTenantPassword(ctx, sqlc.OpenTenantPasswordParams{
		Until:     pgtype.Timestamptz{Time: opening.Until, Valid: true},
		Requester: &opening.Requester,
		Reason:    &opening.Reason,
		Now:       pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("opening the password: %w", err))
	}
	return changed > 0, nil
}

// ClosePassword ends the opening on the transaction's own tenant - any, or only one due by `due`.
func (AdminTenantRepository) ClosePassword(ctx context.Context, due, now time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	changed, err := queries.CloseTenantPassword(ctx, sqlc.CloseTenantPasswordParams{
		Due: pgtype.Timestamptz{Time: due, Valid: !due.IsZero()},
		Now: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("closing the password: %w", err))
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

// TenantPurge is the hard delete's surface (H-06, §5's final phase).
type TenantPurge struct{}

func NewTenantPurge() TenantPurge { return TenantPurge{} }

var _ repository.Purge = TenantPurge{}

// Footprint counts the stores before the fall.
func (TenantPurge) Footprint(ctx context.Context) (repository.Footprint, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Footprint{}, err
	}

	row, err := queries.CountTenantFootprint(ctx)
	if err != nil {
		return repository.Footprint{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the tenant footprint: %w", err))
	}
	return repository.Footprint{
		Items: row.Items, Containers: row.Containers,
		MediaObjects: row.MediaObjects, MediaBytes: row.MediaBytes,
		OutboxEvents: row.OutboxEvents, AuditEntries: row.AuditEntries,
	}, nil
}

// StorageKeys pages through the tenant's object keys.
func (TenantPurge) StorageKeys(ctx context.Context, after string, batch int) ([]string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	keys, err := queries.ListTenantStorageKeys(ctx, sqlc.ListTenantStorageKeysParams{
		After: after, Batch: int32(batch), //nolint:gosec // G115: a batch size, bounded small
	})
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the storage keys: %w", err))
	}
	return keys, nil
}

// DropStructure fells the structure in dependency order; see db/queries/Admin.sql for why the
// cascade cannot be left to do it.
func (TenantPurge) DropStructure(ctx context.Context) (int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	var felled int64
	for _, step := range []func(context.Context) (int64, error){
		queries.DeleteTenantCollections,
		queries.DeleteTenantHubs,
		queries.DeleteTenantMediaRows,
		queries.DeleteTenantAutomationRules,
		queries.DeleteTenantRestoreRuns,
		queries.DeleteTenantRetentionRules,
	} {
		removed, err := step(ctx)
		if err != nil {
			return felled, shared.ErrUnavailable.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("felling the structure: %w", err))
		}
		felled += removed
	}
	return felled, nil
}

// DeleteOutbox removes one batch of the tenant's outbox.
func (TenantPurge) DeleteOutbox(ctx context.Context, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	removed, err := queries.DeleteTenantOutbox(ctx, int32(batch)) //nolint:gosec // G115: bounded small
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing the outbox: %w", err))
	}
	return int(removed), nil
}

// DeleteIdempotency clears the replay guards.
func (TenantPurge) DeleteIdempotency(ctx context.Context) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	removed, err := queries.DeleteTenantIdempotency(ctx)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing the idempotency keys: %w", err))
	}
	return int(removed), nil
}

// DeleteJobs removes the tenant's queue rows except the one running this purge.
func (TenantPurge) DeleteJobs(ctx context.Context, keep shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	keepID, err := uuidOf(keep)
	if err != nil {
		return 0, err
	}
	removed, err := queries.DeleteTenantJobs(ctx, keepID)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("clearing the queue: %w", err))
	}
	return int(removed), nil
}

// PurgeTrail removes the tenant's audit trail through migration 0067's narrow act.
func (TenantPurge) PurgeTrail(ctx context.Context) (int64, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}

	removed, err := queries.PurgeTenantTrail(ctx)
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("purging the trail: %w", err))
	}
	return removed, nil
}

// HardDelete removes the tenant row itself.
func (TenantPurge) HardDelete(ctx context.Context, now time.Time) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	removed, err := queries.HardDeleteTenant(ctx, pgtype.Timestamptz{Time: now, Valid: true})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("deleting the tenant row: %w", err))
	}
	return removed > 0, nil
}

// Page walks the journal backwards, newest first (SI-17).
//
// Keyed on the moment and the identifier together, which is the same keyset every other listing
// here walks: two entries can share a moment - a provisioning writes one while a suspension writes
// another - and an offset would then skip or repeat one.
//
// No tenant anywhere in it, deliberately: the journal is the installation's own record and its rows
// are about workspaces that are usually gone, which is the reason the table exists.
func (j InstanceJournal) Page(
	ctx context.Context, cursor string, size int,
) ([]repository.InstanceEvent, repository.PageInfo, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, repository.PageInfo{}, err
	}

	params := sqlc.PageInstanceJournalParams{Limit: int32(size) + 1} //nolint:gosec // G115: clamped by the use case
	if cursor != "" {
		position, err := j.cursors.Decode(cursor)
		if err != nil {
			return nil, repository.PageInfo{}, shared.ErrValidation.
				WithDetail("shared.cursor_invalid")
		}
		at, err := time.Parse(time.RFC3339Nano, position.SortKey())
		if err != nil {
			return nil, repository.PageInfo{}, shared.ErrValidation.
				WithDetail("shared.cursor_invalid")
		}
		id, err := uuidOf(position.ID)
		if err != nil {
			return nil, repository.PageInfo{}, err
		}
		params.BeforeAt = pgtype.Timestamptz{Time: at, Valid: true}
		params.BeforeID = id
	}

	rows, err := queries.PageInstanceJournal(ctx, params)
	if err != nil {
		return nil, repository.PageInfo{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the instance journal: %w", err))
	}

	page := repository.PageInfo{}
	if len(rows) > size {
		// The row past the page is the answer to "is there another", and it is dropped rather than
		// answered: a caller that got one row more than it asked for would page wrongly.
		rows = rows[:size]
		page.HasMore = true
	}

	entries := make([]repository.InstanceEvent, 0, len(rows))
	for _, row := range rows {
		entry, err := instanceEventFrom(row)
		if err != nil {
			return nil, repository.PageInfo{}, err
		}
		entries = append(entries, entry)
	}
	if page.HasMore && len(entries) > 0 {
		last := entries[len(entries)-1]
		page.NextCursor = j.cursors.Encode(
			security.At(last.OccurredAt.UTC().Format(time.RFC3339Nano), last.ID))
	}
	return entries, page, nil
}

// instanceEventFrom maps one row. The details come back as the map they were written from: the
// journal holds counts and moments, never content, so there is nothing here to narrow.
func instanceEventFrom(row sqlc.InstanceEvent) (repository.InstanceEvent, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return repository.InstanceEvent{}, err
	}
	entry := repository.InstanceEvent{
		ID: id, OccurredAt: row.OccurredAt.Time, Action: row.Action,
	}
	if row.TenantID.Valid {
		tenantID, err := idFrom(row.TenantID)
		if err != nil {
			return repository.InstanceEvent{}, err
		}
		entry.TenantID = tenantID
	}
	if row.TenantSlug != nil {
		entry.TenantSlug = *row.TenantSlug
	}
	if row.ActorLabel != nil {
		entry.ActorLabel = *row.ActorLabel
	}
	if len(row.Details) > 0 {
		details := map[string]any{}
		if err := json.Unmarshal(row.Details, &details); err != nil {
			return repository.InstanceEvent{}, shared.ErrInternal.
				WithDetail("postgres.query_failed").
				WithCause(fmt.Errorf("decoding the journal details: %w", err))
		}
		entry.Details = details
	}
	return entry, nil
}

// InstallationRepository answers the census (SI-17, migration 0105).
//
// Its own type rather than a method on one of the others, for the reason every slice here has one:
// the census reaches through a SECURITY DEFINER function that can answer five integers and nothing
// else, and a repository that could ask it from anywhere is one that eventually grows a sixth
// question.
type InstallationRepository struct{}

func NewInstallationRepository() InstallationRepository { return InstallationRepository{} }

var _ repository.Installation = InstallationRepository{}

func (InstallationRepository) Census(ctx context.Context) (repository.Census, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Census{}, err
	}
	row, err := queries.InstanceCensus(ctx)
	if err != nil {
		return repository.Census{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the installation's census: %w", err))
	}
	return repository.Census{
		WorkspacesActive:          row.WorkspacesActive,
		WorkspacesSuspended:       row.WorkspacesSuspended,
		WorkspacesPendingDeletion: row.WorkspacesPendingDeletion,
		AccountsActive:            row.AccountsActive,
		AccountsTotal:             row.AccountsTotal,
	}, nil
}
