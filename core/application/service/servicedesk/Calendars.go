// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ListBusinessCalendarsName  = "ListBusinessCalendars"
	DefineBusinessCalendarName = "DefineBusinessCalendar"
	DeleteBusinessCalendarName = "DeleteBusinessCalendar"
)

// weekdays spells the days as the contract does, Sunday first as time.Weekday counts.
var weekdays = [...]string{"SUNDAY", "MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "SATURDAY"}

func clockText(minutes int) string { return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60) }

// minutesOf reads "HH:MM"; "24:00" ends a day.
func minutesOf(raw string) (int, bool) {
	hours, minutes, ok := strings.Cut(raw, ":")
	if !ok || len(hours) != 2 || len(minutes) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(hours)
	m, err2 := strconv.Atoi(minutes)
	if err1 != nil || err2 != nil || m < 0 || m > 59 || h < 0 || h > 24 || (h == 24 && m != 0) {
		return 0, false
	}
	return h*60 + m, true
}

func malformed(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

// SpecFromInput reads a calendar from a door's input: a zone, working hours as
// {day, from, to} spans, holidays as dates (schema BusinessCalendarDefine).
func SpecFromInput(zone string, hours any, holidays []string) (calendar.Spec, error) {
	spec := calendar.Spec{Zone: strings.TrimSpace(zone)}
	list, _ := hours.([]any)
	if hours != nil && list == nil {
		return calendar.Spec{}, malformed("/hours", "calendars.hours_malformed")
	}
	for i, raw := range list {
		at := "/hours/" + strconv.Itoa(i)
		span, ok := raw.(map[string]any)
		if !ok {
			return calendar.Spec{}, malformed(at, "calendars.hours_malformed")
		}
		day := -1
		for index, name := range weekdays {
			if span["day"] == name {
				day = index
			}
		}
		if day < 0 {
			return calendar.Spec{}, malformed(at+"/day", "calendars.day_unknown")
		}
		fromText, _ := span["from"].(string)
		toText, _ := span["to"].(string)
		from, okFrom := minutesOf(fromText)
		to, okTo := minutesOf(toText)
		if !okFrom || !okTo {
			return calendar.Spec{}, malformed(at, "calendars.time_malformed")
		}
		spec.Week[day] = append(spec.Week[day], calendar.Span{From: from, To: to})
	}
	for day := range spec.Week {
		slices.SortFunc(spec.Week[day], func(a, b calendar.Span) int { return a.From - b.From })
	}
	for i, raw := range holidays {
		date, err := time.Parse(time.DateOnly, strings.TrimSpace(raw))
		if err != nil {
			return calendar.Spec{}, malformed("/holidays/"+strconv.Itoa(i), "calendar.holiday_invalid")
		}
		spec.Holidays = append(spec.Holidays, calendar.Date{Year: date.Year(), Month: date.Month(), Day: date.Day()})
	}
	return spec, nil
}

// CalendarOutput is the shape every door returns (api/openapi.yaml, schema BusinessCalendar).
func CalendarOutput(c domain.Calendar) usecase.Output {
	spec := c.Calendar.Spec()
	hours := []usecase.Output{}
	for day, spans := range spec.Week {
		for _, span := range spans {
			hours = append(hours, usecase.Output{"day": weekdays[day], "from": clockText(span.From), "to": clockText(span.To)})
		}
	}
	holidays := make([]string, 0, len(spec.Holidays))
	for _, date := range spec.Holidays {
		holidays = append(holidays, fmt.Sprintf("%04d-%02d-%02d", date.Year, date.Month, date.Day))
	}
	return usecase.Output{
		"key": c.Key, "name": c.Name, "zone": spec.Zone, "hours": hours, "holidays": holidays,
		"version": c.Version,
	}
}

// ListBusinessCalendars reads the workspace's calendars (UC-SVC-01).
type ListBusinessCalendars struct{ Desk Desk }

// Execute answers every calendar by key. Any member may read them: an SLA shows which it runs on.
func (h ListBusinessCalendars) Execute(ctx context.Context, actor appshared.ActorContext) ([]domain.Calendar, error) {
	if err := actor.RequireScope(deskRead); err != nil {
		return nil, err
	}
	var out []domain.Calendar
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		out, err = h.Desk.Calendars.List(ctx)
		return err
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h ListBusinessCalendars) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name:        ListBusinessCalendarsName,
		Summary:     "Lists the workspace's business calendars: each one's working hours per weekday, holidays and time zone.",
		SideEffects: "None. Reads only.",
		TokenScope:  deskRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: CalendarsReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			calendars, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(calendars))
			for _, c := range calendars {
				data = append(data, CalendarOutput(c))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, calendarTarget)
}

