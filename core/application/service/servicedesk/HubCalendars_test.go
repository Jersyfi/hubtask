// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"
	"time"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var deskHub = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005f9")

// placedIn answers the fixture's collection under the hub and the hub itself.
type placedIn struct{ workrepo.Containers }

func (placedIn) Find(_ context.Context, id shared.ID) (work.Container, error) {
	switch id {
	case collection:
		return work.Container{ID: id, TenantID: tenantID, Type: work.ContainerCollection, ParentID: deskHub}, nil
	case deskHub:
		return work.Container{ID: id, TenantID: tenantID, Type: work.ContainerHub}, nil
	}
	return work.Container{}, shared.ErrNotFound
}

type hubCalendarStore struct{ rows []domain.Calendar }

func (s *hubCalendarStore) Insert(_ context.Context, cal domain.Calendar) error {
	s.rows = append(s.rows, cal)
	return nil
}
func (s *hubCalendarStore) Update(_ context.Context, cal domain.Calendar, _ int) error {
	for i, row := range s.rows {
		if row.HubID == cal.HubID && row.Key == cal.Key {
			s.rows[i] = cal
		}
	}
	return nil
}
func (s *hubCalendarStore) Find(_ context.Context, hubID shared.ID, key string) (domain.Calendar, error) {
	for _, row := range s.rows {
		if row.HubID == hubID && row.Key == key {
			return row, nil
		}
	}
	return domain.Calendar{}, shared.ErrNotFound
}
func (s *hubCalendarStore) OfHub(_ context.Context, hubID shared.ID) ([]domain.Calendar, error) {
	var out []domain.Calendar
	for _, row := range s.rows {
		if row.HubID == hubID {
			out = append(out, row)
		}
	}
	return out, nil
}
func (s *hubCalendarStore) Delete(_ context.Context, hubID shared.ID, key string) error {
	for i, row := range s.rows {
		if row.HubID == hubID && row.Key == key {
			s.rows = append(s.rows[:i], s.rows[i+1:]...)
			return nil
		}
	}
	return shared.ErrNotFound
}

// afternoons is a hub's own version of *office*: weekdays 12:00 to 18:00 UTC.
func afternoons() calendar.Spec {
	var week calendar.Week
	for day := time.Monday; day <= time.Friday; day++ {
		week[day] = []calendar.Span{{From: 12 * 60, To: 18 * 60}}
	}
	return calendar.Spec{Zone: "UTC", Week: week}
}

// UC-SVC-01 check 5: a hub keeps its own version of a workspace calendar under its key, and a
// policy naming the key counts on it for the hub's entries.
func TestAHubsOwnCalendarIsWhatItsEntriesCountOn(t *testing.T) {
	f := newFixture(t)
	hubs := &hubCalendarStore{}
	f.clocks.HubCalendars, f.clocks.Containers = hubs, placedIn{}
	desk := Desk{
		Calendars: f.clocks.Calendars, HubCalendars: hubs, Containers: placedIn{}, Authorizer: &gate{allow: true},
		Audit: discard{}, UnitOfWork: inline{}, Clock: at(monday), IDs: f.ids, Text: text.Composing{},
	}
	ctx := context.Background()
	if _, err := (DefineHubCalendar{Desk: desk}).Execute(ctx, reader(), DefineHubCalendarCommand{
		HubID: deskHub, Key: "nights", Name: "Nights", Spec: afternoons(),
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a hub's calendar under a key the workspace does not hold: %v", err)
	}
	if _, err := (DefineHubCalendar{Desk: desk}).Execute(ctx, reader(), DefineHubCalendarCommand{
		HubID: collection, Key: "office", Name: "Office", Spec: afternoons(),
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a collection kept a calendar of its own: %v", err)
	}
	defined, err := (DefineHubCalendar{Desk: desk}).Execute(ctx, reader(), DefineHubCalendarCommand{
		HubID: deskHub, Key: "office", Name: "Office (afternoons)", Spec: afternoons(),
	})
	if err != nil || defined.HubID != deskHub {
		t.Fatalf("DefineHubCalendar: %+v %v", defined, err)
	}
	listed, err := (ListHubCalendars{Desk: desk}).Execute(ctx, reader(), deskHub)
	if err != nil || len(listed) != 1 || HubCalendarOutput(listed[0])["hub_id"] != deskHub.String() {
		t.Fatalf("ListHubCalendars: %+v %v", listed, err)
	}

	f.deliver(t, event.ItemCreated, monday, nil)
	// Monday 08:00 UTC: the hub works from 12:00, so four working hours end at 16:00.
	if due := f.records.goal(domain.GoalResolution).DueAt; due == nil || !due.Equal(monday.Add(8*time.Hour)) {
		t.Errorf("resolution due %v, want 16:00 on the hub's own calendar", due)
	}

	if err := (DeleteHubCalendar{Desk: desk}).Execute(ctx, reader(), deskHub, "office"); err != nil || len(hubs.rows) != 0 {
		t.Errorf("DeleteHubCalendar: %v, %d left", err, len(hubs.rows))
	}
}
