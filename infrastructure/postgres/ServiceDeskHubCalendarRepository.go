// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

var _ repository.HubCalendars = ServiceDeskHubCalendars{}

// ServiceDeskHubCalendars is the hubs' own calendars (migration 0270).
type ServiceDeskHubCalendars struct{}

func (ServiceDeskRepository) HubCalendars() ServiceDeskHubCalendars { return ServiceDeskHubCalendars{} }

func (ServiceDeskHubCalendars) Insert(ctx context.Context, cal servicedesk.Calendar) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids := make([]pgtype.UUID, 3)
	for i, id := range []shared.ID{cal.ID, cal.TenantID, cal.HubID} {
		if ids[i], err = uuidOf(id); err != nil {
			return err
		}
	}
	spec, err := calendarJSON(cal.Calendar)
	if err != nil {
		return shared.ErrInternal.WithDetail("calendars.unwritable").WithCause(err)
	}
	if err := queries.InsertHubCalendar(ctx, sqlc.InsertHubCalendarParams{
		ID: ids[0], TenantID: ids[1], HubID: ids[2], Key: cal.Key, Name: cal.Name, Spec: spec,
		Now: timestampOf(cal.CreatedAt),
	}); err != nil {
		switch {
		case isUniqueViolation(err):
			return shared.ErrConflict.WithDetail("calendars.key_taken").WithParams(map[string]string{"key": cal.Key})
		case isForeignKeyViolation(err):
			// The workspace holds no calendar of the key, or the hub is gone.
			return shared.ErrNotFound.WithDetail("calendars.not_found")
		}
		return queryFailed("writing a hub's business calendar", err)
	}
	return nil
}

func (ServiceDeskHubCalendars) Update(ctx context.Context, cal servicedesk.Calendar, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	hub, err := uuidOf(cal.HubID)
	if err != nil {
		return err
	}
	spec, err := calendarJSON(cal.Calendar)
	if err != nil {
		return shared.ErrInternal.WithDetail("calendars.unwritable").WithCause(err)
	}
	affected, err := queries.UpdateHubCalendar(ctx, sqlc.UpdateHubCalendarParams{
		Name: cal.Name, Spec: spec, Now: timestampOf(cal.UpdatedAt), HubID: hub, Key: cal.Key,
		ExpectedVersion: int32(expectedVersion), //nolint:gosec // a version is far below 2^31
	})
	if err != nil {
		return queryFailed("updating a hub's business calendar", err)
	}
	if affected == 0 {
		return shared.ErrVersionConflict.WithDetail("calendars.version_conflict")
	}
	return nil
}

func hubCalendarFrom(row sqlc.HubBusinessCalendar) (servicedesk.Calendar, error) {
	cal, err := calendarFrom(sqlc.BusinessCalendar{
		ID: row.ID, TenantID: row.TenantID, Key: row.Key, Name: row.Name, Spec: row.Spec,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Version: row.Version,
	})
	if err != nil {
		return servicedesk.Calendar{}, err
	}
	if cal.HubID, err = idFrom(row.HubID); err != nil {
		return servicedesk.Calendar{}, err
	}
	return cal, nil
}

func (ServiceDeskHubCalendars) Find(ctx context.Context, hubID shared.ID, key string) (servicedesk.Calendar, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.Calendar{}, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return servicedesk.Calendar{}, err
	}
	row, err := queries.FindHubCalendar(ctx, sqlc.FindHubCalendarParams{HubID: hub, Key: key})
	if err != nil {
		if IsNoRows(err) {
			return servicedesk.Calendar{}, shared.ErrNotFound.WithDetail("calendars.not_found")
		}
		return servicedesk.Calendar{}, queryFailed("reading a hub's business calendar", err)
	}
	return hubCalendarFrom(row)
}

func (ServiceDeskHubCalendars) OfHub(ctx context.Context, hubID shared.ID) ([]servicedesk.Calendar, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListHubCalendars(ctx, hub)
	if err != nil {
		return nil, queryFailed("listing a hub's business calendars", err)
	}
	return mapRows(rows, hubCalendarFrom)
}

func (ServiceDeskHubCalendars) Delete(ctx context.Context, hubID shared.ID, key string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return err
	}
	affected, err := queries.DeleteHubCalendar(ctx, sqlc.DeleteHubCalendarParams{HubID: hub, Key: key})
	if err != nil {
		return queryFailed("deleting a hub's business calendar", err)
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail("calendars.not_found")
	}
	return nil
}
