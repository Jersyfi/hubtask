// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	calendarservice "github.com/Jersyfi/hubtask/core/application/service/calendar"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var (
	tenantID    = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005f1")
	itemID      = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005f2")
	requesterID = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005f3")
	handlerID   = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005f4")
	collection  = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005f5")
)

type seqIDs struct{ n int }

func (s *seqIDs) NewID() shared.ID {
	s.n++
	return shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-8000-%012x", 0x9000+s.n))
}

type timerStore struct{ rows []calendar.Timer }

func liveTimer(t calendar.Timer) bool {
	return t.State == calendar.TimerRunning || t.State == calendar.TimerPaused
}

func (s *timerStore) Insert(ctx context.Context, timer calendar.Timer, _ time.Time) error {
	if _, err := s.FindLive(ctx, timer.Purpose, timer.SubjectID, timer.Name); err == nil {
		return shared.ErrConflict.WithDetail("timer.already_running")
	}
	s.rows = append(s.rows, timer)
	return nil
}

func (s *timerStore) FindLive(_ context.Context, purpose calendar.Purpose, subject shared.ID, name string) (calendar.Timer, error) {
	for _, row := range s.rows {
		if row.Purpose == purpose && row.SubjectID == subject && row.Name == name && liveTimer(row) {
			return row, nil
		}
	}
	return calendar.Timer{}, shared.ErrNotFound
}

func (s *timerStore) Update(_ context.Context, timer calendar.Timer, expected int, _ time.Time) error {
	for i, row := range s.rows {
		if row.ID == timer.ID {
			if row.Version != expected {
				return shared.ErrVersionConflict
			}
			timer.Version = expected + 1
			s.rows[i] = timer
			return nil
		}
	}
	return shared.ErrNotFound
}

func (s *timerStore) ClaimDue(_ context.Context, now time.Time, _ int) ([]calendar.Timer, error) {
	var due []calendar.Timer
	for _, row := range s.rows {
		if row.State == calendar.TimerRunning && !row.DueAt.After(now) {
			due = append(due, row)
		}
	}
	slices.SortFunc(due, func(a, b calendar.Timer) int { return a.DueAt.Compare(*b.DueAt) })
	return due, nil
}

func (s *timerStore) NextDue(context.Context) (*time.Time, error) { return nil, nil }

func (s *timerStore) named(name string) calendar.Timer {
	for _, row := range slices.Backward(s.rows) {
		if row.Name == name {
			return row
		}
	}
	return calendar.Timer{}
}

type recordStore struct{ rows []domain.EntrySLA }

func (s *recordStore) Save(_ context.Context, record domain.EntrySLA, _ time.Time) error {
	for i, row := range s.rows {
		if row.ItemID == record.ItemID && row.Goal == record.Goal {
			s.rows[i] = record
			return nil
		}
	}
	s.rows = append(s.rows, record)
	return nil
}

func (s *recordStore) OfItem(_ context.Context, id shared.ID) ([]domain.EntrySLA, error) {
	var out []domain.EntrySLA
	for _, row := range s.rows {
		if row.ItemID == id {
			out = append(out, row)
		}
	}
	return out, nil
}

func (s *recordStore) Report(context.Context, time.Time, time.Time) ([]domain.ReportRow, error) {
	return nil, nil
}

func (s *recordStore) goal(kind domain.GoalKind) domain.EntrySLA {
	for _, row := range s.rows {
		if row.Goal == kind {
			return row
		}
	}
	return domain.EntrySLA{}
}

type policyStore struct{ rows []domain.SLAPolicy }

func (p policyStore) Insert(context.Context, domain.SLAPolicy) error      { return nil }
func (p policyStore) Update(context.Context, domain.SLAPolicy, int) error { return nil }
func (p policyStore) Find(context.Context, string) (domain.SLAPolicy, error) {
	return domain.SLAPolicy{}, shared.ErrNotFound
}
func (p policyStore) List(context.Context) ([]domain.SLAPolicy, error) { return p.rows, nil }
func (p policyStore) Delete(context.Context, string) error             { return nil }

