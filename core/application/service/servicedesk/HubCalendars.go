// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// A hub's own calendars (UC-SVC-01 check 5): a hub keeps its own version of a workspace calendar
// under the workspace's key - its working hours, holidays and zone -, and every SLA policy naming
// that key counts on it for the hub's entries. Written with STRUCTURE at the hub, read with READ
// there.
const (
	ListHubCalendarsName  = "ListHubCalendars"
	DefineHubCalendarName = "DefineHubCalendar"
	DeleteHubCalendarName = "DeleteHubCalendar"

	hubCalendarTarget = "hub_business_calendar"
)

// hub reads a hub that is not in the trash and asks the permission there.
func (d Desk) hub(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, permission service.Permission, action audit.Action, scope string) error {
	var hub work.Container
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		hub, err = d.Containers.Find(ctx, hubID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) || (err == nil && (hub.Type != work.ContainerHub || hub.DeletedAt != nil)) {
		return appshared.ContainerNotFound(hubID)
	}
	if err != nil {
		return err
	}
	return d.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission, Path: service.ContainerScopes(hub),
		Action: action, TokenScope: scope, TargetType: hubCalendarTarget, TargetID: hubID,
	})
}

// HubCalendarOutput is a hub's calendar as every door answers it (schema BusinessCalendar, with
// the hub).
func HubCalendarOutput(c domain.Calendar) usecase.Output {
	out := CalendarOutput(c)
	out["hub_id"] = c.HubID.String()
	return out
}

// ListHubCalendars reads a hub's own calendars.
type ListHubCalendars struct{ Desk Desk }

// Execute answers them by key.
func (h ListHubCalendars) Execute(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) ([]domain.Calendar, error) {
	if err := h.Desk.hub(ctx, actor, hubID, service.PermissionRead, CalendarsReadAction, deskRead); err != nil {
		return nil, err
	}
	var out []domain.Calendar
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		out, err = h.Desk.HubCalendars.OfHub(ctx, hubID)
		return err
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h ListHubCalendars) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListHubCalendarsName, Permission: service.PermissionRead,
		Summary: "Lists a hub's own business calendars: the hub's version of a workspace calendar under its key, " +
			"which every SLA policy naming the key counts on for the hub's entries.",
		SideEffects: "None. Reads only.", TokenScope: deskRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."}},
		Audit: usecase.AuditDeclaration{Action: CalendarsReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hubID, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			calendars, err := h.Execute(ctx, actor, hubID)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(calendars))
			for _, c := range calendars {
				data = append(data, HubCalendarOutput(c))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, hubCalendarTarget)
}

// DefineHubCalendarCommand is the input, typed.
type DefineHubCalendarCommand struct {
	HubID           shared.ID
	Key, Name       string
	Spec            calendar.Spec
	ExpectedVersion int
}

// DefineHubCalendar keeps a hub's own version of a workspace calendar, or replaces it. A clock
// already running keeps the calendar it was armed on.
type DefineHubCalendar struct{ Desk Desk }

