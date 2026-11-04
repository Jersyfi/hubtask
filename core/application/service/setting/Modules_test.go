// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant = shared.MustParseID("018f0000-0000-7000-8000-000000000001")
	hub    = shared.MustParseID("018f0000-0000-7000-8000-000000000002")
	list   = shared.MustParseID("018f0000-0000-7000-8000-000000000003")
	lena   = shared.MustParseID("018f0000-0000-7000-8000-0000000000a1")
)

type memorySettings struct {
	instance, plan domain.Layer
	rows           map[settingrepo.Place]map[domain.Key]domain.Entry
}

func newMemory() *memorySettings {
	return &memorySettings{
		instance: domain.Layer{Level: domain.LevelInstance, Entries: map[domain.Key]domain.Entry{}},
		plan:     domain.Layer{Level: domain.LevelPlan, Entries: map[domain.Key]domain.Entry{}},
		rows:     map[settingrepo.Place]map[domain.Key]domain.Entry{},
	}
}

func (m *memorySettings) Above(context.Context, string) (domain.Layer, domain.Layer, error) {
	return m.instance, m.plan, nil
}

func (m *memorySettings) At(_ context.Context, places []settingrepo.Place, _ string) ([]settingrepo.Stored, error) {
	var out []settingrepo.Stored
	for _, place := range places {
		for key, entry := range m.rows[place] {
			out = append(out, settingrepo.Stored{Place: place, Key: key, Entry: entry})
		}
	}
	return out, nil
}

func (m *memorySettings) Put(_ context.Context, _ shared.ID, place settingrepo.Place, key domain.Key, entry domain.Entry, _ time.Time) error {
	if m.rows[place] == nil {
		m.rows[place] = map[domain.Key]domain.Entry{}
	}
	m.rows[place][key] = entry
	return nil
}

func (m *memorySettings) Remove(_ context.Context, place settingrepo.Place, key domain.Key) (bool, error) {
	_, held := m.rows[place][key]
	delete(m.rows[place], key)
	return held, nil
}

type containers struct{}

func (containers) Find(_ context.Context, id shared.ID) (work.Container, error) {
	switch id {
	case hub:
		return work.Container{ID: hub, TenantID: tenant, Type: work.ContainerHub}, nil
	case list:
		return work.Container{ID: list, TenantID: tenant, Type: work.ContainerCollection, ParentID: hub}, nil
	}
	return work.Container{}, shared.ErrNotFound
}

type authorizer struct{ refuse bool }

func (a authorizer) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	if a.refuse {
		return shared.ErrForbidden
	}
	return nil
}

type trail struct{ entries []audit.Entry }

func (t *trail) Append(_ context.Context, entry audit.Entry) error {
	t.entries = append(t.entries, entry)
	return nil
}

type unitOfWork struct{}

func (unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (unitOfWork) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type clock struct{}

func (clock) Now() time.Time { return time.Date(2026, 10, 28, 6, 0, 0, 0, time.UTC) }

type ids struct{}

func (ids) NewID() shared.ID { return shared.MustParseID("018f0000-0000-7000-8000-0000000000ff") }

func fixture() (settingservice.Modules, *memorySettings, *trail) {
	store, sink := newMemory(), &trail{}
	return settingservice.Modules{
		Settings: store, Containers: containers{}, Authorizer: authorizer{}, Audit: sink,
		UnitOfWork: unitOfWork{}, Clock: clock{}, IDs: ids{},
	}, store, sink
}

func actor() appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant, AccountID: lena}
}

func on(t *testing.T, states []module.State, key module.Key) module.State {
	t.Helper()
	for _, state := range states {
		if state.Module == key {
			return state
		}
	}
	t.Fatalf("%s missing", key)
	return module.State{}
}

func refusedWith(err error, code string) bool {
	var refusal *shared.Error
	return errors.As(err, &refusal) && refusal.DetailCode == code
}