type calendarStore struct{ office domain.Calendar }

func (c calendarStore) Insert(context.Context, domain.Calendar) error      { return nil }
func (c calendarStore) Update(context.Context, domain.Calendar, int) error { return nil }
func (c calendarStore) Find(_ context.Context, key string) (domain.Calendar, error) {
	if key == c.office.Key {
		return c.office, nil
	}
	return domain.Calendar{}, shared.ErrNotFound
}
func (c calendarStore) List(context.Context) ([]domain.Calendar, error) { return nil, nil }
func (c calendarStore) Delete(context.Context, string) error            { return nil }

type itemStore struct {
	workrepo.Items
	item work.WorkItem
}

func (s *itemStore) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	if id != s.item.ID {
		return work.WorkItem{}, shared.ErrNotFound
	}
	return s.item, nil
}

type eventLog struct{ events []event.Envelope }

func (l *eventLog) Append(_ context.Context, envelope event.Envelope) error {
	l.events = append(l.events, envelope)
	return nil
}

type fixture struct {
	timers  *timerStore
	records *recordStore
	items   *itemStore
	events  *eventLog
	clocks  SLAClocks
	ids     *seqIDs
}

// Monday 2 November 2026, 09:00 in Berlin; the office is open 08:00-18:00 on weekdays.
var monday = time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)

func newFixture(t *testing.T) fixture {
	t.Helper()
	var week calendar.Week
	for day := time.Monday; day <= time.Friday; day++ {
		week[day] = []calendar.Span{{From: 8 * 60, To: 18 * 60}}
	}
	office, err := domain.NewCalendar(domain.NewCalendarInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005c1"), TenantID: tenantID, Key: "office",
		Name: "Office", Spec: calendar.Spec{Zone: "Europe/Berlin", Week: week}, Now: monday, Text: text.Composing{},
	})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := domain.NewSLAPolicy(domain.NewSLAPolicyInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005c2"), TenantID: tenantID, Key: "incidents",
		Name: "Incidents", CalendarKey: "office", EntryTypes: []string{"incident"}, PriorityField: "priority",
		Enabled: true, Now: monday, Text: text.Composing{},
		Goals: []domain.SLAGoal{
			{Kind: domain.GoalFirstResponse, Minutes: 60, ByPriority: map[string]int{"p1": 15}},
			{Kind: domain.GoalResolution, Minutes: 600, ByPriority: map[string]int{"p1": 240}, PauseStatuses: []string{"waiting"}, WarnPercent: 50},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := &seqIDs{}
	f := fixture{
		timers: &timerStore{}, records: &recordStore{}, events: &eventLog{}, ids: ids,
		items: &itemStore{item: work.WorkItem{
			ID: itemID, TenantID: tenantID, CollectionID: collection, EntryType: "incident", Status: "open",
			CreatedBy: requesterID, CustomFields: map[string]any{"priority": "p1"},
		}},
	}
	f.clocks = SLAClocks{
		Calendars: calendarStore{office: office}, Policies: policyStore{rows: []domain.SLAPolicy{policy}},
		Records: f.records, Items: f.items, IDs: ids,
		Timers: calendarservice.Clocks{Timers: f.timers, IDs: ids},
	}
	return f
}

func (f fixture) deliver(t *testing.T, eventType event.Type, at time.Time, payload map[string]any) {
	t.Helper()
	envelope := event.Envelope{
		ID: f.ids.NewID(), Type: eventType, TenantID: tenantID, Subject: event.ItemSubject(itemID),
		OccurredAt: at, Payload: payload,
	}
	if eventType == event.CommentCreated {
		envelope.Subject = "comment/" + f.ids.NewID().String()
	}
	if !f.clocks.Wants(eventType) {
		t.Fatalf("%s is not wanted", eventType)
	}
	if err := f.clocks.Deliver(context.Background(), envelope); err != nil {
		t.Fatalf("%s: %v", eventType, err)
	}
}

// UC-SVC-02 checks 2 and 3: an entry of a covered type gets its goals at its priority on the
// policy's calendar; the first response is met by somebody other than the requester.
func TestAnEntryIsArmedAndItsFirstResponseMet(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, event.ItemCreated, monday, nil)

	first := f.records.goal(domain.GoalFirstResponse)
	if first.Standing != domain.StandingRunning || first.Target != 15*time.Minute || first.PolicyKey != "incidents" {
		t.Fatalf("first response armed as %+v", first)
	}
	if !first.DueAt.Equal(monday.Add(15 * time.Minute)) {
		t.Errorf("due at %v, want 09:15 Berlin", first.DueAt)
	}
	resolution := f.records.goal(domain.GoalResolution)
	if resolution.Target != 4*time.Hour || !resolution.DueAt.Equal(monday.Add(4*time.Hour)) {
		t.Errorf("resolution armed as %+v", resolution)
	}
	if warn := f.timers.named("resolution_warn"); warn.Budget != 2*time.Hour || warn.State != calendar.TimerRunning {
		t.Errorf("the warning timer is %+v", warn)
	}

	f.deliver(t, event.CommentCreated, monday.Add(5*time.Minute), map[string]any{
		"item_id": itemID.String(), "author_id": requesterID.String(),
	})
	if f.records.goal(domain.GoalFirstResponse).Standing != domain.StandingRunning {
		t.Error("the requester's own comment met the first response")
	}
	f.deliver(t, event.CommentCreated, monday.Add(7*time.Minute), map[string]any{
		"item_id": itemID.String(), "author_id": handlerID.String(), "internal": true,
	})
	if f.records.goal(domain.GoalFirstResponse).Standing != domain.StandingRunning {
		t.Error("an internal note met the first response (UC-SVC-04 check 3)")
	}
	f.deliver(t, event.CommentCreated, monday.Add(10*time.Minute), map[string]any{
		"item_id": itemID.String(), "author_id": handlerID.String(),
	})
	if met := f.records.goal(domain.GoalFirstResponse); met.Standing != domain.StandingMet || met.MetAt == nil {
		t.Errorf("a handler's comment did not meet the first response: %+v", met)
	}
}

