// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// UC-SVC-02 check 1 through the doors REST and MCP call: a policy's goals are read from
// JSON - numbers as float64, targets by priority and the statuses that pause - listed, and
// replaced only at the version last read; goals that are no list are refused by name.
func TestAnSlaPolicyIsDefinedThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	policies := &memPolicies{}
	auth := &gate{allow: true}
	desk := hubPolicyDesk(f, policies, auth)
	desk.Organisations = &organisationStore{rows: map[string]domain.Organisation{}}
	doors, err := usecase.NewRegistry(nil,
		ListSlaPolicies{Desk: desk}.Descriptor(), DefineSlaPolicy{Desk: desk}.Descriptor(), DeleteSlaPolicy{Desk: desk}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	policy := func(extra usecase.Input) usecase.Input {
		in := usecase.Input{
			"key": "incidents_p", "name": "Incidents", "calendar": "office", "entry_types": []any{"incident"},
			"priority_field": "priority",
			"goals": []any{map[string]any{
				"kind": "RESOLUTION", "minutes": float64(480), "warn_percent": float64(80),
				"by_priority": map[string]any{"high": float64(120)}, "pause_statuses": []any{"waiting"},
			}},
		}
		for k, v := range extra {
			in[k] = v
		}
		return in
	}

	if _, err := doors.Invoke(ctx, DefineSLAPolicyName, reader(), policy(usecase.Input{"goals": []any{"RESOLUTION"}})); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "sla_policies.goals_malformed" {
		t.Fatalf("a goal that is no object: %v", err)
	}
	if _, err := doors.Invoke(ctx, DefineSLAPolicyName, reader(), policy(usecase.Input{"organisations": []any{"nobody"}})); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "sla_policies.organisation_unknown" {
		t.Fatalf("an organisation the workspace does not hold: %v", err)
	}
	if len(policies.rows) != 0 {
		t.Fatal("a refused policy was written")
	}

	out, err := doors.Invoke(ctx, DefineSLAPolicyName, reader(), policy(nil))
	if err != nil {
		t.Fatalf("defining: %v", err)
	}
	goal := policies.rows[0].Goals[0]
	if goal.Minutes != 480 || goal.WarnPercent != 80 || goal.ByPriority["high"] != 120 || len(goal.PauseStatuses) != 1 {
		t.Fatalf("the goal read from JSON: %+v", goal)
	}
	if _, err := doors.Invoke(ctx, DefineSLAPolicyName, reader(), policy(usecase.Input{"expected_version": out.Int("version") + 4})); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale replacement: %v", err)
	}
	listed, err := doors.Invoke(ctx, ListSLAPoliciesName, reader(), usecase.Input{})
	if err != nil || len(listed["data"].([]usecase.Output)) != 1 {
		t.Fatalf("listing: %v %v", listed, err)
	}

	auth.allow = false
	if _, err := doors.Invoke(ctx, DeleteSLAPolicyName, reader(), usecase.Input{"key": "incidents_p"}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("deleting without the right: %v", err)
	}
	auth.allow = true
	if _, err := doors.Invoke(ctx, DeleteSLAPolicyName, reader(), usecase.Input{"key": "incidents_p"}); err != nil {
		t.Fatalf("deleting: %v", err)
	}
}

// UC-SVC-11 check 1 through the doors: an organisation is defined by its domains, listed,
// replaced only at the version last read, refused while a policy names it, and deleted.
func TestARequesterOrganisationIsDefinedThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	store := &organisationStore{rows: map[string]domain.Organisation{}}
	policies := &memPolicies{}
	auth := &gate{allow: true}
	desk := Desk{
		Organisations: store, Policies: policies, Authorizer: auth, UnitOfWork: inline{}, Audit: discard{},
		Clock: at(monday), IDs: &seqIDs{},
	}
	doors, err := usecase.NewRegistry(nil,
		ListRequesterOrganisations{Desk: desk}.Descriptor(), DefineRequesterOrganisation{Desk: desk}.Descriptor(),
		DeleteRequesterOrganisation{Desk: desk}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	acme := usecase.Input{"key": "acme", "name": "Acme", "domains": []any{"acme.example"}, "shares_requests": true}

	if _, err := doors.Invoke(ctx, DefineRequesterOrganisationName, reader(), usecase.Input{"key": "acme", "name": "Acme", "domains": []any{7.0}}); err == nil {
		t.Fatal("a domain that is no text was accepted")
	}
	out, err := doors.Invoke(ctx, DefineRequesterOrganisationName, reader(), acme)
	if err != nil || out["shares_requests"] != true {
		t.Fatalf("defining: %v %v", out, err)
	}
	stale := usecase.Input{"expected_version": 9}
	for k, v := range acme {
		stale[k] = v
	}
	if _, err := doors.Invoke(ctx, DefineRequesterOrganisationName, reader(), stale); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale replacement: %v", err)
	}
	if _, err := doors.Invoke(ctx, ListRequesterOrganisationsName, reader(), usecase.Input{}); err != nil {
		t.Fatalf("listing: %v", err)
	}

	policies.rows = []domain.SLAPolicy{{Key: "acme_desk", Organisations: []string{"acme"}}}
	if _, err := doors.Invoke(ctx, DeleteRequesterOrganisationName, reader(), usecase.Input{"key": "acme"}); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "organisations.in_use" {
		t.Fatalf("an organisation a policy names was deleted: %v", err)
	}
	policies.rows = nil
	auth.allow = false
	if _, err := doors.Invoke(ctx, DeleteRequesterOrganisationName, reader(), usecase.Input{"key": "acme"}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("deleting without the right: %v", err)
	}
	auth.allow = true
	if _, err := doors.Invoke(ctx, DeleteRequesterOrganisationName, reader(), usecase.Input{"key": "acme"}); err != nil {
		t.Fatalf("deleting: %v", err)
	}
	if _, held := store.rows["acme"]; held {
		t.Fatal("the organisation was not deleted")
	}
}

