// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// DeviceRepository is the statements behind the devices that synchronise (N-03). Every one runs
// inside the caller's transaction and therefore under `SET LOCAL app.tenant_id` (rule 3).
type DeviceRepository struct{}

func NewDeviceRepository() DeviceRepository { return DeviceRepository{} }

var _ repository.Devices = DeviceRepository{}

// Touch registers or records a contact, and refuses with the code that says why when it cannot.
func (r DeviceRepository) Touch(ctx context.Context, contact domain.Contact) (domain.Device, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Device{}, err
	}
	id, err := uuidOf(contact.DeviceID)
	if err != nil {
		return domain.Device{}, err
	}
	account, err := uuidOf(contact.AccountID)
	if err != nil {
		return domain.Device{}, err
	}
	credential, err := optionalUUID(contact.CredentialID)
	if err != nil {
		return domain.Device{}, err
	}
	var lastSeq *int64
	if contact.LastSeq > 0 {
		lastSeq = &contact.LastSeq
	}

	row, err := queries.TouchDevice(ctx, sqlc.TouchDeviceParams{
		ID: id, AccountID: account,
		Platform: optionalText(contact.Platform), DisplayName: optionalText(contact.DisplayName),
		LastCursor: lastSeq, Now: timestampOf(contact.Now), CredentialID: credential,
	})
	if err == nil {
		return deviceFrom(sqlc.FindDeviceRow(row))
	}
	if !IsNoRows(err) {
		return domain.Device{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("touching the device: %w", err))
	}

	// The update was refused. The row says why - forgotten, or somebody else's - and a row this
	// workspace cannot see at all is somebody else's too: the identifier exists (the insert hit
	// it) and it is not this account's.
	existing, err := queries.FindDevice(ctx, id)
	switch {
	case err == nil && existing.Blocked:
		return domain.Device{}, shared.ErrForbidden.
			WithDetail("sync.device_revoked").
			WithFields(shared.FieldError{Path: "/device_id", Code: "sync.device_revoked"})
	case err == nil, IsNoRows(err):
		return domain.Device{}, shared.ErrForbidden.
			WithDetail("sync.device_foreign").
			WithFields(shared.FieldError{Path: "/device_id", Code: "sync.device_foreign"})
	default:
		return domain.Device{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the device: %w", err))
	}
}

// ForAccount lists one account's devices, most recently seen first.
func (r DeviceRepository) ForAccount(ctx context.Context, accountID shared.ID) ([]domain.Device, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListDevicesOfAccount(ctx, account)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the devices: %w", err))
	}
	devices := make([]domain.Device, 0, len(rows))
	for _, row := range rows {
		device, err := deviceFrom(sqlc.FindDeviceRow(row))
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	return devices, nil
}

// Forget marks the device blocked and answers it as it was.
func (r DeviceRepository) Forget(
	ctx context.Context, id, accountID shared.ID, now time.Time,
) (domain.Device, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Device{}, false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Device{}, false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return domain.Device{}, false, err
	}
	row, err := queries.ForgetDevice(ctx, sqlc.ForgetDeviceParams{
		ID: key, AccountID: account, Now: timestampOf(now),
	})
	if err != nil {
		if IsNoRows(err) {
			return domain.Device{}, false, nil
		}
		return domain.Device{}, false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("forgetting the device: %w", err))
	}
	device, err := deviceFrom(sqlc.FindDeviceRow(row))
	return device, err == nil, err
}

// Find answers one device of this workspace.
func (r DeviceRepository) Find(ctx context.Context, id shared.ID) (domain.Device, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Device{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Device{}, err
	}
	row, err := queries.FindDevice(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return domain.Device{}, shared.ErrNotFound.WithDetail("sync.device_not_found")
		}
		return domain.Device{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the device: %w", err))
	}
	return deviceFrom(row)
}

// DeleteExpired removes one batch of devices silent past the cutoff, revoking the session each
// last synchronised under first (the DEVICE data kind, offline-sync.md §6).
func (r DeviceRepository) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	at := pgtype.Timestamptz{Time: cutoff, Valid: true}
	if _, err := queries.RevokeSessionsOfStaleDevices(ctx, sqlc.RevokeSessionsOfStaleDevicesParams{
		Now: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, Cutoff: at,
		Batch: int32(batch), //nolint:gosec // G115: the batch size is a small configuration value
	}); err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("revoking the sessions of stale devices: %w", err))
	}
	removed, err := queries.DeleteStaleDevices(ctx, sqlc.DeleteStaleDevicesParams{
		Cutoff: at,
		Batch:  int32(batch), //nolint:gosec // G115: the batch size is a small configuration value
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("sweeping the devices: %w", err))
	}
	return int(removed), nil
}

// CountExpired reports what is due, bounded by the ceiling.
func (r DeviceRepository) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	due, err := queries.CountStaleDevices(ctx, sqlc.CountStaleDevicesParams{
		Cutoff:  pgtype.Timestamptz{Time: cutoff, Valid: true},
		Ceiling: int32(ceiling), //nolint:gosec // G115: the ceiling is a small configuration value
	})
	if err != nil {
		return 0, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("counting the stale devices: %w", err))
	}
	return int(due), nil
}

func deviceFrom(row sqlc.FindDeviceRow) (domain.Device, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Device{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return domain.Device{}, err
	}
	accountID, err := idFrom(row.AccountID)
	if err != nil {
		return domain.Device{}, err
	}
	credentialID, err := optionalID(row.CredentialID)
	if err != nil {
		return domain.Device{}, err
	}
	device := domain.Device{
		ID: id, TenantID: tenantID, AccountID: accountID,
		Platform: stringFrom(row.Platform), DisplayName: stringFrom(row.DisplayName),
		LastSeenAt: timeFrom(row.LastSeenAt), CreatedAt: timeFrom(row.CreatedAt),
		Blocked: row.Blocked, CredentialID: credentialID,
	}
	if row.LastCursor != nil {
		device.LastSeq = *row.LastCursor
	}
	return device, nil
}
