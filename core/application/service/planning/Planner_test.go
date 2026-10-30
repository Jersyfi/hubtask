// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	planrepo "github.com/Jersyfi/hubtask/core/application/repository/planning"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenantID   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000001")
	accountID  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000002")
	hubID      = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000010")
	collection = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000011")
	design     = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000101")
	build      = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000102")
	launch     = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000103")
	secret     = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000104")
	dependsOn  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000201")
	duplicates = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000202")
	toSecret   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000203")
	launchDep  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000204")
)

type inline struct{}

func (inline) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (inline) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

// gate allows everything but the entries it hides, and records what was asked.
type gate struct {
	hidden map[shared.ID]bool
	deny   bool
	asked  []access.Request
}

func (g *gate) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	g.asked = append(g.asked, request)
	if g.deny {
		return shared.ErrNotFound
	}
	return nil
}

func (g *gate) Permitted(_ context.Context, _ appshared.ActorContext, _ access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i, path := range paths {
		out[i] = !g.hidden[shared.ID(path[len(path)-1].ID)]
	}
	return out, nil
}

func at(text string) *time.Time {
	t, _ := time.Parse(time.DateOnly, text)
	return &t
}

type plans struct {
	entries   []planrepo.PlanEntry
	schedules map[shared.ID]planrepo.EntrySchedule
	timings   map[shared.ID]domain.Timing
}

func (p *plans) Entries(context.Context, shared.ID, int) ([]planrepo.PlanEntry, error) {
	return p.entries, nil
}

func (p *plans) Schedules(_ context.Context, ids []shared.ID) (map[shared.ID]planrepo.EntrySchedule, error) {
	out := map[shared.ID]planrepo.EntrySchedule{}
	for _, id := range ids {
		if s, ok := p.schedules[id]; ok {
			out[id] = s
		}
	}
	return out, nil
}

func (p *plans) SetSchedule(_ context.Context, s planrepo.EntrySchedule) error {
	p.schedules[s.EntryID] = s
	return nil
}

func (p *plans) SetTiming(_ context.Context, id shared.ID, t domain.Timing) error {
	p.timings[id] = t
	return nil
}

type links struct {
	workrepo.EntryLinks
	all []work.EntryLink
}

func (l links) Touching(context.Context, []shared.ID, int) ([]work.EntryLink, error) {
	return l.all, nil
}

func (l links) Find(_ context.Context, id shared.ID) (work.EntryLink, error) {
	for _, link := range l.all {
		if link.ID == id {
			return link, nil
		}
	}
	return work.EntryLink{}, shared.ErrNotFound
}

type linkTypes struct{ workrepo.LinkTypes }

func (linkTypes) List(context.Context) ([]work.LinkType, error) { return nil, nil }

type references struct {
	workrepo.References
	summaries []workrepo.EntrySummary
}

func (r references) Entries(_ context.Context, ids []shared.ID) ([]workrepo.EntrySummary, error) {
	var out []workrepo.EntrySummary
	for _, s := range r.summaries {
		if slices.Contains(ids, s.ID) {
			out = append(out, s)
		}
	}
	return out, nil
}

type containers struct{ workrepo.Containers }

func (containers) Find(_ context.Context, id shared.ID) (work.Container, error) {
	if id == hubID {
		return work.Container{ID: id, TenantID: tenantID, Type: work.ContainerHub}, nil
	}
	return work.Container{ID: id, TenantID: tenantID, Type: work.ContainerCollection, ParentID: hubID}, nil
}

type items struct{ workrepo.Items }

func (items) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	return work.WorkItem{ID: id, TenantID: tenantID, CollectionID: collection}, nil
}

type sink struct{ entries []audit.Entry }

func (s *sink) Append(_ context.Context, e audit.Entry) error {
	s.entries = append(s.entries, e)
	return nil
}

type catalogue struct{ calls []usecase.Input }

func (c *catalogue) Invoke(_ context.Context, name string, _ appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	c.calls = append(c.calls, in)
	switch name {
	case updateWorkItemName:
		return usecase.Output{}, nil
	case createPageName:
		return usecase.Output{"id": notesPage.String()}, nil
	}
	return nil, errors.New("unexpected " + name)
}

type baselines struct {
	count int
	taken []domain.Baseline
	found domain.Baseline
}

