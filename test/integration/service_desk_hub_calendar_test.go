// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A hub keeps its own version of a workspace calendar under the workspace's key (migration 0270,
// UC-SVC-01 check 5): stored and read back, refused under a key the workspace does not hold, the
// workspace's calendar then in use, and the workspace next door reads none of it.
func TestAHubsCalendarStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := postgres.NewServiceDeskRepository()
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	hub, _ := seedHubWithCollection(ctx, t, tenantA, authorA)
	key := "k" + strings.ReplaceAll(freshID(t).String(), "-", "")[:20]

	define := func(hubID shared.ID, key string) servicedesk.Calendar {
		cal, err := servicedesk.NewCalendar(servicedesk.NewCalendarInput{
			ID: freshID(t), TenantID: tenantA, Key: key, Name: "Office", Spec: officeCalendar(t).Spec(),
			Now: now, Text: text.Composing{},
		})
		if err != nil {
			t.Fatal(err)
		}
		cal.HubID = hubID
		return cal
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.Calendars().Insert(ctx, define("", key)); err != nil {
			return err
		}
		return repo.HubCalendars().Insert(ctx, define(hub.ID, key))
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.HubCalendars().Insert(ctx, define(hub.ID, key+"x"))
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a hub's calendar under a key the workspace does not hold: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := repo.HubCalendars().Find(ctx, hub.ID, key)
		if err != nil || found.HubID != hub.ID || found.Calendar.Spec().Zone != officeCalendar(t).Spec().Zone {
			return errors.Join(err, errors.New("the hub's calendar read back wrong"))
		}
		if list, err := repo.HubCalendars().OfHub(ctx, hub.ID); err != nil || len(list) != 1 {
			return errors.Join(err, errors.New("the hub lists its calendars wrong"))
		}
		return nil
	}); err != nil {
		t.Error(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Calendars().Delete(ctx, key)
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a workspace calendar a hub keeps its own version of was deleted: %v", err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.HubCalendars().Find(ctx, hub.ID, key); !errors.Is(err, shared.ErrNotFound) {
			return errors.Join(err, errors.New("the hub's calendar crossed the workspace"))
		}
		return nil
	}); err != nil {
		t.Error(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.HubCalendars().Delete(ctx, hub.ID, key); err != nil {
			return err
		}
		return repo.Calendars().Delete(ctx, key)
	}); err != nil {
		t.Errorf("deleting the hub's calendar, then the workspace's: %v", err)
	}
}
