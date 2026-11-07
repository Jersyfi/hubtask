// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

func policyInput() NewSLAPolicyInput {
	return NewSLAPolicyInput{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000005a1"), TenantID: shared.MustParseID("0192f000-0000-7000-8000-0000000005a2"), Key: "incidents", Name: " Incidents ",
		CalendarKey: "office", EntryTypes: []string{"incident", "incident"}, PriorityField: "priority",
		Goals: []SLAGoal{
			{Kind: GoalResolution, Minutes: 480, ByPriority: map[string]int{"p1": 240}, PauseStatuses: []string{"waiting"}, WarnPercent: 75},
			{Kind: GoalFirstResponse, Minutes: 60, ByPriority: map[string]int{"p1": 15}},
		},
		Enabled: true, Now: time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC), Text: text.Composing{},
	}
}

// UC-SVC-02 check 1: a policy promises a first response and a resolution per priority.
func TestNewSlaPolicy(t *testing.T) {
	policy, err := NewSLAPolicy(policyInput())
	if err != nil {
		t.Fatalf("a sound policy was refused: %v", err)
	}
	if policy.Name != "Incidents" || len(policy.EntryTypes) != 1 || policy.Goals[0].Kind != GoalFirstResponse {
		t.Errorf("not normalised: %+v", policy)
	}
	resolution, _ := policy.Goal(GoalResolution)
	if resolution.Target("p1") != 4*time.Hour || resolution.Target("p3") != 8*time.Hour || resolution.Target("") != 8*time.Hour {
		t.Errorf("targets wrong: %v %v", resolution.Target("p1"), resolution.Target("p3"))
	}
	if warning, ok := resolution.Warning("p1"); !ok || warning != 3*time.Hour {
		t.Errorf("warning at 75%% of 4h is 3h, got %v", warning)
	}
	first, _ := policy.Goal(GoalFirstResponse)
	if _, ok := first.Warning(""); ok {
		t.Error("a goal without a share announces no risk")
	}
	if !resolution.Pauses("waiting") || resolution.Pauses("open") || resolution.Pauses("") {
		t.Error("pause statuses wrong")
	}

	cases := []struct {
		name string
		edit func(*NewSLAPolicyInput)
		code string
	}{
		{"a malformed key", func(in *NewSLAPolicyInput) { in.Key = "Bad Key" }, "sla_policies.key_malformed"},
		{"no name", func(in *NewSLAPolicyInput) { in.Name = " " }, "sla_policies.name_required"},
		{"no type", func(in *NewSLAPolicyInput) { in.EntryTypes = nil }, "sla_policies.entry_types_required"},
		{"no goal", func(in *NewSLAPolicyInput) { in.Goals = nil }, "sla_policies.goals_required"},
		{"a goal twice", func(in *NewSLAPolicyInput) { in.Goals[1].Kind = GoalResolution }, "sla_policies.goal_twice"},
		{"an unknown goal", func(in *NewSLAPolicyInput) { in.Goals[0].Kind = "LUNCH" }, "sla_policies.goal_kind_unknown"},
		{"no target", func(in *NewSLAPolicyInput) { in.Goals[0].Minutes = 0 }, "sla_policies.target_invalid"},
		{"a target past a year", func(in *NewSLAPolicyInput) { in.Goals[0].Minutes = MaxTargetMinutes + 1 }, "sla_policies.target_invalid"},
		{"a priority target of nothing", func(in *NewSLAPolicyInput) { in.Goals[0].ByPriority = map[string]int{"p1": -1} }, "sla_policies.target_invalid"},
		{"a share of all", func(in *NewSLAPolicyInput) { in.Goals[0].WarnPercent = 100 }, "sla_policies.warn_percent_invalid"},
		{"a malformed status", func(in *NewSLAPolicyInput) { in.Goals[0].PauseStatuses = []string{"Waiting!"} }, "sla_policies.key_malformed"},
		{"a malformed calendar", func(in *NewSLAPolicyInput) { in.CalendarKey = "Office" }, "sla_policies.calendar_malformed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := policyInput()
			c.edit(&in)
			_, err := NewSLAPolicy(in)
			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != c.code {
				t.Errorf("want %s, got %v", c.code, err)
			}
		})
	}
}