// DefineBusinessCalendarCommand is the input, typed.
type DefineBusinessCalendarCommand struct {
	Key  string
	Name string
	Spec calendar.Spec
	// ExpectedVersion guards a replacement; 0 replaces whatever stands.
	ExpectedVersion int
}

// DefineBusinessCalendar creates a calendar under a key, or replaces the one there (UC-SVC-01).
// A clock already running keeps the calendar it was armed on.
type DefineBusinessCalendar struct{ Desk Desk }

// Execute defines the calendar.
func (h DefineBusinessCalendar) Execute(ctx context.Context, actor appshared.ActorContext, cmd DefineBusinessCalendarCommand) (domain.Calendar, error) {
	if err := h.Desk.authorizeWrite(ctx, actor, CalendarDefinedAction, calendarTarget); err != nil {
		return domain.Calendar{}, err
	}
	now := h.Desk.Clock.Now()
	var out domain.Calendar
	err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Desk.Calendars.Find(ctx, strings.TrimSpace(cmd.Key))
		switch {
		case errors.Is(err, shared.ErrNotFound):
			if out, err = domain.NewCalendar(domain.NewCalendarInput{
				ID: h.Desk.IDs.NewID(), TenantID: actor.TenantID, Key: cmd.Key, Name: cmd.Name,
				Spec: cmd.Spec, Now: now, Text: h.Desk.Text,
			}); err != nil {
				return err
			}
			if err := h.Desk.Calendars.Insert(ctx, out); err != nil {
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
			if err := h.Desk.Calendars.Update(ctx, out, current.Version); err != nil {
				return err
			}
			out.Version = current.Version + 1
		}
		return h.Desk.record(ctx, actor, CalendarDefinedAction, calendarTarget, out.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, To: out.Key},
			audit.Change{Field: "name", Classification: audit.Sensitive, To: out.Name},
			audit.Change{Field: "zone", Classification: audit.Open, To: out.Calendar.Spec().Zone})
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h DefineBusinessCalendar) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DefineBusinessCalendarName,
		Summary: "Creates a business calendar under a key - working hours per weekday, holidays and a time " +
			"zone - or replaces the one there. An SLA counts its targets in this calendar's working time; a " +
			"clock already running keeps the calendar it was armed on.",
		SideEffects: "Writes the calendar and an audit entry.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "Small letters, digits and underscores, starting with a letter."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "Up to 120 characters, on one line."},
			{Name: "zone", Kind: usecase.KindString, Required: true, Description: "An IANA time zone, e.g. Europe/Berlin."},
			{Name: "hours", Kind: usecase.KindList, Required: true, Description: "Working spans: {day: MONDAY … SUNDAY, from: HH:MM, to: HH:MM}, ordered and disjoint per day."},
			{Name: "holidays", Kind: usecase.KindList, Description: "Whole local days without working time, as YYYY-MM-DD; at most 1,000."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read; a replacement of a newer one is refused."},
		},
		Audit: usecase.AuditDeclaration{Action: CalendarDefinedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			holidays, err := in.StringList("holidays")
			if err != nil {
				return nil, err
			}
			spec, err := SpecFromInput(in.String("zone"), in["hours"], holidays)
			if err != nil {
				return nil, err
			}
			defined, err := h.Execute(ctx, actor, DefineBusinessCalendarCommand{
				Key: in.String("key"), Name: in.String("name"), Spec: spec, ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return CalendarOutput(defined), nil
		}),
	}, calendarTarget)
}

// DeleteBusinessCalendar removes a calendar no policy names (UC-SVC-01).
type DeleteBusinessCalendar struct{ Desk Desk }

// Execute deletes the calendar; one a policy names is refused with `calendars.in_use`.
func (h DeleteBusinessCalendar) Execute(ctx context.Context, actor appshared.ActorContext, key string) error {
	if err := h.Desk.authorizeWrite(ctx, actor, CalendarDeletedAction, calendarTarget); err != nil {
		return err
	}
	now := h.Desk.Clock.Now()
	return h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Desk.Calendars.Find(ctx, key)
		if err != nil {
			return err
		}
		if err := h.Desk.Calendars.Delete(ctx, key); err != nil {
			return err
		}
		return h.Desk.record(ctx, actor, CalendarDeletedAction, calendarTarget, current.ID, now,
			audit.Change{Field: "key", Classification: audit.Open, From: current.Key})
	})
}

// Descriptor is the catalogue entry.
func (h DeleteBusinessCalendar) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name:        DeleteBusinessCalendarName,
		Summary:     "Deletes a business calendar that no SLA policy names; one in use is refused with calendars.in_use.",
		SideEffects: "Deletes the calendar and writes an audit entry.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "key", Kind: usecase.KindString, Required: true, Description: "The calendar's key."},
		},
		Audit: usecase.AuditDeclaration{Action: CalendarDeletedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := h.Execute(ctx, actor, in.String("key")); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}, calendarTarget)
}