// UC-INS-21 checks 1, 2, 3, 4 and 8 through the two use cases.
func TestSwitchingModules(t *testing.T) {
	ctx := context.Background()
	modules, store, sink := fixture()
	set := settingservice.SetModule{Modules: modules}
	read := settingservice.ListModules{Modules: modules}

	// Check 1: a new workspace has nothing on.
	states, err := read.Execute(ctx, actor(), "")
	if err != nil {
		t.Fatal(err)
	}
	if got := module.SetOf(states); len(got) != 0 {
		t.Errorf("a new workspace has %v on", got)
	}

	// Check 2: one switch, at once on.
	states, err = set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.Planning, Switch: settingservice.SwitchOn})
	if err != nil {
		t.Fatal(err)
	}
	if state := on(t, states, module.Planning); !state.On() || state.Resolution.Source != domain.LevelWorkspace {
		t.Errorf("planning after switching on = %+v", state)
	}

	// Check 3: narrowed to a hub; a hub never widens.
	states, err = set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.Planning, Switch: settingservice.SwitchOff, HubID: hub})
	if err != nil {
		t.Fatal(err)
	}
	if on(t, states, module.Planning).On() {
		t.Error("the hub did not narrow planning")
	}
	_, err = set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.Time, Switch: settingservice.SwitchOn, HubID: hub})
	if !refusedWith(err, "setting.not_allowed") {
		t.Errorf("a hub widening = %v, want setting.not_allowed", err)
	}
	if _, err = set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.Time, Switch: settingservice.SwitchOn, HubID: list}); !refusedWith(err, "module.hub_only") {
		t.Errorf("a collection's switch = %v, want module.hub_only", err)
	}

	// Check 4: off and on again - the store keeps nothing but the switch, so a module's data is
	// never touched by switching.
	if _, err = set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.Planning, Switch: settingservice.SwitchOff}); err != nil {
		t.Fatal(err)
	}
	if got := len(store.rows[settingrepo.Place{Level: domain.LevelWorkspace, ScopeID: tenant}]); got != 1 {
		t.Errorf("the workspace stores %d entries, want the one switch", got)
	}

	// Check 8: every switch is in the trail with who did it.
	if len(sink.entries) != 3 {
		t.Fatalf("trail holds %d entries, want 3", len(sink.entries))
	}
	for _, entry := range sink.entries {
		if entry.Action != settingservice.ModuleSwitchedAction || entry.ActorID != lena {
			t.Errorf("trail entry %+v", entry)
		}
	}
}

// UC-INS-21 check 6: a plan's lock is shown with its origin and refuses the switch.
func TestAPlansLockIsShownAndHeld(t *testing.T) {
	ctx := context.Background()
	modules, store, _ := fixture()
	store.plan.Entries[module.ServiceDesk.SettingKey()] = domain.Entry{Value: true, Hold: domain.HoldLock}

	states, err := settingservice.ListModules{Modules: modules}.Execute(ctx, actor(), "")
	if err != nil {
		t.Fatal(err)
	}
	if state := on(t, states, module.ServiceDesk); !state.On() || state.Resolution.LockedBy != domain.LevelPlan {
		t.Errorf("service desk = %+v, want on and locked by the plan", state)
	}
	_, err = settingservice.SetModule{Modules: modules}.Execute(ctx, actor(),
		settingservice.SetModuleCommand{Module: module.ServiceDesk, Switch: settingservice.SwitchOff})
	if !refusedWith(err, "setting.locked") {
		t.Errorf("switching a locked module = %v, want setting.locked", err)
	}
}