// UC-SVC-02 check 1: one policy per entry - the first enabled one covering its type.
func TestChoose(t *testing.T) {
	first, _ := NewSLAPolicy(policyInput())
	second := first
	second.Key, second.Position = "all_incidents", 1
	disabled := first
	disabled.Key, disabled.Position, disabled.Enabled = "aaa", -1, false
	chosen, ok := Choose([]SLAPolicy{second, disabled, first}, "incident", "")
	if !ok || chosen.Key != "incidents" {
		t.Errorf("want incidents, got %q", chosen.Key)
	}
	if _, ok := Choose([]SLAPolicy{first}, "task", ""); ok {
		t.Error("a type no policy covers is measured by none")
	}
}

// UC-SVC-02 check 9: a policy naming organisations measures only their requesters' entries; the
// next policy measures everybody else's, and a requester of no organisation is never taken by one.
func TestAPolicyForOrganisationsMeasuresOnlyTheirRequests(t *testing.T) {
	in := policyInput()
	in.Key, in.Organisations = "key_accounts", []string{"acme", " acme", "globex"}
	vip, err := NewSLAPolicy(in)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(vip.Organisations, []string{"acme", "globex"}) {
		t.Errorf("organisations %v", vip.Organisations)
	}
	everyone, _ := NewSLAPolicy(policyInput())
	everyone.Position = 1
	for organisation, want := range map[string]string{"acme": "key_accounts", "globex": "key_accounts", "initech": "incidents", "": "incidents"} {
		if chosen, _ := Choose([]SLAPolicy{everyone, vip}, "incident", organisation); chosen.Key != want {
			t.Errorf("a requester of %q is measured by %q, want %q", organisation, chosen.Key, want)
		}
	}
	in.Organisations = make([]string, MaxPolicyOrganisations+1)
	for i := range in.Organisations {
		in.Organisations[i] = "org_" + strconv.Itoa(i)
	}
	if _, err := NewSLAPolicy(in); shared.AsError(err).DetailCode != "sla_policies.too_many" {
		t.Errorf("too many organisations: %v", err)
	}
}

func TestGoalOfTimer(t *testing.T) {
	cases := []struct {
		name string
		kind GoalKind
		warn bool
		ok   bool
	}{
		{"first_response", GoalFirstResponse, false, true},
		{"resolution_warn", GoalResolution, true, true},
		{"review", "", false, false},
	}
	for _, c := range cases {
		kind, warn, ok := GoalOfTimer(c.name)
		if kind != c.kind || warn != c.warn || ok != c.ok {
			t.Errorf("%s: got %s %v %v", c.name, kind, warn, ok)
		}
	}
}

func TestStandingOf(t *testing.T) {
	cases := map[calendar.TimerState]Standing{
		calendar.TimerRunning: StandingRunning, calendar.TimerPaused: StandingPaused,
		calendar.TimerStopped: StandingMet, calendar.TimerFired: StandingBreached,
	}
	for state, want := range cases {
		if got := StandingOf(calendar.Timer{State: state}); got != want {
			t.Errorf("%s: got %s", state, got)
		}
	}
}

// UC-SVC-01 check 1: a calendar is working hours, holidays and a zone under a key.
func TestNewCalendar(t *testing.T) {
	var week calendar.Week
	for day := time.Monday; day <= time.Friday; day++ {
		week[day] = []calendar.Span{{From: 8 * 60, To: 18 * 60}}
	}
	in := NewCalendarInput{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000005a1"), TenantID: shared.MustParseID("0192f000-0000-7000-8000-0000000005a2"), Key: "office", Name: "Office hours",
		Spec: calendar.Spec{Zone: "Europe/Berlin", Week: week, Holidays: []calendar.Date{{Year: 2026, Month: 12, Day: 25}}},
		Text: text.Composing{},
	}
	cal, err := NewCalendar(in)
	if err != nil || cal.Key != "office" || cal.Calendar.IsZero() {
		t.Fatalf("a sound calendar was refused: %v", err)
	}
	changed, err := cal.Change("Office", in.Spec, time.Now(), text.Composing{})
	if err != nil || changed.Key != "office" || changed.Name != "Office" {
		t.Errorf("change lost the key: %+v %v", changed, err)
	}
	bad := in
	bad.Spec.Zone = "Mars/Olympus"
	if _, err := NewCalendar(bad); err == nil {
		t.Error("an unknown zone was taken")
	}
	bad = in
	bad.Key = "9office"
	if _, err := NewCalendar(bad); err == nil {
		t.Error("a malformed key was taken")
	}
}

