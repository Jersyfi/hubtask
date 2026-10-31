// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"testing"
	"time"

	calendarservice "github.com/Jersyfi/hubtask/core/application/service/calendar"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var (
	networkGroup = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000006a1")
	networker    = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000006a2")
)

type olaStore struct{ rows []domain.EntrySLA }

func (s *olaStore) SaveOLA(_ context.Context, record domain.EntrySLA, _ time.Time) error {
	for i, row := range s.rows {
		if row.ItemID == record.ItemID && row.PolicyKey == record.PolicyKey && row.Goal == record.Goal {
			s.rows[i] = record
			return nil
		}
	}
	s.rows = append(s.rows, record)
	return nil
}

func (s *olaStore) OLAsOf(_ context.Context, id shared.ID) ([]domain.EntrySLA, error) {
	var out []domain.EntrySLA
	for _, row := range s.rows {
		if row.ItemID == id {
			out = append(out, row)
		}
	}
	return out, nil
}

func (s *olaStore) FindOLA(_ context.Context, id shared.ID) (domain.EntrySLA, error) {
	for _, row := range s.rows {
		if row.ID == id {
			return row, nil
		}
	}
	return domain.EntrySLA{}, shared.ErrNotFound
}

func (s *olaStore) goal(kind domain.GoalKind) domain.EntrySLA {
	for _, row := range s.rows {
		if row.Goal == kind {
			return row
		}
	}
	return domain.EntrySLA{}
}

type groupStore map[shared.ID][]shared.ID

func (g groupStore) Members(_ context.Context, id shared.ID) ([]shared.ID, error) {
	members, ok := g[id]
	if !ok {
		return nil, shared.ErrNotFound
	}
	return members, nil
}

