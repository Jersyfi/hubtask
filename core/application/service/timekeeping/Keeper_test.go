// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	planrepo "github.com/Jersyfi/hubtask/core/application/repository/planning"
	timerepo "github.com/Jersyfi/hubtask/core/application/repository/timekeeping"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/timekeeping"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenantID  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000001")
	ana       = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000002")
	lead      = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000003")
	ben       = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000004")
	hubID     = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000010")
	listID    = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000011")
	otherHub  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000012")
	otherCol  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000013")
	checkout  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000101")
	payment   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000102")
	secret    = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000103")
	elsewhere = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000104")
)

// Monday 2 November 2026, 09:00 UTC.
var monday = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

type inline struct{}

func (inline) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (inline) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

// gate reads every entry but the hidden ones and lets an approver approve where they were made
// one: at the workspace (tenantID) or at a hub.
type gate struct {
	hidden    map[shared.ID]bool
	approvers map[shared.ID][]shared.ID
	refused   []access.Request
}

func (g *gate) approves(actor shared.ID, path []identity.Scope) bool {
	for _, scope := range path {
		id := scope.ID
		if scope.Type == identity.ScopeTenant {
			id = tenantID
		}
		if slices.Contains(g.approvers[actor], id) {
			return true
		}
	}
	return false
}

func (g *gate) Authorize(_ context.Context, actor appshared.ActorContext, r access.Request) error {
	if guarded(r.Permission) && !g.approves(actor.AccountID, r.Path) {
		g.refused = append(g.refused, r)
		return shared.ErrForbidden.WithDetail("access.denied")
	}
	if r.Permission == service.PermissionRead && g.hidden[r.TargetID] {
		g.refused = append(g.refused, r)
		return shared.ErrNotFound
	}
	return nil
}

func (g *gate) Permitted(_ context.Context, actor appshared.ActorContext, r access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i, path := range paths {
		if guarded(r.Permission) {
			out[i] = g.approves(actor.AccountID, path)
		} else {
			out[i] = !g.hidden[path[len(path)-1].ID]
		}
	}
	return out, nil
}

// guarded are the permissions only the approvers hold.
func guarded(p service.Permission) bool {
	return p == service.PermissionApproveTime || p == service.PermissionManageMembers || p == service.PermissionManageCosts
}

type sink struct{ entries []audit.Entry }

func (s *sink) Append(_ context.Context, e audit.Entry) error {
	s.entries = append(s.entries, e)
	return nil
}

type ids struct{ n int }

func (i *ids) NewID() shared.ID {
	i.n++
	return shared.MustParseID("01936f2a-7c1e-7000-8000-" + strings.Repeat("0", 8) + string(rune('a'+i.n/10)) + string(rune('0'+i.n%10)) + "00")
}

// the entries and their places
var places = map[shared.ID]workrepo.EntrySummary{
	checkout:  {ID: checkout, HubID: hubID, CollectionID: listID, Title: "Checkout form"},
	payment:   {ID: payment, HubID: hubID, CollectionID: listID, Title: "Payment"},
	secret:    {ID: secret, HubID: hubID, CollectionID: listID, Title: "Secret"},
	elsewhere: {ID: elsewhere, HubID: otherHub, CollectionID: otherCol, Title: "Elsewhere"},
}

type references struct{ workrepo.References }