// UC-SVC-02 check 4: a status the goal pauses in stands its clock still; the time spent there
// does not count; the completion meets what is left.
func TestAGoalPausesWhileWaitingAndIsMetByCompletion(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, event.ItemCreated, monday, nil)

	f.items.item.Status = "waiting"
	f.deliver(t, event.ItemUpdated, monday.Add(time.Hour), map[string]any{"change_set": map[string]any{"status": map[string]any{}}})
	paused := f.records.goal(domain.GoalResolution)
	if paused.Standing != domain.StandingPaused || paused.DueAt != nil {
		t.Fatalf("waiting did not pause the resolution: %+v", paused)
	}
	if f.records.goal(domain.GoalFirstResponse).Standing != domain.StandingRunning {
		t.Error("a goal that does not pause in the status stood still")
	}

	f.items.item.Status = "open"
	f.deliver(t, event.ItemUpdated, monday.Add(3*time.Hour), map[string]any{"change_set": map[string]any{"status": map[string]any{}}})
	resumed := f.records.goal(domain.GoalResolution)
	// One working hour spent before the wait, three left from 12:00 Berlin: due 15:00 Berlin.
	if resumed.Standing != domain.StandingRunning || !resumed.DueAt.Equal(monday.Add(6*time.Hour)) {
		t.Fatalf("resumed as %+v", resumed)
	}

	f.items.item.Completion.IsCompleted = true
	f.deliver(t, event.ItemCompleted, monday.Add(4*time.Hour), nil)
	if met := f.records.goal(domain.GoalResolution); met.Standing != domain.StandingMet {
		t.Errorf("the completion did not meet the resolution: %+v", met)
	}
	for _, row := range f.timers.rows {
		if liveTimer(row) {
			t.Errorf("a clock still runs after the completion: %s", row.Name)
		}
	}
}