// olaFixture is the SLA fixture with the network team's OLA beside the requester's policy: a
// first response in 30 and a resolution in 120 working minutes, paused while waiting.
func olaFixture(t *testing.T) (fixture, OLAClocks, *olaStore) {
	t.Helper()
	f := newFixture(t)
	ola, err := domain.NewSLAPolicy(domain.NewSLAPolicyInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000006c2"), TenantID: tenantID, Key: "network_team",
		Name: "Network team", CalendarKey: "office", EntryTypes: []string{"incident"}, AssigneeGroup: networkGroup,
		Enabled: true, Now: monday, Text: text.Composing{},
		Goals: []domain.SLAGoal{
			{Kind: domain.GoalFirstResponse, Minutes: 30},
			{Kind: domain.GoalResolution, Minutes: 120, PauseStatuses: []string{"waiting"}, WarnPercent: 50},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	policies := f.clocks.Policies.(policyStore)
	policies.rows = append(policies.rows, ola)
	f.clocks.Policies = policies
	store := &olaStore{}
	clocks := OLAClocks{
		Calendars: f.clocks.Calendars, Policies: policies, Records: store, Items: f.items, IDs: f.ids,
		Groups: groupStore{networkGroup: {networker}}, Timers: calendarservice.Clocks{Timers: f.timers, IDs: f.ids},
	}
	return f, clocks, store
}

func deliverTo(t *testing.T, f fixture, s interface {
	Deliver(context.Context, event.Envelope) error
}, eventType event.Type, at time.Time, payload map[string]any) {
	t.Helper()
	if err := s.Deliver(context.Background(), event.Envelope{
		ID: f.ids.NewID(), Type: eventType, TenantID: tenantID, Subject: event.ItemSubject(itemID),
		OccurredAt: at, Payload: payload,
	}); err != nil {
		t.Fatalf("%s: %v", eventType, err)
	}
}

// UC-SVC-02 check 10: an OLA is armed when the entry is on a member of its group, beside the
// requester's promise, which it neither replaces nor moves; a member's internal note meets its
// first response.
func TestAnOlaIsArmedWhileTheEntryIsOnTheGroup(t *testing.T) {
	f, clocks, olas := olaFixture(t)
	f.deliver(t, event.ItemCreated, monday, nil)
	deliverTo(t, f, clocks, event.ItemCreated, monday, nil)
	if len(olas.rows) != 0 {
		t.Fatalf("an entry nobody holds is measured by the OLA: %+v", olas.rows)
	}
	if f.records.goal(domain.GoalFirstResponse).PolicyKey != "incidents" {
		t.Fatalf("the requester's promise is not the SLA's")
	}

	f.items.item.AssigneeID = networker
	deliverTo(t, f, clocks, event.ItemUpdated, monday.Add(10*time.Minute), map[string]any{"change_set": map[string]any{"assignee_id": nil}})
	first := olas.goal(domain.GoalFirstResponse)
	if first.Standing != domain.StandingRunning || first.Group != networkGroup || first.PolicyKey != "network_team" ||
		!first.DueAt.Equal(monday.Add(40*time.Minute)) {
		t.Fatalf("the OLA's first response is %+v", first)
	}
	if olas.goal(domain.GoalResolution).Target != 2*time.Hour {
		t.Errorf("the OLA's resolution is %+v", olas.goal(domain.GoalResolution))
	}
	if len(f.records.rows) != 2 {
		t.Errorf("the OLA wrote into the requester's records: %+v", f.records.rows)
	}

	deliverTo(t, f, clocks, event.CommentCreated, monday.Add(20*time.Minute), map[string]any{
		"item_id": itemID.String(), "author_id": handlerID.String(), "internal": true,
	})
	if olas.goal(domain.GoalFirstResponse).Standing != domain.StandingRunning {
		t.Error("a comment from outside the group met the team's first response")
	}
	deliverTo(t, f, clocks, event.CommentCreated, monday.Add(25*time.Minute), map[string]any{
		"item_id": itemID.String(), "author_id": networker.String(), "internal": true,
	})
	if got := olas.goal(domain.GoalFirstResponse); got.Standing != domain.StandingMet {
		t.Errorf("the member's note did not meet the team's first response: %+v", got)
	}
	if f.records.goal(domain.GoalFirstResponse).Standing != domain.StandingRunning {
		t.Error("the team's note moved the requester's clock")
	}
}

// UC-SVC-02 check 11: handing the entry on to somebody outside the group meets the OLA; handed
// back, it is not measured again; a paused status stands it still; the completion meets the rest.
func TestAnOlaEndsWhenTheEntryIsHandedOn(t *testing.T) {
	f, clocks, olas := olaFixture(t)
	f.items.item.AssigneeID = networker
	deliverTo(t, f, clocks, event.ItemCreated, monday, nil)

	f.items.item.Status = "waiting"
	deliverTo(t, f, clocks, event.ItemUpdated, monday.Add(30*time.Minute), nil)
	if got := olas.goal(domain.GoalResolution); got.Standing != domain.StandingPaused {
		t.Fatalf("the team's resolution did not stand still while waiting: %+v", got)
	}
	f.items.item.Status = "open"
	f.items.item.AssigneeID = handlerID
	deliverTo(t, f, clocks, event.ItemUpdated, monday.Add(time.Hour), nil)
	for _, kind := range []domain.GoalKind{domain.GoalFirstResponse, domain.GoalResolution} {
		if got := olas.goal(kind); got.Standing != domain.StandingMet {
			t.Errorf("%s: handed on, the team's goal is %+v", kind, got)
		}
	}
	f.items.item.AssigneeID = networker
	deliverTo(t, f, clocks, event.ItemUpdated, monday.Add(2*time.Hour), nil)
	if len(olas.rows) != 2 {
		t.Errorf("handed back, the entry was measured again: %+v", olas.rows)
	}

	g, clocks, olas := olaFixture(t)
	g.items.item.AssigneeID = networker
	deliverTo(t, g, clocks, event.ItemCreated, monday, nil)
	deliverTo(t, g, clocks, event.ItemCompleted, monday.Add(time.Hour), nil)
	if got := olas.goal(domain.GoalResolution); got.Standing != domain.StandingMet {
		t.Errorf("the completion did not meet the team's resolution: %+v", got)
	}
}

// UC-SVC-02 check 12: an OLA's goal running out is breached and announced with its group, so that
// a rule escalates it; a group that is gone measures nobody.
func TestAnOlaBreachIsAnnouncedWithItsGroup(t *testing.T) {
	f, clocks, olas := olaFixture(t)
	f.items.item.AssigneeID = networker
	deliverTo(t, f, clocks, event.ItemCreated, monday, nil)
	elapsed := OLAElapsed{Records: olas, Items: f.items, Events: f.events, IDs: f.ids}
	firing := calendarservice.FireTimers{
		Timers: f.timers, Clock: at(monday.Add(3 * time.Hour)), BatchSize: 10,
		Elapsed: map[calendar.Purpose]calendarservice.TimerElapsed{calendar.PurposeOLA: elapsed},
	}
	if _, err := firing.Execute(context.Background(), appshared.ActorContext{TenantID: tenantID}); err != nil {
		t.Fatalf("firing: %v", err)
	}
	if got := olas.goal(domain.GoalResolution); got.Standing != domain.StandingBreached || got.AtRiskAt == nil {
		t.Fatalf("the team's resolution is %+v", got)
	}
	breached := 0
	for _, envelope := range f.events.events {
		if envelope.Type == event.ItemSLABreached && envelope.Payload["assignee_group_id"] == networkGroup.String() {
			breached++
		}
	}
	if breached != 2 {
		t.Errorf("%d breaches announced with the group, want first response and resolution: %+v", breached, f.events.events)
	}

	g, clocks, olas := olaFixture(t)
	clocks.Groups = groupStore{}
	g.items.item.AssigneeID = networker
	deliverTo(t, g, clocks, event.ItemCreated, monday, nil)
	if len(olas.rows) != 0 {
		t.Errorf("a deleted group's OLA measured the entry: %+v", olas.rows)
	}
}

// UC-SVC-02 checks 10 and 12: an OLA names a group the workspace holds and reads it back; an
// entry's goals and the report carry the teams' beside the requester's, each marked by kind.
func TestAnOlaIsDefinedForAGroupAndReadBesideTheSla(t *testing.T) {
	define := DefineSlaPolicy{Desk: Desk{
		Policies: policyStore{}, Groups: groupStore{networkGroup: nil},
		Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday), IDs: &seqIDs{},
		Text: text.Composing{},
	}}
	command := DefineSLAPolicyCommand{
		Key: "network_team", Name: "Network team", EntryTypes: []string{"incident"}, Enabled: true,
		AssigneeGroup: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000006ff"),
		Goals:         []domain.SLAGoal{{Kind: domain.GoalResolution, Minutes: 120}},
	}
	if _, err := define.Execute(context.Background(), reader(), command); shared.AsError(err).DetailCode != "sla_policies.group_unknown" {
		t.Errorf("a group nobody holds: %v", err)
	}
	command.AssigneeGroup = networkGroup
	policy, err := define.Execute(context.Background(), reader(), command)
	if err != nil {
		t.Fatal(err)
	}
	if out := PolicyOutput(policy); out["assignee_group"] != networkGroup.String() {
		t.Errorf("output %v", out["assignee_group"])
	}
	if out := PolicyOutput(domain.SLAPolicy{Key: "incidents"}); out["assignee_group"] != nil {
		t.Errorf("the requester's policy names a group: %v", out["assignee_group"])
	}

	sla := RecordOutput(domain.EntrySLA{PolicyKey: "incidents", Goal: domain.GoalResolution})
	ola := RecordOutput(domain.EntrySLA{PolicyKey: "network_team", Goal: domain.GoalResolution, Group: networkGroup})
	if sla["kind"] != "SLA" || sla["assignee_group"] != nil || ola["kind"] != "OLA" || ola["assignee_group"] != networkGroup.String() {
		t.Errorf("the goals read %v and %v", sla, ola)
	}
}