func (b *baselines) Count(context.Context, shared.ID) (int, error) { return b.count, nil }
func (b *baselines) Take(_ context.Context, baseline domain.Baseline, _ int) (int, error) {
	b.taken = append(b.taken, baseline)
	return 3, nil
}
func (b *baselines) List(context.Context, shared.ID) ([]domain.Baseline, error) { return b.taken, nil }
func (b *baselines) Find(context.Context, shared.ID) (domain.Baseline, error)   { return b.found, nil }
func (b *baselines) Delete(context.Context, shared.ID) error                    { return nil }

type ids struct{ n int }

func (i *ids) NewID() shared.ID {
	i.n++
	return shared.MustParseID("01936f2a-7c1e-7000-8000-00000000090" + string(rune('0'+i.n)))
}

type fixture struct {
	planner   Planner
	gate      *gate
	plans     *plans
	audit     *sink
	catalogue *catalogue
	baselines *baselines
}

// newFixture is the relaunch: "design" (manual, Mon 2 - Wed 4) is waited on by "build" (automatic,
// two days, drawn far too late) and by "launch" (automatic milestone); "secret" is out of the
// reader's reach and waits on "design" too. A duplicates link schedules nothing.
func newFixture() fixture {
	g := &gate{hidden: map[shared.ID]bool{secret: true}}
	p := &plans{
		schedules: map[shared.ID]planrepo.EntrySchedule{},
		timings:   map[shared.ID]domain.Timing{},
		entries: []planrepo.PlanEntry{
			{ID: design, CollectionID: collection, HubID: hubID, Title: "Approve the design", StartAt: at("2026-11-02"), DueAt: at("2026-11-04"), Mode: domain.Manual, Version: 3},
			{ID: build, CollectionID: collection, HubID: hubID, StartAt: at("2026-11-20"), DueAt: at("2026-11-23"), Mode: domain.Automatic, Version: 5},
			{ID: launch, CollectionID: collection, HubID: hubID, DueAt: at("2026-12-01"), Mode: domain.Automatic, Milestone: true, Version: 1},
			{ID: secret, CollectionID: collection, HubID: hubID, StartAt: at("2026-11-02"), DueAt: at("2026-11-02"), Mode: domain.Manual},
		},
	}
	all := []work.EntryLink{
		{ID: dependsOn, Type: work.LinkTypeDependsOn, SourceID: build, TargetID: design, Timing: domain.Timing{Kind: domain.FinishToStart}},
		{ID: launchDep, Type: work.LinkTypeDependsOn, SourceID: launch, TargetID: build, Timing: domain.Timing{Kind: domain.FinishToStart}},
		{ID: toSecret, Type: work.LinkTypeBlocks, SourceID: design, TargetID: secret, Timing: domain.Timing{Kind: domain.FinishToStart}},
		{ID: duplicates, Type: work.LinkTypeDuplicates, SourceID: launch, TargetID: design, Timing: domain.Timing{Kind: domain.FinishToStart}},
	}
	s, c, b := &sink{}, &catalogue{}, &baselines{}
	return fixture{
		gate: g, plans: p, audit: s, catalogue: c, baselines: b,
		planner: Planner{
			Plans: p, Baselines: b, Items: items{}, Containers: containers{}, LinkTypes: linkTypes{},
			Links: links{all: all}, References: references{}, Catalogue: c, Authorizer: g, Reader: g,
			Audit: s, UnitOfWork: inline{}, Clock: clock.Fixed(time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)),
			IDs: &ids{},
		},
	}
}

func actor() appshared.ActorContext {
	return appshared.ActorContext{TenantID: tenantID, AccountID: accountID, Kind: appshared.ActorUser, Scopes: []string{itemsRead, itemsWrite}}
}

func entryOf(t *testing.T, out usecase.Output, id shared.ID) usecase.Output {
	t.Helper()
	for _, e := range out["entries"].([]usecase.Output) {
		if e["item_id"] == id.String() {
			return e
		}
	}
	return nil
}