// UC-SVC-02 check 8: a next response waits for the requester, and a met one is armed again by
// their next words; a breached one stays breached, a live one keeps its round.
func TestANextResponseIsArmedAgainOnlyOnceMet(t *testing.T) {
	if !GoalNextResponse.Valid() || !GoalNextResponse.WaitsForRequester() || GoalFirstResponse.WaitsForRequester() {
		t.Fatal("NEXT_RESPONSE is not a goal that waits for the requester")
	}
	monday := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	met := monday.Add(time.Hour)
	breached := monday.Add(2 * time.Hour)
	cases := []struct {
		name  string
		was   EntrySLA
		again bool
	}{
		{"met", EntrySLA{Goal: GoalNextResponse, Standing: StandingMet, MetAt: &met, AtRiskAt: &met}, true},
		{"breached", EntrySLA{Goal: GoalNextResponse, Standing: StandingBreached, BreachedAt: &breached}, false},
		{"running", EntrySLA{Goal: GoalNextResponse, Standing: StandingRunning}, false},
		{"paused", EntrySLA{Goal: GoalNextResponse, Standing: StandingPaused}, false},
	}
	for _, c := range cases {
		next, again := c.was.Rearm(30*time.Minute, breached)
		if again != c.again {
			t.Errorf("%s: armed again %v, want %v", c.name, again, c.again)
			continue
		}
		if again && (next.MetAt != nil || next.AtRiskAt != nil || next.Target != 30*time.Minute ||
			!next.ArmedAt.Equal(breached) || next.Standing != StandingRunning) {
			t.Errorf("%s: armed again as %+v", c.name, next)
		}
	}
}

// UC-SVC-02 check 10: a policy naming an assignee group is the group's OLA - never the requester's
// promise, every one covering the type measured beside it, and without a next response or
// organisations, which are the requester's.
func TestAnOlaIsMeasuredBesideTheRequestersPromise(t *testing.T) {
	network := shared.MustParseID("0192f000-0000-7000-8000-0000000005b1")
	in := policyInput()
	in.Key, in.AssigneeGroup, in.Position = "network_team", network, -5
	ola, err := NewSLAPolicy(in)
	if err != nil || !ola.IsOLA() || ola.AssigneeGroup != network {
		t.Fatalf("an OLA was refused or lost its group: %+v %v", ola, err)
	}
	in.Key = "another_team"
	other, _ := NewSLAPolicy(in)
	sla, _ := NewSLAPolicy(policyInput())
	sla.Position = 3
	if chosen, ok := Choose([]SLAPolicy{ola, sla, other}, "incident", ""); !ok || chosen.Key != "incidents" {
		t.Errorf("the requester's promise is %q, want incidents", chosen.Key)
	}
	if chosen, ok := Choose([]SLAPolicy{ola}, "incident", ""); ok {
		t.Errorf("an OLA alone measured the requester's promise: %q", chosen.Key)
	}
	olas := OLAsFor([]SLAPolicy{ola, sla, other}, "incident")
	if len(olas) != 2 || olas[0].Key != "another_team" || olas[1].Key != "network_team" {
		t.Errorf("the OLAs are %+v", olas)
	}
	if got := OLAsFor([]SLAPolicy{ola}, "task"); len(got) != 0 {
		t.Errorf("an OLA measured a type it does not cover")
	}
	for name, edit := range map[string]func(*NewSLAPolicyInput){
		"sla_policies.ola_goal_unsupported": func(in *NewSLAPolicyInput) {
			in.Goals = append(in.Goals, SLAGoal{Kind: GoalNextResponse, Minutes: 30})
		},
		"sla_policies.ola_organisations": func(in *NewSLAPolicyInput) { in.Organisations = []string{"acme"} },
	} {
		in := policyInput()
		in.AssigneeGroup = network
		edit(&in)
		if _, err := NewSLAPolicy(in); shared.AsError(err).DetailCode != name {
			t.Errorf("want %s, got %v", name, err)
		}
	}
}