// Execute defines it; a key the workspace holds no calendar under is `calendars.not_found`.
func (h DefineHubCalendar) Execute(ctx context.Context, actor appshared.ActorContext, cmd DefineHubCalendarCommand) (domain.Calendar, error) {
	if err := h.Desk.hub(ctx, actor, cmd.HubID, service.PermissionStructure, CalendarDefinedAction, deskWrite); err != nil {
		return domain.Calendar{}, err
	}
	key := strings.TrimSpace(cmd.Key)
	now := h.Desk.Clock.Now()
	var out domain.Calendar
	err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Desk.Calendars.Find(ctx, key); err != nil {
			return err
		}
		current, err := h.Desk.HubCalendars.Find(ctx, cmd.HubID, key)
		switch {
		case errors.Is(err, shared.ErrNotFound):
			if out, err = domain.NewCalendar(domain.NewCalendarInput{
				ID: h.Desk.IDs.NewID(), TenantID: actor.TenantID, Key: key, Name: cmd.Name,
				Spec: cmd.Spec, Now: now, Text: h.Desk.Text,
			}); err != nil {
				return err
			}
			out.HubID = cmd.HubID
			if err := h.Desk.HubCalendars.Insert(ctx, out); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
				return shared.ErrVersionConflict.WithDetail("calendars.version_conflict")
			}
			if out, err = current.Change(cmd.Name, cmd.Spec, now, h.Desk.Text); err != nil {
				return err
			}
			if err := h.Desk.HubCalendars.Update(ctx, out, current.Version); err != nil {
				return err
			}
			out.Version = current.Version + 1
		}
		return h.Desk.record(ctx, actor, CalendarDefinedAction, hubCalendarTarget, out.ID, now,
			audit.Change{Field: "hub_id", Classification: audit.Open, To: out.HubID.String()},
			audit.Change{Field: "key", Classification: audit.Open, To: out.Key},
			audit.Change{Field: "name", Classification: audit.Sensitive, To: out.Name},
			audit.Change{Field: "zone", Classification: audit.Open, To: out.Calendar.Spec().Zone})
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h DefineHubCalendar) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DefineHubCalendarName,
		Summary: "Keeps a hub's own version of a workspace business calendar under the workspace's key - working " +
			"hours per weekday, holidays and a time zone - or replaces it. Every SLA policy naming the key counts " +
			"on it for the hub's entries; a clock already running keeps the calendar it was armed on.",
		SideEffects: "Writes the hub's calendar and an audit entry.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The key of a calendar the workspace holds."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 120 characters, on one line."},
			{Name: "zone", Kind: usecase.KindString, Required: true, Description: "An IANA time zone, e.g. Europe/Berlin."},
			{Name: "hours", Kind: usecase.KindList, Required: true, Description: "Working spans: {day: MONDAY … SUNDAY, from: HH:MM, to: HH:MM}, ordered and disjoint per day."},
			{Name: "holidays", Kind: usecase.KindList, Description: "Whole local days without working time, as YYYY-MM-DD; at most 1,000."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; a replacement of a newer one is refused."},
		},
		Audit: usecase.AuditDeclaration{Action: CalendarDefinedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hubID, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			holidays, err := in.StringList("holidays")
			if err != nil {
				return nil, err
			}
			spec, err := SpecFromInput(in.String("zone"), in["hours"], holidays)
			if err != nil {
				return nil, err
			}
			defined, err := h.Execute(ctx, actor, DefineHubCalendarCommand{
				HubID: hubID, Key: in.String("key"), Name: in.String("name"), Spec: spec, ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return HubCalendarOutput(defined), nil
		}),
	}, hubCalendarTarget)
}

// DeleteHubCalendar removes a hub's own calendar; its entries count on the workspace's again.
type DeleteHubCalendar struct{ Desk Desk }

// Execute deletes it.
func (h DeleteHubCalendar) Execute(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, key string) error {
	if err := h.Desk.hub(ctx, actor, hubID, service.PermissionStructure, CalendarDeletedAction, deskWrite); err != nil {
		return err
	}
	now := h.Desk.Clock.Now()
	return h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Desk.HubCalendars.Find(ctx, hubID, key)
		if err != nil {
			return err
		}
		if err := h.Desk.HubCalendars.Delete(ctx, hubID, key); err != nil {
			return err
		}
		return h.Desk.record(ctx, actor, CalendarDeletedAction, hubCalendarTarget, current.ID, now,
			audit.Change{Field: "hub_id", Classification: audit.Open, From: hubID.String()},
			audit.Change{Field: "key", Classification: audit.Open, From: current.Key})
	})
}

// Descriptor is the catalogue entry.
func (h DeleteHubCalendar) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name:        DeleteHubCalendarName,
		Summary:     "Deletes a hub's own version of a business calendar; the hub's entries count on the workspace's calendar of the key again.",
		SideEffects: "Deletes the hub's calendar and writes an audit entry.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The calendar's key."},
		},
		Audit: usecase.AuditDeclaration{Action: CalendarDeletedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hubID, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			if err := h.Execute(ctx, actor, hubID, in.String("key")); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}, hubCalendarTarget)
}
