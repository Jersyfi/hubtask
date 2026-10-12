// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// EmergencyAccessRepository keeps the emergency accesses to private hubs (ADR-0073 §4). Row level
// security bounds every statement to the transaction's tenant (ADR-0010).
type EmergencyAccessRepository struct{}

func NewEmergencyAccessRepository() EmergencyAccessRepository { return EmergencyAccessRepository{} }

var _ repository.EmergencyAccesses = EmergencyAccessRepository{}

func accessQueryFailed(what string, err error) error {
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("%s: %w", what, err))
}

// accessFrom reads one stored access, its tenant included.
func accessFrom(
	tenant, id, hub, opener pgtype.UUID, reason string, openedAt, endsAt pgtype.Timestamptz,
) (work.EmergencyAccess, error) {
	tenantID, err := idFrom(tenant)
	if err != nil {
		return work.EmergencyAccess{}, err
	}
	accessID, err := idFrom(id)
	if err != nil {
		return work.EmergencyAccess{}, err
	}
	hubID, err := idFrom(hub)
	if err != nil {
		return work.EmergencyAccess{}, err
	}
	openedBy, err := optionalID(opener)
	if err != nil {
		return work.EmergencyAccess{}, err
	}
	return work.EmergencyAccess{
		ID: accessID, TenantID: tenantID, HubID: hubID, OpenedBy: openedBy, Reason: reason,
		OpenedAt: timeFrom(openedAt), EndsAt: timeFrom(endsAt),
	}, nil
}

// Insert writes a new access.
func (r EmergencyAccessRepository) Insert(ctx context.Context, access work.EmergencyAccess) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(access.ID)
	if err != nil {
		return err
	}
	hub, err := uuidOf(access.HubID)
	if err != nil {
		return err
	}
	opener, err := optionalUUID(access.OpenedBy)
	if err != nil {
		return err
	}
	if err := queries.InsertPrivateHubAccess(ctx, sqlc.InsertPrivateHubAccessParams{
		ID: id, HubID: hub, OpenedBy: opener, Reason: access.Reason,
		OpenedAt: timestampOf(access.OpenedAt), EndsAt: timestampOf(access.EndsAt),
	}); err != nil {
		return accessQueryFailed("writing an emergency access", err)
	}
	return nil
}

// Running answers the account's running access to the hub.
func (r EmergencyAccessRepository) Running(
	ctx context.Context, hubID, openedBy shared.ID, now time.Time,
) (work.EmergencyAccess, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.EmergencyAccess{}, false, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return work.EmergencyAccess{}, false, err
	}
	opener, err := uuidOf(openedBy)
	if err != nil {
		return work.EmergencyAccess{}, false, err
	}
	rows, err := queries.RunningPrivateHubAccess(ctx, sqlc.RunningPrivateHubAccessParams{
		HubID: hub, OpenedBy: opener, Now: timestampOf(now),
	})
	if err != nil {
		return work.EmergencyAccess{}, false, accessQueryFailed("reading a running emergency access", err)
	}
	if len(rows) == 0 {
		return work.EmergencyAccess{}, false, nil
	}
	row := rows[0]
	access, err := accessFrom(row.TenantID, row.ID, row.HubID, row.OpenedBy, row.Reason, row.OpenedAt, row.EndsAt)
	return access, err == nil, err
}

// List answers a hub's accesses, newest first.
func (r EmergencyAccessRepository) List(ctx context.Context, hubID shared.ID, limit int) ([]work.EmergencyAccess, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListPrivateHubAccesses(ctx, sqlc.ListPrivateHubAccessesParams{
		HubID: hub, PageSize: int32(min(max(limit, 1), 1000)),
	})
	if err != nil {
		return nil, accessQueryFailed("listing the emergency accesses", err)
	}
	out := make([]work.EmergencyAccess, 0, len(rows))
	for _, row := range rows {
		access, err := accessFrom(row.TenantID, row.ID, row.HubID, row.OpenedBy, row.Reason, row.OpenedAt, row.EndsAt)
		if err != nil {
			return nil, err
		}
		out = append(out, access)
	}
	return out, nil
}

// Members answers the hub's members.
func (r EmergencyAccessRepository) Members(ctx context.Context, hubID shared.ID) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PrivateHubMembers(ctx, hub)
	if err != nil {
		return nil, accessQueryFailed("reading a private hub's members", err)
	}
	return idsFrom(rows)
}

// NoticesFor answers the accesses the recipient was told of since the moment.
func (r EmergencyAccessRepository) NoticesFor(
	ctx context.Context, recipientID shared.ID, since time.Time,
) ([]work.EmergencyAccess, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	recipient, err := uuidOf(recipientID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.EmergencyNoticesFor(ctx, sqlc.EmergencyNoticesForParams{
		RecipientID: recipient, Since: timestampOf(since),
	})
	if err != nil {
		return nil, accessQueryFailed("reading the emergency notices", err)
	}
	out := make([]work.EmergencyAccess, 0, len(rows))
	for _, row := range rows {
		access, err := accessFrom(row.TenantID, row.ID, row.HubID, row.OpenedBy, row.Reason, row.OpenedAt, row.EndsAt)
		if err != nil {
			return nil, err
		}
		out = append(out, access)
	}
	return out, nil
}

// Find answers one access with its hub's name.
func (r EmergencyAccessRepository) Find(ctx context.Context, id shared.ID) (work.EmergencyAccess, string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.EmergencyAccess{}, "", err
	}
	key, err := uuidOf(id)
	if err != nil {
		return work.EmergencyAccess{}, "", err
	}
	row, err := queries.FindPrivateHubAccess(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return work.EmergencyAccess{}, "", shared.ErrNotFound.WithDetail("containers.emergency_access_not_found")
	}
	if err != nil {
		return work.EmergencyAccess{}, "", accessQueryFailed("reading an emergency access", err)
	}
	access, err := accessFrom(row.TenantID, row.ID, row.HubID, row.OpenedBy, row.Reason, row.OpenedAt, row.EndsAt)
	return access, row.HubName, err
}
