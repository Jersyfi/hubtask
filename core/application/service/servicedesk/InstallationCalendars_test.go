// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/calendar"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

func installationEntry(key string) map[string]any {
	return map[string]any{
		"key": key, "name": "Federal", "zone": "Europe/Berlin",
		"hours":    []any{map[string]any{"day": "MONDAY", "from": "08:00", "to": "16:00"}},
		"holidays": []any{"2026-12-25"},
	}
}

// UC-SVC-01 check 6: the instance file's calendars are read in the API's shape and checked as a
// workspace's are; one malformed entry or a key named twice refuses the whole list.
func TestTheInstallationsCalendarsAreReadWholeOrNotAtAll(t *testing.T) {
	read, err := InstallationCalendars([]map[string]any{installationEntry("federal")}, text.Composing{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	federal, ok := read["federal"]
	if !ok || federal.Name != "Federal" || federal.Calendar.Spec().Zone != "Europe/Berlin" ||
		len(federal.Calendar.Spec().Holidays) != 1 {
		t.Fatalf("read %+v", read)
	}
	for name, entries := range map[string][]map[string]any{
		"a key twice":     {installationEntry("federal"), installationEntry("federal")},
		"a bad key":       {installationEntry("Federal Days")},
		"no working time": {{"key": "x", "name": "X", "zone": "UTC"}},
		"an unknown zone": {{"key": "x", "name": "X", "zone": "Mars/Base", "hours": installationEntry("x")["hours"]}},
		"a bad holiday":   {{"key": "x", "name": "X", "zone": "UTC", "hours": installationEntry("x")["hours"], "holidays": []any{"soon"}}},
	} {
		if _, err := InstallationCalendars(entries, text.Composing{}); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%s answered %v", name, err)
		}
	}
}

// UC-SVC-01 check 6: a reader of a calendar by key meets the workspace's own of that key first and
// the installation's where the workspace holds none; a key neither holds is not found.
func TestAWorkspaceCalendarFallsBackToTheInstallations(t *testing.T) {
	var week calendar.Week
	week[1] = []calendar.Span{{From: 9 * 60, To: 17 * 60}}
	own, err := domain.NewInstallationCalendar("federal", "Ours", calendar.Spec{Zone: "UTC", Week: week}, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	offered, err := InstallationCalendars([]map[string]any{installationEntry("federal"), installationEntry("bavaria")}, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	reader := WithInstallation{Calendars: calendarStore{office: own}, Installation: offered}

	if found, err := reader.Find(context.Background(), "federal"); err != nil || found.Name != "Ours" {
		t.Fatalf("the workspace's own: %+v, %v", found, err)
	}
	if found, err := reader.Find(context.Background(), "bavaria"); err != nil || found.Name != "Federal" {
		t.Fatalf("the installation's: %+v, %v", found, err)
	}
	if _, err := reader.Find(context.Background(), "nowhere"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a key nobody holds answered %v", err)
	}
}

// UC-SVC-01 check 6: a policy may name a calendar the installation offers and none that nobody
// holds; a workspace calendar a policy names is deleted only where the installation offers one of
// its key; and an entry whose policy names a calendar gone from the instance file is kept, its
// goals not armed.
func TestAPolicyNamesACalendarTheInstallationOffers(t *testing.T) {
	f := newFixture(t)
	offered, err := InstallationCalendars([]map[string]any{installationEntry("bavaria")}, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	office := f.clocks.Calendars.(calendarStore).office
	named := f.clocks.Policies.(policyStore).rows
	desk := Desk{
		Calendars: calendarStore{office: office}, Offered: offered, Policies: policyStore{rows: named},
		Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday), IDs: &seqIDs{},
	}
	goals := []domain.SLAGoal{{Kind: domain.GoalResolution, Minutes: 600}}
	define := func(calendar string) error {
		_, err := DefineSlaPolicy{Desk: desk}.Execute(context.Background(), reader(), DefineSLAPolicyCommand{
			Key: "changes", Name: "Changes", CalendarKey: calendar, EntryTypes: []string{"change"}, Goals: goals, Enabled: true,
		})
		return err
	}
	if err := define("bavaria"); err != nil {
		t.Fatalf("a policy on the installation's calendar: %v", err)
	}
	if err := define("nowhere"); shared.AsError(err).DetailCode != "sla_policies.calendar_unknown" {
		t.Fatalf("a policy on a calendar nobody holds: %v", err)
	}
	remove := DeleteBusinessCalendar{Desk: desk}
	if err := remove.Execute(context.Background(), reader(), "office"); shared.AsError(err).DetailCode != "calendars.in_use" {
		t.Fatalf("a named calendar was deleted: %v", err)
	}
	desk.Offered = map[string]domain.Calendar{"office": office}
	if err := (DeleteBusinessCalendar{Desk: desk}).Execute(context.Background(), reader(), "office"); err != nil {
		t.Fatalf("a named calendar the installation also offers: %v", err)
	}

	f.clocks.Calendars = WithInstallation{Calendars: calendarStore{}, Installation: offered}
	f.deliver(t, event.ItemCreated, monday, nil)
	if len(f.timers.rows) != 0 {
		t.Fatalf("goals armed on a calendar nobody holds: %+v", f.timers.rows)
	}
}

// UC-SVC-01 check 5: the list answers the installation's calendars where it is asked to - those of a
// key the workspace holds none under, each marked offered, by key - so a hub's administrator finds
// them to keep a version of; asked plainly, it answers the workspace's alone.
func TestTheListAnswersTheInstallationsCalendarsWhenAsked(t *testing.T) {
	f := newFixture(t)
	office := f.clocks.Calendars.(calendarStore).office
	offered, err := InstallationCalendars([]map[string]any{installationEntry("bavaria"), installationEntry(office.Key)}, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	desk := Desk{
		Calendars: calendarStore{office: office}, Offered: offered,
		Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday), IDs: &seqIDs{},
	}
	list := ListBusinessCalendars{Desk: desk}.Descriptor().Handler
	keys := func(in usecase.Input) []string {
		out, err := list.Invoke(context.Background(), reader(), in)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, row := range out["data"].([]usecase.Output) {
			got = append(got, fmt.Sprintf("%s offered=%v", row["key"], row["offered"]))
		}
		return got
	}
	if got := keys(usecase.Input{}); !slices.Equal(got, []string{office.Key + " offered=<nil>"}) {
		t.Errorf("asked plainly: %v", got)
	}
	want := []string{"bavaria offered=true", office.Key + " offered=<nil>"}
	if got := keys(usecase.Input{"include_offered": true}); !slices.Equal(got, want) {
		t.Errorf("with the installation's: %v, want %v", got, want)
	}
}

// A RELATIVE_DATE rule's calendar is read as the SLA clocks read a policy's: the hub's own version
// of the key first, the workspace's or the installation's otherwise.
func TestARulesCalendarIsTheHubsOwnFirst(t *testing.T) {
	var week calendar.Week
	week[1] = []calendar.Span{{From: 9 * 60, To: 17 * 60}}
	workspace, err := domain.NewInstallationCalendar("office", "Workspace", calendar.Spec{Zone: "UTC", Week: week}, text.Composing{})
	if err != nil {
		t.Fatal(err)
	}
	hubs := &hubCalendarStore{}
	hubs.rows = append(hubs.rows, domain.Calendar{HubID: deskHub, Key: "office", Name: "Hub", Calendar: mustCalendar(t, afternoons())})
	rules := RuleCalendars{Calendars: WithInstallation{Calendars: calendarStore{office: workspace}}, HubCalendars: hubs}
	ctx := context.Background()

	if found, err := rules.Find(ctx, deskHub, "office"); err != nil || !found.WorksOn(2026, time.November, 2) || found.Spec().Week[1][0].From != afternoons().Week[1][0].From {
		t.Errorf("the hub's own: %+v, %v", found.Spec(), err)
	}
	if found, err := rules.Find(ctx, shared.ID("01936f2a-7c1e-7000-8000-0000000000ff"), "office"); err != nil || found.Spec().Week[1][0].From != 9*60 {
		t.Errorf("the workspace's: %+v, %v", found.Spec(), err)
	}
	if _, err := rules.Find(ctx, deskHub, "nowhere"); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a key nobody holds answered %v", err)
	}
}

func mustCalendar(t *testing.T, spec calendar.Spec) calendar.BusinessCalendar {
	t.Helper()
	cal, err := calendar.New(spec)
	if err != nil {
		t.Fatal(err)
	}
	return cal
}

type rulesOn map[string]int

func (r rulesOn) Calendar(_ context.Context, key string) (int, error) { return r[key], nil }

// A calendar no policy names but a switched-on rule's trigger counts on is refused with the count:
// deleting it would stop the auto-close rule without a word (configuration.md §2).
func TestACalendarARuleCountsOnIsNotDeleted(t *testing.T) {
	office := domain.Calendar{Key: "office", Name: "Office"}
	desk := Desk{
		Calendars: calendarStore{office: office}, Policies: policyStore{}, Rules: rulesOn{"office": 2},
		Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday), IDs: &seqIDs{},
	}
	err := (DeleteBusinessCalendar{Desk: desk}).Execute(context.Background(), reader(), "office")
	refusal := shared.AsError(err)
	if refusal.DetailCode != "calendars.named_by_rules" || refusal.Params["rules"] != "2" {
		t.Fatalf("a calendar two rules count on: %v", err)
	}
	desk.Rules = rulesOn{}
	if err := (DeleteBusinessCalendar{Desk: desk}).Execute(context.Background(), reader(), "office"); err != nil {
		t.Fatalf("a calendar nothing names: %v", err)
	}
}
