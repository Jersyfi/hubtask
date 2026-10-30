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