// UC-SVC-02 check 2: a type no policy covers gets no clock.
func TestAnUncoveredEntryGetsNoClock(t *testing.T) {
	f := newFixture(t)
	f.items.item.EntryType = "task"
	f.deliver(t, event.ItemCreated, monday, nil)
	if len(f.records.rows) != 0 || len(f.timers.rows) != 0 {
		t.Errorf("an uncovered entry was armed: %d records", len(f.records.rows))
	}
}

// UC-SVC-02 check 5: a warning timer marks the goal at risk, the goal's own breached; each is
// announced once.
func TestARunOutGoalIsAtRiskThenBreached(t *testing.T) {
	f := newFixture(t)
	f.deliver(t, event.ItemCreated, monday, nil)
	fire := calendarservice.FireTimers{
		Timers: f.timers,
		Elapsed: map[calendar.Purpose]calendarservice.TimerElapsed{
			calendar.PurposeSLA: SLAElapsed{Records: f.records, Items: f.items, Events: f.events, IDs: f.ids},
		},
	}
	actor := appshared.ActorContext{TenantID: tenantID, Kind: appshared.ActorSystem}

	fire.Clock = at(monday.Add(2 * time.Hour))
	if _, err := fire.Execute(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	resolution := f.records.goal(domain.GoalResolution)
	if resolution.AtRiskAt == nil || resolution.Standing != domain.StandingRunning {
		t.Errorf("not at risk at half its target: %+v", resolution)
	}
	if f.records.goal(domain.GoalFirstResponse).Standing != domain.StandingBreached {
		t.Error("the first response ran out unbreached")
	}

	fire.Clock = at(monday.Add(4 * time.Hour))
	if _, err := fire.Execute(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	if f.records.goal(domain.GoalResolution).Standing != domain.StandingBreached {
		t.Error("the resolution ran out unbreached")
	}
	var kinds []event.Type
	for _, envelope := range f.events.events {
		kinds = append(kinds, envelope.Type)
	}
	want := []event.Type{event.ItemSLABreached, event.ItemSLAAtRisk, event.ItemSLABreached}
	if !slices.Equal(kinds, want) {
		t.Errorf("announced %v, want %v", kinds, want)
	}

	f.items.item.Completion.IsCompleted = true
	f.deliver(t, event.ItemCompleted, monday.Add(5*time.Hour), nil)
	if f.records.goal(domain.GoalResolution).Standing != domain.StandingBreached {
		t.Error("a completion after the breach unmade it")
	}
}

func withNextResponse(t *testing.T, f *fixture) {
	t.Helper()
	policy, err := domain.NewSLAPolicy(domain.NewSLAPolicyInput{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005c2"), TenantID: tenantID, Key: "incidents",
		Name: "Incidents", CalendarKey: "office", EntryTypes: []string{"incident"}, Enabled: true, Now: monday,
		Text: text.Composing{},
		Goals: []domain.SLAGoal{
			{Kind: domain.GoalFirstResponse, Minutes: 60},
			{Kind: domain.GoalNextResponse, Minutes: 30},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.clocks.Policies = policyStore{rows: []domain.SLAPolicy{policy}}
}

// UC-SVC-02 check 8: the next response is armed by the requester's words once the first response
// is met, met by the team's next public reply - never by an internal note - and armed again.
func TestTheNextResponseWaitsForEachAnswerOfTheRequester(t *testing.T) {
	f := newFixture(t)
	withNextResponse(t, &f)
	reply := func(at time.Duration, author shared.ID, extra map[string]any) {
		payload := map[string]any{"item_id": itemID.String(), "author_id": author.String()}
		for k, v := range extra {
			payload[k] = v
		}
		f.deliver(t, event.CommentCreated, monday.Add(at), payload)
	}
	f.deliver(t, event.ItemCreated, monday, nil)
	reply(5*time.Minute, requesterID, nil)
	if next := f.records.goal(domain.GoalNextResponse); !next.ID.IsZero() {
		t.Fatalf("the next response ran before the first was met: %+v", next)
	}
	reply(10*time.Minute, handlerID, nil)
	reply(20*time.Minute, requesterID, nil)
	next := f.records.goal(domain.GoalNextResponse)
	if next.Standing != domain.StandingRunning || next.DueAt == nil || !next.DueAt.Equal(monday.Add(50*time.Minute)) {
		t.Fatalf("the next response armed as %+v", next)
	}
	reply(25*time.Minute, handlerID, map[string]any{"internal": true})
	if f.records.goal(domain.GoalNextResponse).Standing != domain.StandingRunning {
		t.Fatal("an internal note met the next response")
	}
	reply(30*time.Minute, handlerID, nil)
	if met := f.records.goal(domain.GoalNextResponse); met.Standing != domain.StandingMet {
		t.Fatalf("the team's reply did not meet the next response: %+v", met)
	}
	reply(40*time.Minute, requesterID, nil)
	again := f.records.goal(domain.GoalNextResponse)
	if again.Standing != domain.StandingRunning || !again.ArmedAt.Equal(monday.Add(40*time.Minute)) || again.MetAt != nil {
		t.Errorf("the next round armed as %+v", again)
	}
}

// An entry the mail intake filed is raised by the administrator who opened it: their own reply
// answers the requester, and the requester's filed words ask for the next answer.
func TestOnAnEntryFiledByMailTheOpenersReplyAnswers(t *testing.T) {
	f := newFixture(t)
	withNextResponse(t, &f)
	f.clocks.Requesters = requesterBook{itemID: {{Address: "kunde@shop.test"}}}
	f.deliver(t, event.ItemCreated, monday, nil)
	f.deliver(t, event.CommentCreated, monday.Add(10*time.Minute), map[string]any{
		"item_id": itemID.String(), "author_id": requesterID.String(),
	})
	if first := f.records.goal(domain.GoalFirstResponse); first.Standing != domain.StandingMet {
		t.Fatalf("the opener's reply did not meet the first response: %+v", first)
	}
	f.deliver(t, event.CommentCreated, monday.Add(20*time.Minute), map[string]any{
		"item_id": itemID.String(), "author_id": requesterID.String(), "by_requester": true,
	})
	if next := f.records.goal(domain.GoalNextResponse); next.Standing != domain.StandingRunning {
		t.Errorf("the requester's filed words did not arm the next response: %+v", next)
	}
}

// UC-SVC-02 check 9: a policy for some organisations measures their requesters' entries; the next
// one measures everybody else's - a requester of another organisation, of none, and one the mail
// intake filed for, who belongs to none.
func TestAnOrganisationsPolicyMeasuresItsRequestersEntries(t *testing.T) {
	for name, c := range map[string]struct {
		email  string
		filed  bool
		policy string
	}{
		"of the organisation":       {email: "mehmet@acme.example", policy: "key_accounts"},
		"of another organisation":   {email: "ann@globex.example", policy: "incidents"},
		"of none":                   {email: "bo@elsewhere.example", policy: "incidents"},
		"filed for by mail":         {email: "mehmet@acme.example", filed: true, policy: "incidents"},
		"an account no longer held": {policy: "incidents"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			everyone := f.clocks.Policies.(policyStore).rows[0]
			everyone.Position = 1
			vip := everyone
			vip.Key, vip.Position, vip.Organisations = "key_accounts", 0, []string{"acme"}
			f.clocks.Policies = policyStore{rows: []domain.SLAPolicy{everyone, vip}}
			emails := map[shared.ID]string{}
			if c.email != "" {
				emails[requesterID] = c.email
			}
			f.clocks.Accounts = addressBook{emails: emails}
			f.clocks.Organisations = &organisationStore{rows: map[string]domain.Organisation{
				"acme":   {Key: "acme", Domains: []string{"acme.example"}},
				"globex": {Key: "globex", Domains: []string{"globex.example"}},
			}}
			if c.filed {
				f.clocks.Requesters = requesterBook{itemID: {{Address: "kunde@shop.test"}}}
			}
			f.deliver(t, event.ItemCreated, monday, nil)
			if got := f.records.goal(domain.GoalFirstResponse).PolicyKey; got != c.policy {
				t.Errorf("measured by %q, want %q", got, c.policy)
			}
		})
	}
}