func (references) Entries(_ context.Context, list []shared.ID) ([]workrepo.EntrySummary, error) {
	var out []workrepo.EntrySummary
	for _, id := range list {
		if s, ok := places[id]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}

type containers struct{ workrepo.Containers }

func (containers) Find(_ context.Context, id shared.ID) (work.Container, error) {
	if id == hubID {
		return work.Container{ID: id, Type: work.ContainerHub}, nil
	}
	return work.Container{ID: id, Type: work.ContainerCollection, ParentID: hubID}, nil
}

type plans struct{ planrepo.Plans }

func (plans) Entries(context.Context, shared.ID, int) ([]planrepo.PlanEntry, error) {
	return []planrepo.PlanEntry{
		{ID: checkout, HubID: hubID, CollectionID: listID, Title: "Checkout form"},
		{ID: payment, ParentID: checkout, HubID: hubID, CollectionID: listID, Title: "Payment"},
		{ID: secret, ParentID: checkout, HubID: hubID, CollectionID: listID, Title: "Secret"},
	}, nil
}

type estimates struct{ planrepo.Sprints }

func (estimates) Estimates(context.Context, []shared.ID) (map[shared.ID]planning.Estimate, error) {
	return map[shared.ID]planning.Estimate{
		checkout: {EntryID: checkout, Unit: planning.Hours, Value: 4},
		payment:  {EntryID: payment, Unit: planning.Hours, Value: 2},
		secret:   {EntryID: secret, Unit: planning.Points, Value: 3},
	}, nil
}

// store keeps the module's rows in memory.
type store struct {
	records map[shared.ID]domain.Record
	sheets  map[string]domain.Timesheet
	timers  map[shared.ID]domain.Timer
	policy  domain.Policy
	kinds   []domain.WorkType
}

func newStore() *store {
	return &store{
		records: map[shared.ID]domain.Record{}, sheets: map[string]domain.Timesheet{}, timers: map[shared.ID]domain.Timer{},
		policy: domain.DefaultPolicy(),
		kinds:  []domain.WorkType{{Key: "development", Name: "Development", Billable: true}, {Key: "meeting", Name: "Meeting"}},
	}
}

func sheetKey(a shared.ID, d domain.Day) string { return a.String() + domain.WeekOf(d).String() }

func (s *store) WorkTypes(context.Context) ([]domain.WorkType, error) { return s.kinds, nil }
func (s *store) PutWorkType(_ context.Context, _ shared.ID, w domain.WorkType, _ time.Time) error {
	for i, k := range s.kinds {
		if k.Key == w.Key {
			s.kinds[i] = w
			return nil
		}
	}
	s.kinds = append(s.kinds, w)
	return nil
}
func (s *store) InsertRecord(_ context.Context, r domain.Record) error {
	s.records[r.ID] = r
	return nil
}
func (s *store) UpdateRecord(_ context.Context, r domain.Record, expected int) error {
	if s.records[r.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	s.records[r.ID] = r
	return nil
}
func (s *store) DeleteRecord(_ context.Context, id shared.ID, expected int) error {
	if s.records[id].Version != expected {
		return shared.ErrVersionConflict
	}
	delete(s.records, id)
	return nil
}
func (s *store) FindRecord(_ context.Context, id shared.ID) (domain.Record, error) {
	r, ok := s.records[id]
	if !ok {
		return r, shared.ErrNotFound
	}
	return r, nil
}
func (s *store) placed(f timerepo.Filter) []timerepo.Placed {
	var out []timerepo.Placed
	for _, r := range s.records {
		p := places[r.EntryID]
		if r.Day < f.From || r.Day > f.To || (!f.AccountID.IsZero() && r.AccountID != f.AccountID) ||
			(!f.EntryID.IsZero() && r.EntryID != f.EntryID) || (!f.HubID.IsZero() && p.HubID != f.HubID) ||
			(!f.CollectionID.IsZero() && p.CollectionID != f.CollectionID) || (f.Billable != nil && r.Billable != *f.Billable) {
			continue
		}
		out = append(out, timerepo.Placed{Record: r, HubID: p.HubID, CollectionID: p.CollectionID, EntryTitle: p.Title})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (s *store) Records(_ context.Context, f timerepo.Filter, _ int) ([]timerepo.Placed, error) {
	return s.placed(f), nil
}
func (s *store) Lines(_ context.Context, f timerepo.Filter, _ int) ([]domain.Line, error) {
	var out []domain.Line
	for _, p := range s.placed(f) {
		out = append(out, domain.Line{
			AccountID: p.AccountID, EntryID: p.EntryID, CollectionID: p.CollectionID, HubID: p.HubID,
			WorkType: p.WorkType, Day: p.Day, Billable: p.Billable, Minutes: p.Minutes, Records: 1,
		})
	}
	return out, nil
}
func (s *store) MinutesOn(_ context.Context, list []shared.ID) (map[shared.ID]int, error) {
	out := map[shared.ID]int{}
	for _, r := range s.records {
		if slices.Contains(list, r.EntryID) {
			out[r.EntryID] += r.Minutes
		}
	}
	return out, nil
}
func (s *store) WeekMinutes(_ context.Context, a shared.ID, from, to domain.Day) (int, error) {
	total := 0
	for _, p := range s.placed(timerepo.Filter{From: from, To: to, AccountID: a}) {
		total += p.Minutes
	}
	return total, nil
}
func (s *store) Timer(_ context.Context, a shared.ID) (domain.Timer, bool, error) {
	t, ok := s.timers[a]
	return t, ok, nil
}
func (s *store) StartTimer(_ context.Context, _ shared.ID, t domain.Timer) error {
	if _, ok := s.timers[t.AccountID]; ok {
		return shared.ErrConflict.WithDetail("timers.running")
	}
	s.timers[t.AccountID] = t
	return nil
}
func (s *store) RemoveTimer(_ context.Context, a shared.ID) (bool, error) {
	_, ok := s.timers[a]
	delete(s.timers, a)
	return ok, nil
}
func (s *store) Sheet(_ context.Context, a shared.ID, d domain.Day) (domain.Timesheet, bool, error) {
	sheet, ok := s.sheets[sheetKey(a, d)]
	if !ok {
		return domain.OpenSheet(a, d), false, nil
	}
	return sheet, true, nil
}
func (s *store) Sheets(_ context.Context, a shared.ID, state domain.SheetState, from, to domain.Day, _ int) ([]domain.Timesheet, error) {
	var out []domain.Timesheet
	for _, sheet := range s.sheets {
		if (a.IsZero() || sheet.AccountID == a) && (state == "" || sheet.State == state) && sheet.WeekStart >= from && sheet.WeekStart <= to {
			out = append(out, sheet)
		}
	}
	return out, nil
}
func (s *store) PutSheet(_ context.Context, _ shared.ID, sheet domain.Timesheet, expected int, _ time.Time) error {
	if s.sheets[sheetKey(sheet.AccountID, sheet.WeekStart)].Version != expected {
		return shared.ErrVersionConflict
	}
	sheet.Version = expected + 1
	s.sheets[sheetKey(sheet.AccountID, sheet.WeekStart)] = sheet
	return nil
}
func (s *store) Policy(context.Context) (domain.Policy, error) { return s.policy, nil }
func (s *store) PutPolicy(_ context.Context, _ shared.ID, p domain.Policy, expected int) error {
	p.Version = expected + 1
	s.policy = p
	return nil
}

type fixture struct {
	keeper Keeper
	store  *store
	gate   *gate
	audit  *sink
	clock  *movingClock
}

type movingClock struct{ now time.Time }

func (c *movingClock) Now() time.Time { return c.now }

func newFixture() fixture {
	s, g, a := newStore(), &gate{
		hidden:    map[shared.ID]bool{secret: true},
		approvers: map[shared.ID][]shared.ID{lead: {hubID}},
	}, &sink{}
	c := &movingClock{now: monday}
	return fixture{
		store: s, gate: g, audit: a, clock: c,
		keeper: Keeper{
			Store: s, References: references{}, Containers: containers{}, Plans: plans{}, Estimates: estimates{},
			Authorizer: g, Reader: g, Audit: a, UnitOfWork: inline{}, Clock: c, IDs: &ids{},
		},
	}
}

var _ clock.Clock = (*movingClock)(nil)

func as(id shared.ID) appshared.ActorContext {
	return appshared.ActorContext{TenantID: tenantID, AccountID: id, Kind: appshared.ActorUser, Scopes: []string{itemsRead, itemsWrite, workspaceManage}}
}

func day(t *testing.T, text string) domain.Day {
	t.Helper()
	d, ok := domain.ParseDay(text)
	if !ok {
		t.Fatalf("day %q", text)
	}
	return d
}

func detail(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

func (f fixture) log(t *testing.T, who shared.ID, entry shared.ID, on string, minutes int) domain.Record {
	t.Helper()
	r, err := LogTime{Keeper: f.keeper}.Execute(context.Background(), as(who), RecordFields{
		EntryID: entry, WorkType: "development", Day: day(t, on), Minutes: minutes, Note: "private words",
	}, time.UTC)
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	return r
}

// UC-TIM-01 checks 1 and 2: a person records time on an entry they may see or on none, billable
// as its kind of work says; an entry out of reach is refused; the note never reaches the trail.
func TestAPersonRecordsTimeWhereTheWorkIs(t *testing.T) {
	f := newFixture()
	r := f.log(t, ana, checkout, "2026-11-02", 90)
	if !r.Billable || r.AccountID != ana || r.Source != domain.SourceManual {
		t.Fatalf("record %+v", r)
	}
	none, err := LogTime{Keeper: f.keeper}.Execute(context.Background(), as(ana), RecordFields{
		WorkType: "meeting", Day: day(t, "2026-11-02"), Minutes: 30,
	}, time.UTC)
	if err != nil || none.Billable || !none.EntryID.IsZero() {
		t.Fatalf("time on no entry: %+v %v", none, err)
	}
	start := monday.Add(time.Hour)
	spanned, err := LogTime{Keeper: f.keeper}.Execute(context.Background(), as(ana), RecordFields{
		EntryID: payment, StartedAt: &start, EndedAt: ptr(start.Add(45 * time.Minute)),
	}, time.UTC)
	if err != nil || spanned.Minutes != 45 || spanned.Day.String() != "2026-11-02" {
		t.Fatalf("a start and an end: %+v %v", spanned, err)
	}
	if _, err := (LogTime{Keeper: f.keeper}).Execute(context.Background(), as(ana), RecordFields{
		EntryID: secret, Day: day(t, "2026-11-02"), Minutes: 10,
	}, time.UTC); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("an entry out of reach: %v", err)
	}
	if _, err := (LogTime{Keeper: f.keeper}).Execute(context.Background(), as(ana), RecordFields{
		WorkType: "lunch", Day: day(t, "2026-11-02"), Minutes: 10,
	}, time.UTC); detail(err) != "work_types.not_found" {
		t.Fatalf("an unknown kind: %v", err)
	}
	for _, e := range f.audit.entries {
		if strings.Contains(fmt.Sprint(e.Changes), "private words") {
			t.Fatalf("the note reached the trail: %+v", e)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// UC-TIM-01 check 3: one timer per person; stopped, it is a record of the day it started.
func TestATimerRunsOnceAndStopsIntoARecord(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	if _, err := (StartTimer{Keeper: f.keeper}).Execute(ctx, as(ana), checkout, "development", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := (StartTimer{Keeper: f.keeper}).Execute(ctx, as(ana), payment, "", "", nil); detail(err) != "timers.running" {
		t.Fatalf("a second timer: %v", err)
	}
	if _, running, _ := (ReadTimer{Keeper: f.keeper}).Execute(ctx, as(ana)); !running {
		t.Fatal("the timer is not read back")
	}
	f.clock.now = monday.Add(50*time.Minute + time.Second)
	r, err := StopTimer{Keeper: f.keeper}.Execute(ctx, as(ana), time.UTC)
	if err != nil || r.Minutes != 51 || r.Source != domain.SourceTimer || r.EntryID != checkout || !r.Billable {
		t.Fatalf("stopped: %+v %v", r, err)
	}
	if _, running, _ := (ReadTimer{Keeper: f.keeper}).Execute(ctx, as(ana)); running {
		t.Fatal("the timer still runs")
	}
	if err := (DiscardTimer{Keeper: f.keeper}).Execute(ctx, as(ana)); detail(err) != "timers.not_running" {
		t.Fatalf("discard nothing: %v", err)
	}
}

// UC-TIM-02 checks 1 to 4: the week answers its days; handed in, its records no longer change
// and the timer cannot stop into it; taken back it is open; last week is copied day by day.
func TestAWeekIsFilledInAndHandedIn(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	mon := f.log(t, ana, checkout, "2026-11-02", 240)
	f.log(t, ana, payment, "2026-11-04", 120)
	w, err := ReadTimesheet{Keeper: f.keeper}.Execute(ctx, as(ana), "", day(t, "2026-11-05"))
	if err != nil || w.Minutes != 360 || w.Days[0] != 240 || w.Days[2] != 120 || w.Sheet.State != domain.SheetOpen {
		t.Fatalf("week %+v %v", w, err)
	}
	if _, err := (SubmitTimesheet{Keeper: f.keeper}).Execute(ctx, as(ana), day(t, "2026-11-09")); detail(err) != "timesheets.empty" {
		t.Fatalf("an empty week: %v", err)
	}
	sheet, err := SubmitTimesheet{Keeper: f.keeper}.Execute(ctx, as(ana), day(t, "2026-11-03"))
	if err != nil || sheet.State != domain.SheetSubmitted || sheet.Minutes != 360 {
		t.Fatalf("submit %+v %v", sheet, err)
	}
	if _, err := (UpdateTimeRecord{Keeper: f.keeper}).Execute(ctx, as(ana), mon.ID, 1, RecordFields{
		EntryID: checkout, Day: mon.Day, Minutes: 60,
	}, time.UTC); detail(err) != "time.week_not_open" {
		t.Fatalf("a change in a handed-in week: %v", err)
	}
	if err := (DeleteTimeRecord{Keeper: f.keeper}).Execute(ctx, as(ana), mon.ID, 1); detail(err) != "time.week_not_open" {
		t.Fatalf("a removal in a handed-in week: %v", err)
	}
	if _, err := (StartTimer{Keeper: f.keeper}).Execute(ctx, as(ana), checkout, "", "", nil); err != nil {
		t.Fatal(err)
	}
	f.clock.now = monday.Add(time.Hour)
	if _, err := (StopTimer{Keeper: f.keeper}).Execute(ctx, as(ana), time.UTC); detail(err) != "time.week_not_open" {
		t.Fatalf("a timer into a handed-in week: %v", err)
	}
	if _, err := (WithdrawTimesheet{Keeper: f.keeper}).Execute(ctx, as(ana), day(t, "2026-11-02")); err != nil {
		t.Fatal(err)
	}
	if _, err := (UpdateTimeRecord{Keeper: f.keeper}).Execute(ctx, as(ana), mon.ID, 1, RecordFields{
		EntryID: checkout, Day: mon.Day, Minutes: 60,
	}, time.UTC); err != nil {
		t.Fatalf("a change in a taken-back week: %v", err)
	}
	n, err := CopyTimesheetWeek{Keeper: f.keeper}.Execute(ctx, as(ana), day(t, "2026-11-11"))
	if err != nil || n != 2 {
		t.Fatalf("copied %d %v", n, err)
	}
	next, _ := ReadTimesheet{Keeper: f.keeper}.Execute(ctx, as(ana), "", day(t, "2026-11-09"))
	if next.Days[0] != 60 || next.Days[2] != 120 {
		t.Fatalf("the copy %+v", next.Days)
	}
	if _, err := (UpdateTimeRecord{Keeper: f.keeper}).Execute(ctx, as(ben), mon.ID, 2, RecordFields{Day: mon.Day, Minutes: 5}, time.UTC); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("somebody else's record changed: %v", err)
	}
}

// UC-TIM-03 checks 1 to 5: an approver of the hub approves the week, a member and the person
// themselves cannot; a sent-back week opens again; an approved record is corrected only by a
// new one; the locked period refuses every write.
func TestAnApproverDecidesTheWeek(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	r := f.log(t, ana, checkout, "2026-11-02", 240)
	if _, err := (SubmitTimesheet{Keeper: f.keeper}).Execute(ctx, as(ana), r.Day); err != nil {
		t.Fatal(err)
	}
	queue, err := ListTimesheets{Keeper: f.keeper}.Execute(ctx, as(lead), "", r.Day-7, r.Day+7, false)
	if err != nil || len(queue) != 1 || queue[0].AccountID != ana {
		t.Fatalf("queue %+v %v", queue, err)
	}
	if q, _ := (ListTimesheets{Keeper: f.keeper}).Execute(ctx, as(ben), "", r.Day-7, r.Day+7, false); len(q) != 0 {
		t.Fatalf("a member's queue %+v", q)
	}
	if _, err := (ApproveTimesheet{Keeper: f.keeper}).Execute(ctx, as(ben), ana, r.Day); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member approved: %v", err)
	}
	if _, err := (ReadTimesheet{Keeper: f.keeper}).Execute(ctx, as(ben), ana, r.Day); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member read the week: %v", err)
	}
	if _, err := (ApproveTimesheet{Keeper: f.keeper}).Execute(ctx, as(ana), ana, r.Day); detail(err) != "timesheets.own_week" {
		t.Fatalf("own week approved: %v", err)
	}
	rejected, err := RejectTimesheet{Keeper: f.keeper}.Execute(ctx, as(lead), ana, r.Day, "Tuesday is missing")
	if err != nil || rejected.State != domain.SheetRejected {
		t.Fatalf("reject %+v %v", rejected, err)
	}
	f.log(t, ana, payment, "2026-11-03", 60)
	if _, err := (SubmitTimesheet{Keeper: f.keeper}).Execute(ctx, as(ana), r.Day); err != nil {
		t.Fatal(err)
	}
	approved, err := ApproveTimesheet{Keeper: f.keeper}.Execute(ctx, as(lead), ana, r.Day)
	if err != nil || approved.State != domain.SheetApproved || approved.DecidedBy != lead {
		t.Fatalf("approve %+v %v", approved, err)
	}
	if _, err := (CorrectTimeRecord{Keeper: f.keeper}).Execute(ctx, as(ana), r.ID, -30, day(t, "2026-11-03"), "too much"); detail(err) != "time.week_not_open" {
		t.Fatalf("a correction into the approved week: %v", err)
	}
	c, err := CorrectTimeRecord{Keeper: f.keeper}.Execute(ctx, as(ana), r.ID, -30, day(t, "2026-11-09"), "too much")
	if err != nil || c.Minutes != -30 || c.Corrects != r.ID || c.Source != domain.SourceCorrection {
		t.Fatalf("correction %+v %v", c, err)
	}
	if f.store.records[r.ID].Minutes != 240 {
		t.Fatal("the approved record changed")
	}
	open := f.log(t, ana, checkout, "2026-11-10", 15)
	if _, err := (CorrectTimeRecord{Keeper: f.keeper}).Execute(ctx, as(ana), open.ID, 5, day(t, "2026-11-10"), ""); detail(err) != "time.correction_not_needed" {
		t.Fatalf("a correction of an open record: %v", err)
	}
	if _, err := (SetTimePolicy{Keeper: f.keeper}).Execute(ctx, as(lead), domain.Policy{Visibility: domain.VisibilityFull, LockedThrough: ptr(day(t, "2026-11-10"))}, 0); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a hub's approver locked the workspace: %v", err)
	}
	f.gate.approvers[lead] = append(f.gate.approvers[lead], tenantID)
	if _, err := (SetTimePolicy{Keeper: f.keeper}).Execute(ctx, as(lead), domain.Policy{Visibility: domain.VisibilityFull, LockedThrough: ptr(day(t, "2026-11-10"))}, 0); err != nil {
		t.Fatal(err)
	}
	if err := (DeleteTimeRecord{Keeper: f.keeper}).Execute(ctx, as(ana), open.ID, 1); detail(err) != "time.period_locked" {
		t.Fatalf("a removal in the locked period: %v", err)
	}
	if _, err := (CorrectTimeRecord{Keeper: f.keeper}).Execute(ctx, as(ana), open.ID, 5, day(t, "2026-11-11"), ""); err != nil {
		t.Fatalf("a correction of a locked record: %v", err)
	}
}

// UC-TIM-04 checks 1 to 4: a report sums the caller's own time, or everybody's where they
// approve; aggregated, it names nobody and withholds a small group; estimates meet actuals.
func TestAReportSumsWhatItsReaderMaySee(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.log(t, ana, checkout, "2026-11-02", 60)
	f.log(t, ben, checkout, "2026-11-02", 30)
	f.log(t, ben, elsewhere, "2026-11-03", 45)
	week := timerepo.Filter{From: day(t, "2026-11-02"), To: day(t, "2026-11-08")}
	own, err := ReadTimeReport{Keeper: f.keeper}.Execute(ctx, as(ana), week, true, []domain.Dimension{domain.ByEntry})
	if err != nil || own.Minutes != 60 {
		t.Fatalf("own %+v %v", own, err)
	}
	team, err := ReadTimeReport{Keeper: f.keeper}.Execute(ctx, as(lead), week, false, []domain.Dimension{domain.ByPerson})
	if err != nil || team.Minutes != 90 || len(team.Groups) != 2 {
		t.Fatalf("the lead's hub %+v %v", team, err)
	}
	f.store.policy.Visibility = domain.VisibilityAggregated
	if _, err := (ReadTimeReport{Keeper: f.keeper}).Execute(ctx, as(lead), week, false, []domain.Dimension{domain.ByPerson}); detail(err) != "time.aggregated_only" {
		t.Fatalf("a person named in an aggregated report: %v", err)
	}
	summed, err := ReadTimeReport{Keeper: f.keeper}.Execute(ctx, as(lead), week, false, []domain.Dimension{domain.ByEntry})
	if err != nil || len(summed.Groups) != 0 || summed.WithheldMinutes != 90 {
		t.Fatalf("a group of two shown: %+v %v", summed, err)
	}
	if _, _, err := (ListTimeRecords{Keeper: f.keeper}).Execute(ctx, as(lead), week, false); detail(err) != "time.aggregated_only" {
		t.Fatalf("records listed in aggregated mode: %v", err)
	}
	if _, err := (ReadEstimatesVsActuals{Keeper: f.keeper}).Execute(ctx, as(ben), listID); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member read actuals in aggregated mode: %v", err)
	}
	f.store.policy.Visibility = domain.VisibilityFull
	rows, err := ReadEstimatesVsActuals{Keeper: f.keeper}.Execute(ctx, as(ben), listID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("actuals %+v %v", rows, err)
	}
	if rows[0].ID != checkout || rows[0].RolledHours != 6 || rows[0].RolledMinutes != 90 || rows[0].RemainingHours != 4.5 {
		t.Fatalf("the checkout %+v", rows[0])
	}
}

// UC-TIM-05 checks 1 to 3: the export carries fixed columns and the records the caller may read,
// is the caller's own only in aggregated mode, and is recorded.
func TestTimeIsHandedToAccounting(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.log(t, ana, checkout, "2026-11-02", 60)
	f.log(t, ben, elsewhere, "2026-11-03", 45)
	week := timerepo.Filter{From: day(t, "2026-11-02"), To: day(t, "2026-11-08")}
	rows, _, _, err := ExportTimeRecords{Keeper: f.keeper}.Execute(ctx, as(lead), week, false, false)
	if err != nil || len(rows) != 1 || rows[0].AccountID != ana {
		t.Fatalf("the lead's export %+v %v", rows, err)
	}
	f.store.policy.Visibility = domain.VisibilityAggregated
	if _, _, _, err := (ExportTimeRecords{Keeper: f.keeper}).Execute(ctx, as(lead), week, false, false); detail(err) != "time.aggregated_only" {
		t.Fatalf("others exported in aggregated mode: %v", err)
	}
	mine, _, _, err := ExportTimeRecords{Keeper: f.keeper}.Execute(ctx, as(ben), week, true, false)
	if err != nil || len(mine) != 1 || mine[0].EntryID != elsewhere {
		t.Fatalf("own export %+v %v", mine, err)
	}
	last := f.audit.entries[len(f.audit.entries)-1]
	if last.Action != RecordsExportedAction {
		t.Fatalf("the export was not recorded: %+v", last)
	}
	out, err := ExportTimeRecords{Keeper: f.keeper}.Descriptor().Handler.Invoke(ctx, as(ben), usecase.Input{"from": "2026-11-02", "to": "2026-11-08", "mine": true})
	if err != nil || len(out["columns"].([]string)) != len(ExportColumns) {
		t.Fatalf("the door's export %+v %v", out, err)
	}
}

func TestEveryTimeUseCaseIsInTheTimeModule(t *testing.T) {
	k := Keeper{}
	for _, d := range []usecase.Descriptor{
		LogTime{k}.Descriptor(), UpdateTimeRecord{k}.Descriptor(), DeleteTimeRecord{k}.Descriptor(),
		CorrectTimeRecord{k}.Descriptor(), ListTimeRecords{k}.Descriptor(), StartTimer{k}.Descriptor(),
		StopTimer{k}.Descriptor(), DiscardTimer{k}.Descriptor(), ReadTimer{k}.Descriptor(),
		ReadTimesheet{k}.Descriptor(), CopyTimesheetWeek{k}.Descriptor(), SubmitTimesheet{k}.Descriptor(),
		WithdrawTimesheet{k}.Descriptor(), ApproveTimesheet{k}.Descriptor(), RejectTimesheet{k}.Descriptor(),
		ListTimesheets{k}.Descriptor(), ListWorkTypes{k}.Descriptor(), DefineWorkType{k}.Descriptor(),
		ReadTimePolicy{k}.Descriptor(), SetTimePolicy{k}.Descriptor(), ReadTimeReport{k}.Descriptor(),
		ReadEstimatesVsActuals{k}.Descriptor(), ExportTimeRecords{k}.Descriptor(),
	} {
		if d.Module != "TIME" || d.Activity.Exempt == "" || d.TokenScope == "" {
			t.Errorf("%s: %+v", d.Name, d)
		}
	}
}
