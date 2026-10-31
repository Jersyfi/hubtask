// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"
	"time"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

type inline struct{}

func (inline) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (inline) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

// gate answers READ as the test decides and records what was asked.
type gate struct {
	allow bool
	asked []access.Request
}

func (g *gate) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	g.asked = append(g.asked, request)
	if !g.allow {
		return shared.ErrNotFound
	}
	return nil
}

type containerStore struct{ workrepo.Containers }

func (containerStore) Find(_ context.Context, id shared.ID) (work.Container, error) {
	return work.Container{ID: id, TenantID: tenantID, Type: work.ContainerCollection}, nil
}

func reader() appshared.ActorContext {
	return appshared.ActorContext{TenantID: tenantID, AccountID: handlerID, Kind: appshared.ActorUser, Scopes: []string{deskRead, deskWrite}}
}

// UC-SVC-02 check 6: whoever may read the entry reads its goals; anybody else is told it does
// not exist.
func TestAnEntrysGoalsAreReadByItsReaders(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, "de.hubtask.work.item.created.v1", monday, nil)
	authorizer := &gate{allow: true}
	read := ReadEntrySLA{
		Desk:  Desk{Records: f.records, Authorizer: authorizer, UnitOfWork: inline{}},
		Items: f.items, Containers: containerStore{},
	}
	records, err := read.Execute(context.Background(), reader(), itemID)
	if err != nil || len(records) != 2 {
		t.Fatalf("a reader got %d goals: %v", len(records), err)
	}
	if asked := authorizer.asked[0]; asked.Permission != service.PermissionRead || asked.TargetID != itemID {
		t.Errorf("asked %+v, want READ on the entry", asked)
	}
	authorizer.allow = false
	if _, err := read.Execute(context.Background(), reader(), itemID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody who may not read the entry got %v", err)
	}
	other := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005ff")
	if _, err := read.Execute(context.Background(), reader(), other); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an entry of nobody's got %v", err)
	}
}

// UC-SVC-02 check 7: the report asks STRUCTURE at the workspace and refuses a period past a year.
func TestTheReportAsksStructureAndABoundedPeriod(t *testing.T) {
	authorizer := &gate{allow: true}
	report := ReportSLA{Desk: Desk{Records: &recordStore{}, Authorizer: authorizer, UnitOfWork: inline{}}}
	if _, err := report.Execute(context.Background(), reader(), monday, monday.Add(MaxReportSpan+time.Hour)); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a period past a year: %v", err)
	}
	if _, err := report.Execute(context.Background(), reader(), monday, monday); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an empty period: %v", err)
	}
	if _, err := report.Execute(context.Background(), reader(), monday, monday.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if asked := authorizer.asked[0]; asked.Permission != service.PermissionStructure {
		t.Errorf("asked %s, want STRUCTURE", asked.Permission)
	}
}

// UC-SVC-01 checks 1 and 2: a calendar is defined under a key and defined again replaces it; a
// replacement of a newer one is refused.
func TestACalendarIsDefinedAndReplaced(t *testing.T) {
	store := &memoryCalendars{}
	authorizer := &gate{allow: true}
	define := DefineBusinessCalendar{Desk: Desk{
		Calendars: store, Authorizer: authorizer, UnitOfWork: inline{}, Audit: discard{},
		Clock: at(monday), IDs: &seqIDs{},
	}}
	spec, err := SpecFromInput("Europe/Berlin", []any{
		map[string]any{"day": "MONDAY", "from": "13:00", "to": "18:00"},
		map[string]any{"day": "MONDAY", "from": "08:00", "to": "12:00"},
	}, []string{"2026-12-25"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := define.Execute(context.Background(), reader(), DefineBusinessCalendarCommand{Key: "office", Name: "Office", Spec: spec})
	if err != nil || first.Version != 1 {
		t.Fatalf("defined %+v: %v", first, err)
	}
	out := CalendarOutput(first)
	if hours := out["hours"].([]usecase.Output); len(hours) != 2 || hours[0]["from"] != "08:00" {
		t.Errorf("hours drawn as %v", out["hours"])
	}
	second, err := define.Execute(context.Background(), reader(), DefineBusinessCalendarCommand{Key: "office", Name: "Office hours", Spec: spec, ExpectedVersion: 1})
	if err != nil || second.Version != 2 || second.Name != "Office hours" {
		t.Fatalf("replaced %+v: %v", second, err)
	}
	if _, err := define.Execute(context.Background(), reader(), DefineBusinessCalendarCommand{Key: "office", Name: "Late", Spec: spec, ExpectedVersion: 1}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale replacement: %v", err)
	}
	if _, err := SpecFromInput("Europe/Berlin", []any{map[string]any{"day": "FUNDAY", "from": "08:00", "to": "12:00"}}, nil); err == nil {
		t.Error("an unknown day was taken")
	}
	if _, err := SpecFromInput("Europe/Berlin", []any{map[string]any{"day": "MONDAY", "from": "8", "to": "12:00"}}, nil); err == nil {
		t.Error("a malformed time was taken")
	}
}

type memoryCalendars struct{ rows []domain.Calendar }

func (m *memoryCalendars) Insert(_ context.Context, cal domain.Calendar) error {
	m.rows = append(m.rows, cal)
	return nil
}

func (m *memoryCalendars) Update(_ context.Context, cal domain.Calendar, expected int) error {
	for i, row := range m.rows {
		if row.Key == cal.Key {
			if row.Version != expected {
				return shared.ErrVersionConflict
			}
			cal.Version = expected + 1
			m.rows[i] = cal
			return nil
		}
	}
	return shared.ErrNotFound
}

func (m *memoryCalendars) Find(_ context.Context, key string) (domain.Calendar, error) {
	for _, row := range m.rows {
		if row.Key == key {
			return row, nil
		}
	}
	return domain.Calendar{}, shared.ErrNotFound
}

func (m *memoryCalendars) List(context.Context) ([]domain.Calendar, error) { return m.rows, nil }
func (m *memoryCalendars) Delete(context.Context, string) error            { return nil }

type discard struct{}

func (discard) Append(context.Context, audit.Entry) error { return nil }