// PLN-02 check 3, PLN-03 checks 1 and 2: the plan proposes the automatic moves, keeps the manual
// entry, measures the float and names the critical path - without the entry the reader may not see.
func TestAPlanIsScheduledForItsReader(t *testing.T) {
	f := newFixture()
	read := ReadPlanSchedule{Planner: f.planner}
	scheduled, err := read.Execute(context.Background(), actor(), ScheduleQuery{PlanID: hubID})
	if err != nil {
		t.Fatal(err)
	}
	out := scheduled.Output()
	if asked := f.gate.asked[0]; asked.Permission != service.PermissionRead || asked.TargetID != hubID {
		t.Errorf("asked %+v, want READ at the plan", asked)
	}
	if b := entryOf(t, out, build); b["start"] != "2026-11-05" || b["finish"] != "2026-11-06" || b["moved"] != true {
		t.Errorf("build = %v, want Thursday and Friday, moved", b)
	}
	if l := entryOf(t, out, launch); l["start"] != "2026-11-06" || l["milestone"] != true || l["duration_days"] != 0 {
		t.Errorf("launch = %v, want the milestone on the Friday build finishes", l)
	}
	if d := entryOf(t, out, design); d["title"] != "Approve the design" || d["parent_id"] != nil || d["collection_id"] != collection.String() {
		t.Errorf("design = %v, want its title, no parent and its collection for the chart", d)
	}
	if d := entryOf(t, out, design); d["moved"] != false || d["critical"] != true {
		t.Errorf("design = %v", d)
	}
	if entryOf(t, out, secret) != nil {
		t.Error("an entry out of reach was scheduled for the reader")
	}
	if deps := out["dependencies"].([]usecase.Output); len(deps) != 2 {
		t.Errorf("dependencies = %v, want the two among visible entries, no duplicates link", deps)
	}
	if path := out["critical_path"].([]string); !slices.Equal(path, []string{design.String(), build.String(), launch.String()}) {
		t.Errorf("critical path = %v", path)
	}
}