// UC-INS-21 check 5: an operation of a module that is off is refused on the registry, the door
// every channel comes through, with one code.
func TestTheGateRefusesAModuleThatIsOff(t *testing.T) {
	ctx := context.Background()
	modules, _, _ := fixture()
	ran := false
	descriptor := usecase.Descriptor{
		Name: "DrawGantt", Summary: "test", Module: module.Planning,
		Handler: usecase.HandlerFunc(func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
			ran = true
			return usecase.Output{}, nil
		}),
	}
	registry, err := usecase.NewRegistry(settingservice.ModuleGate{Modules: modules}.Wrap(nil), descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Invoke(ctx, "DrawGantt", actor(), usecase.Input{}); !refusedWith(err, module.OffCode) || ran {
		t.Errorf("an operation of an off module = %v (ran %v), want %s", err, ran, module.OffCode)
	}
	if _, err := (settingservice.SetModule{Modules: modules}).Execute(ctx, actor(),
		settingservice.SetModuleCommand{Module: module.Planning, Switch: settingservice.SwitchOn}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Invoke(ctx, "DrawGantt", actor(), usecase.Input{}); err != nil || !ran {
		t.Errorf("an operation of an on module = %v (ran %v)", err, ran)
	}
}

func TestReadingNeedsTheRightToShape(t *testing.T) {
	modules, _, sink := fixture()
	modules.Authorizer = authorizer{refuse: true}
	if _, err := (settingservice.ListModules{Modules: modules}).Execute(context.Background(), actor(), ""); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a read without the right = %v", err)
	}
	if _, err := (settingservice.SetModule{Modules: modules}).Execute(context.Background(), actor(),
		settingservice.SetModuleCommand{Module: module.Planning, Switch: settingservice.SwitchOn}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a switch without the right = %v", err)
	}
	if len(sink.entries) != 0 {
		t.Error("a refused switch wrote a success entry")
	}
}

type epochs struct{ advanced int }

func (e *epochs) Advance(context.Context) (int64, error) {
	e.advanced++
	return int64(e.advanced), nil
}

// The goals leave a device's copy while off (offline-sync.md §8): a change of what the workspace
// resolves them to advances the epoch, so every device walks again; a hub's narrowing, another
// module and a switch that changes nothing advance none.
func TestSwitchingTheGoalsResynchronisesTheWorkspace(t *testing.T) {
	ctx := context.Background()
	modules, _, _ := fixture()
	counter := &epochs{}
	modules.Epochs = counter
	set := settingservice.SetModule{Modules: modules}
	steps := []struct {
		cmd  settingservice.SetModuleCommand
		want int
	}{
		{settingservice.SetModuleCommand{Module: module.Planning, Switch: settingservice.SwitchOn}, 0},
		{settingservice.SetModuleCommand{Module: module.Goals, Switch: settingservice.SwitchOn}, 1},
		{settingservice.SetModuleCommand{Module: module.Goals, Switch: settingservice.SwitchOn}, 1},
		{settingservice.SetModuleCommand{Module: module.Goals, Switch: settingservice.SwitchOff, HubID: hub}, 1},
		{settingservice.SetModuleCommand{Module: module.Goals, Switch: settingservice.SwitchInherit}, 2},
	}
	for i, step := range steps {
		if _, err := set.Execute(ctx, actor(), step.cmd); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if counter.advanced != step.want {
			t.Errorf("after step %d the epoch advanced %d times, want %d", i, counter.advanced, step.want)
		}
	}
}

type gateItems map[shared.ID]shared.ID

func (g gateItems) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	if collection, ok := g[id]; ok {
		return work.WorkItem{ID: id, CollectionID: collection}, nil
	}
	return work.WorkItem{}, shared.ErrNotFound
}

// UC-INS-21 check 3 at the gate: planning on for the workspace and narrowed off for one hub is
// refused for an operation that names that hub, a collection in it or an entry there - the same
// refusal every door meets - and accepted for one judged at the workspace.
func TestTheGateRefusesAModuleAHubNarrowedOff(t *testing.T) {
	ctx := context.Background()
	modules, _, _ := fixture()
	set := settingservice.SetModule{Modules: modules}
	for _, cmd := range []settingservice.SetModuleCommand{
		{Module: module.Planning, Switch: settingservice.SwitchOn},
		{Module: module.Planning, Switch: settingservice.SwitchOff, HubID: hub},
	} {
		if _, err := set.Execute(ctx, actor(), cmd); err != nil {
			t.Fatal(err)
		}
	}
	entry := shared.MustParseID("018f0000-0000-7000-8000-0000000000e1")
	descriptor := usecase.Descriptor{
		Name: "MovePlan", Summary: "test", Module: module.Planning,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID},
			{Name: "item_id", Kind: usecase.KindID},
		},
		Handler: usecase.HandlerFunc(func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
			return usecase.Output{}, nil
		}),
	}
	gate := settingservice.ModuleGate{Modules: modules, Items: gateItems{entry: list}}
	registry, err := usecase.NewRegistry(gate.Wrap(nil), descriptor)
	if err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string]usecase.Input{
		"a collection in the hub": {"collection_id": list.String()},
		"an entry in the hub":     {"item_id": entry.String()},
	} {
		if _, err := registry.Invoke(ctx, "MovePlan", actor(), in); !refusedWith(err, module.OffCode) {
			t.Errorf("%s: %v, want %s", name, err, module.OffCode)
		}
	}
	if _, err := registry.Invoke(ctx, "MovePlan", actor(), usecase.Input{}); err != nil {
		t.Errorf("at the workspace, where planning is on: %v", err)
	}
	// A place nobody can find is the handler's to answer, not the gate's.
	unknown := shared.MustParseID("018f0000-0000-7000-8000-0000000000ff")
	if _, err := registry.Invoke(ctx, "MovePlan", actor(), usecase.Input{"item_id": unknown.String()}); err != nil {
		t.Errorf("an unknown entry: %v, want the workspace's answer", err)
	}
}