func weekdayHours() []any {
	var hours []any
	for _, day := range []string{"MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY"} {
		hours = append(hours, map[string]any{"day": day, "from": "12:00", "to": "18:00"})
	}
	return hours
}

// UC-SVC-01 checks 1 and 5 through the doors: a calendar's hours are read as the contract spells
// them and refused by name where they are not - a span that is no object, a day that is none, a
// time that is no HH:MM - and a hub keeps its own version of the workspace's calendar, listed in
// the hub and deleted there; a hub out of reach is not found.
func TestCalendarsAreDefinedThroughTheDoors(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	hubs := &hubCalendarStore{}
	auth := &gate{allow: true}
	desk := Desk{
		Calendars: f.clocks.Calendars, HubCalendars: hubs, Containers: placedIn{}, Authorizer: auth,
		Audit: discard{}, UnitOfWork: inline{}, Clock: at(monday), IDs: f.ids, Text: text.Composing{},
	}
	doors, err := usecase.NewRegistry(nil,
		ListBusinessCalendars{Desk: desk}.Descriptor(), DefineBusinessCalendar{Desk: desk}.Descriptor(),
		DeleteBusinessCalendar{Desk: desk}.Descriptor(),
		ListHubCalendars{Desk: desk}.Descriptor(), DefineHubCalendar{Desk: desk}.Descriptor(),
		DeleteHubCalendar{Desk: desk}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}

	for _, c := range []struct {
		name  string
		hours []any
		code  string
	}{
		{"a span that is no object", []any{"MONDAY 9-17"}, "calendars.hours_malformed"},
		{"a day that is none", []any{map[string]any{"day": "FUNDAY", "from": "09:00", "to": "17:00"}}, "calendars.day_unknown"},
		{"a time that is no HH:MM", []any{map[string]any{"day": "MONDAY", "from": "nine", "to": "17:00"}}, "calendars.time_malformed"},
	} {
		_, err := doors.Invoke(ctx, DefineBusinessCalendarName, reader(), usecase.Input{
			"key": "office", "name": "Office", "zone": "UTC", "hours": c.hours,
		})
		if shared.AsError(err) == nil || shared.AsError(err).DetailCode != c.code {
			t.Errorf("%s: %v, want %s", c.name, err, c.code)
		}
	}
	if _, err := doors.Invoke(ctx, DefineBusinessCalendarName, reader(), usecase.Input{
		"key": "office", "name": "Office", "zone": "Europe/Berlin", "hours": weekdayHours(), "holidays": []any{"2026-12-24"},
	}); err != nil {
		t.Fatalf("defining the workspace's calendar: %v", err)
	}
	if listed, err := doors.Invoke(ctx, ListBusinessCalendarsName, reader(), usecase.Input{}); err != nil || len(listed["data"].([]usecase.Output)) == 0 {
		t.Fatalf("listing the workspace's calendars: %v %v", listed, err)
	}

	hub := usecase.Input{"hub_id": deskHub.String(), "key": "office", "name": "Office (afternoons)", "zone": "UTC", "hours": weekdayHours()}
	if _, err := doors.Invoke(ctx, DefineHubCalendarName, reader(), hub); err != nil {
		t.Fatalf("the hub's own version: %v", err)
	}
	if listed, err := doors.Invoke(ctx, ListHubCalendarsName, reader(), usecase.Input{"hub_id": deskHub.String()}); err != nil ||
		len(listed["data"].([]usecase.Output)) != 1 {
		t.Fatalf("the hub's calendars: %v %v", listed, err)
	}
	auth.allow = false
	if _, err := doors.Invoke(ctx, DeleteHubCalendarName, reader(), usecase.Input{"hub_id": deskHub.String(), "key": "office"}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("deleting out of reach: %v", err)
	}
	if len(hubs.rows) != 1 {
		t.Fatal("a refused deletion deleted")
	}
	auth.allow = true
	if _, err := doors.Invoke(ctx, DeleteHubCalendarName, reader(), usecase.Input{"hub_id": deskHub.String(), "key": "office"}); err != nil || len(hubs.rows) != 0 {
		t.Fatalf("deleting the hub's version: %v, %d left", err, len(hubs.rows))
	}
}