// PLN-02 check 4: only the accepted moves are applied, each through UpdateWorkItem with the
// version read.
func TestOnlyAcceptedMovesAreApplied(t *testing.T) {
	f := newFixture()
	apply := ApplyPlanSchedule{Planner: f.planner}
	applied, err := apply.Execute(context.Background(), actor(), ApplyPlanScheduleCommand{
		ScheduleQuery: ScheduleQuery{PlanID: hubID, Zone: "Europe/Berlin"}, ItemIDs: []shared.ID{build},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.catalogue.calls) != 1 || len(applied.Moved) != 1 {
		t.Fatalf("calls = %v", f.catalogue.calls)
	}
	call := f.catalogue.calls[0]
	if call["item_id"] != build.String() || call["start_at"] != "2026-11-05T00:00:00+01:00" ||
		call["due_at"] != "2026-11-06T00:00:00+01:00" || call["due_date_only"] != true ||
		call["due_time_zone"] != "Europe/Berlin" || call["expected_version"] != 5 {
		t.Errorf("update = %v", call)
	}
	if asked := f.gate.asked[0]; asked.Permission != service.PermissionWriteItems {
		t.Errorf("asked %+v, want WRITE_ITEMS at the plan", asked)
	}
	if _, err := apply.Execute(context.Background(), actor(), ApplyPlanScheduleCommand{ScheduleQuery: ScheduleQuery{PlanID: hubID}}); err == nil {
		t.Error("a schedule was applied with nothing accepted")
	}
}

// PLN-02 check 1: a dependency takes a kind and a lag; a link nobody waits on takes none, and the
// waiting entry's writer sets it.
func TestADependencysTimingIsSetOnTheWaitingEnd(t *testing.T) {
	f := newFixture()
	set := SetDependencyTiming{Planner: f.planner}
	link, err := set.Execute(context.Background(), actor(), SetDependencyTimingCommand{
		LinkID: dependsOn, Timing: domain.Timing{Kind: domain.StartToStart, LagDays: -2},
	})
	if err != nil || link.Timing != (domain.Timing{Kind: domain.StartToStart, LagDays: -2}) {
		t.Fatalf("link = %+v, err = %v", link, err)
	}
	if f.plans.timings[dependsOn].LagDays != -2 {
		t.Error("the timing was not written")
	}
	if asked := f.gate.asked[0]; asked.Permission != service.PermissionWriteItems || asked.TargetID != build {
		t.Errorf("asked %+v, want WRITE_ITEMS on the waiting entry", asked)
	}
	var e *shared.Error
	if _, err := set.Execute(context.Background(), actor(), SetDependencyTimingCommand{
		LinkID: duplicates, Timing: domain.Timing{Kind: domain.FinishToFinish},
	}); !errors.As(err, &e) || e.DetailCode != "links.timing_without_dependency" {
		t.Errorf("a duplicates link took a timing: %v", err)
	}
	if _, err := set.Execute(context.Background(), actor(), SetDependencyTimingCommand{
		LinkID: dependsOn, Timing: domain.Timing{Kind: domain.FinishToStart, LagDays: 1000},
	}); !errors.As(err, &e) || e.DetailCode != "planning.lag_out_of_range" {
		t.Errorf("a lag past the bound: %v", err)
	}
}

type changes struct{ recorded []changelog.Change }

func (c *changes) Record(_ context.Context, change changelog.Change) error {
	c.recorded = append(c.recorded, change)
	return nil
}

type hlc struct{}

func (hlc) Next() shared.HLC { return shared.HLC{} }

// PLN-02 check 2 offline: an entry's new mode is filed for offline devices under its collection.
func TestAnEntrysModeIsToldToOfflineDevices(t *testing.T) {
	f := newFixture()
	log := &changes{}
	f.planner.Changes, f.planner.HLC = log, hlc{}
	automatic := domain.Automatic
	if _, err := (SetEntryScheduling{Planner: f.planner}).Execute(context.Background(), actor(), SetEntrySchedulingCommand{ItemID: design, Mode: &automatic}); err != nil {
		t.Fatal(err)
	}
	if len(log.recorded) != 1 {
		t.Fatalf("recorded %d changes, want one", len(log.recorded))
	}
	got := log.recorded[0]
	if got.Entity != ScheduleEntity || got.EntityID != design || got.ContainerID != collection || got.Payload["mode"] != "AUTOMATIC" {
		t.Errorf("change = %+v", got)
	}
}

// PLN-05 check 1: an entry is made a milestone and automatic; what is left out stays.
func TestAnEntrysModeAndMilestoneAreSet(t *testing.T) {
	f := newFixture()
	set := SetEntryScheduling{Planner: f.planner}
	automatic, yes := domain.Automatic, true
	if _, err := set.Execute(context.Background(), actor(), SetEntrySchedulingCommand{ItemID: design, Mode: &automatic}); err != nil {
		t.Fatal(err)
	}
	got, err := set.Execute(context.Background(), actor(), SetEntrySchedulingCommand{ItemID: design, Milestone: &yes})
	if err != nil || got.Mode != domain.Automatic || !got.Milestone {
		t.Errorf("schedule = %+v, err = %v", got, err)
	}
	bad := domain.Mode("SOMETIMES")
	if _, err := set.Execute(context.Background(), actor(), SetEntrySchedulingCommand{ItemID: design, Mode: &bad}); err == nil {
		t.Error("an unknown mode was accepted")
	}
}

// PLN-04 checks 1 and 2: a baseline is taken under a name up to the limit, and compared without
// the entries the reader may not see.
func TestABaselineIsTakenAndComparedForItsReader(t *testing.T) {
	f := newFixture()
	take := TakeBaseline{Planner: f.planner}
	baseline, err := take.Execute(context.Background(), actor(), TakeBaselineCommand{PlanID: hubID, Name: "Agreed with the board"})
	if err != nil || baseline.EntryCount != 3 || baseline.TakenBy != accountID {
		t.Fatalf("baseline = %+v, err = %v", baseline, err)
	}
	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != BaselineTakenAction {
		t.Errorf("audit = %+v", f.audit.entries)
	}
	f.baselines.count = domain.MaxBaselinesPerPlan
	var e *shared.Error
	if _, err := take.Execute(context.Background(), actor(), TakeBaselineCommand{PlanID: hubID, Name: "One more"}); !errors.As(err, &e) || e.DetailCode != "baselines.limit" {
		t.Errorf("a twenty-first baseline: %v", err)
	}

	f.baselines.found = domain.Baseline{ID: baseline.ID, PlanID: hubID, Entries: []domain.BaselineEntry{
		{EntryID: design, StartAt: at("2026-11-02"), DueAt: at("2026-11-03")},
		{EntryID: secret, StartAt: at("2026-11-01"), DueAt: at("2026-11-01")},
	}}
	f.planner.References = references{summaries: []workrepo.EntrySummary{{ID: secret, CollectionID: collection, HubID: hubID}}}
	comparison, err := CompareBaseline{Planner: f.planner}.Execute(context.Background(), actor(), baseline.ID)
	if err != nil {
		t.Fatal(err)
	}
	var seen []shared.ID
	for _, v := range comparison.Variances {
		seen = append(seen, v.EntryID)
		if v.EntryID == design && (v.FinishVarianceDays == nil || *v.FinishVarianceDays != 1) {
			t.Errorf("design slipped %v, want 1 day", v.FinishVarianceDays)
		}
	}
	if slices.Contains(seen, secret) {
		t.Error("an entry out of reach was compared")
	}
	if !slices.Contains(seen, build) {
		t.Error("an entry added since is missing")
	}

	f.gate.deny = true
	if _, err := (ListBaselines{Planner: f.planner}).Execute(context.Background(), actor(), hubID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody who may not read the plan listed its baselines: %v", err)
	}
}