// UC-INS-21 check 3 for a module's own objects: a page or a board names only itself, and the gate
// judges it at its hub - pages narrowed off for that hub are refused there, through every door.
func TestTheGateJudgesAModuleObjectAtItsHub(t *testing.T) {
	ctx := context.Background()
	modules, _, _ := fixture()
	set := settingservice.SetModule{Modules: modules}
	for _, cmd := range []settingservice.SetModuleCommand{
		{Module: module.Knowledge, Switch: settingservice.SwitchOn},
		{Module: module.Knowledge, Switch: settingservice.SwitchOff, HubID: hub},
	} {
		if _, err := set.Execute(ctx, actor(), cmd); err != nil {
			t.Fatal(err)
		}
	}
	page := shared.MustParseID("018f0000-0000-7000-8000-0000000000a1")
	descriptor := usecase.Descriptor{
		Name: "EditPage", Summary: "test", Module: module.Knowledge,
		Input: []usecase.Field{{Name: "page_id", Kind: usecase.KindID}},
		Handler: usecase.HandlerFunc(func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
			return usecase.Output{}, nil
		}),
	}
	gate := settingservice.ModuleGate{Modules: modules, Objects: map[string]settingservice.GateObject{
		"page_id": func(_ context.Context, id shared.ID) (shared.ID, error) {
			if id == page {
				return hub, nil
			}
			return "", shared.ErrNotFound
		},
	}}
	registry, err := usecase.NewRegistry(gate.Wrap(nil), descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Invoke(ctx, "EditPage", actor(), usecase.Input{"page_id": page.String()}); !refusedWith(err, module.OffCode) {
		t.Errorf("a page in the hub: %v, want %s", err, module.OffCode)
	}
	unknown := shared.MustParseID("018f0000-0000-7000-8000-0000000000ff")
	if _, err := registry.Invoke(ctx, "EditPage", actor(), usecase.Input{"page_id": unknown.String()}); err != nil {
		t.Errorf("an unknown page: %v, want the workspace's answer", err)
	}
}

// countingWork counts the transactions a call opens: a nested one joins the running one, as the
// PostgreSQL unit of work does.
type countingWork struct{ opened int }

type runningKey struct{}

func (w *countingWork) run(ctx context.Context, fn func(context.Context) error) error {
	if ctx.Value(runningKey{}) != nil {
		return fn(ctx)
	}
	w.opened++
	return fn(context.WithValue(ctx, runningKey{}, true))
}

func (w *countingWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return w.run(ctx, fn)
}

func (w *countingWork) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return w.run(ctx, fn)
}

// The gate reads the place and the switches in one transaction of its own: on the busiest module
// paths every extra one is a pool acquisition the handler's own transaction waits behind.
func TestTheGateReadsItsAnswerInOneTransaction(t *testing.T) {
	modules, _, _ := fixture()
	work := &countingWork{}
	modules.UnitOfWork = work
	descriptor := usecase.Descriptor{
		Name: "DrawGantt", Summary: "test", Module: module.Planning,
		Handler: usecase.HandlerFunc(func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
			return usecase.Output{}, nil
		}),
	}
	registry, err := usecase.NewRegistry(settingservice.ModuleGate{Modules: modules}.Wrap(nil), descriptor)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = registry.Invoke(context.Background(), "DrawGantt", actor(), usecase.Input{})
	if work.opened != 1 {
		t.Errorf("the gate opened %d transactions, want one", work.opened)
	}
}

// Switching the service desk on switches on what it reads, in the same write: its home reads the
// entry types and workflows, and a desk with Workflows off answered module.off on every tab.
func TestSwitchingTheServiceDeskOnSwitchesWorkflowsOn(t *testing.T) {
	ctx := context.Background()
	modules, _, sink := fixture()
	states, err := settingservice.SetModule{Modules: modules}.Execute(ctx, actor(),
		settingservice.SetModuleCommand{Module: module.ServiceDesk, Switch: settingservice.SwitchOn})
	if err != nil {
		t.Fatal(err)
	}
	if !on(t, states, module.ServiceDesk).On() || !on(t, states, module.Workflows).On() {
		t.Errorf("after switching the desk on: %v", module.SetOf(states))
	}
	if len(sink.entries) != 2 {
		t.Errorf("trail holds %d entries, want one per module switched", len(sink.entries))
	}
	descriptor := usecase.Descriptor{
		Name: "ListWorkflows", Summary: "test", Module: module.Workflows,
		Handler: usecase.HandlerFunc(func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error) {
			return usecase.Output{}, nil
		}),
	}
	registry, err := usecase.NewRegistry(settingservice.ModuleGate{Modules: modules}.Wrap(nil), descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Invoke(ctx, "ListWorkflows", actor(), usecase.Input{}); err != nil {
		t.Errorf("the desk's read of the workflows = %v, want it served", err)
	}
}

// Where Workflows is locked off above, the desk is refused rather than switched on broken.
func TestTheServiceDeskIsRefusedWhereWorkflowsIsLockedOff(t *testing.T) {
	ctx := context.Background()
	modules, store, _ := fixture()
	store.plan.Entries[module.Workflows.SettingKey()] = domain.Entry{Value: false, Hold: domain.HoldLock}
	_, err := settingservice.SetModule{Modules: modules}.Execute(ctx, actor(),
		settingservice.SetModuleCommand{Module: module.ServiceDesk, Switch: settingservice.SwitchOn})
	if !refusedWith(err, "setting.locked") {
		t.Errorf("the desk without the workflows it needs = %v, want setting.locked", err)
	}
	states, err := settingservice.ListModules{Modules: modules}.Execute(ctx, actor(), "")
	if err != nil {
		t.Fatal(err)
	}
	if on(t, states, module.ServiceDesk).On() {
		t.Error("the desk was switched on although its workflows cannot be")
	}
}

// Workflows switched off under a desk that is on would leave the desk reading what is refused:
// the switch is refused and nothing is written.
func TestWorkflowsIsNotSwitchedOffUnderTheServiceDesk(t *testing.T) {
	ctx := context.Background()
	modules, _, sink := fixture()
	set := settingservice.SetModule{Modules: modules}
	if _, err := set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.ServiceDesk, Switch: settingservice.SwitchOn}); err != nil {
		t.Fatal(err)
	}
	written := len(sink.entries)
	for _, to := range []settingservice.Switch{settingservice.SwitchOff, settingservice.SwitchInherit} {
		_, err := set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.Workflows, Switch: to})
		if !refusedWith(err, "module.needed") {
			t.Errorf("workflows %s under the desk = %v, want module.needed", to, err)
		}
	}
	states, err := settingservice.ListModules{Modules: modules}.Execute(ctx, actor(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !on(t, states, module.Workflows).On() || len(sink.entries) != written {
		t.Errorf("after the refusals: %v, %d trail entries, want workflows on and none added", module.SetOf(states), len(sink.entries)-written)
	}
	if _, err := set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.ServiceDesk, Switch: settingservice.SwitchOff}); err != nil {
		t.Fatal(err)
	}
	if _, err := set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.Workflows, Switch: settingservice.SwitchOff}); err != nil {
		t.Errorf("workflows off once the desk is off = %v, want it switched", err)
	}
}
