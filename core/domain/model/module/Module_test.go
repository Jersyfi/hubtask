// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package module_test

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func at(level setting.Level, key module.Key, on bool, hold setting.Hold) setting.Layer {
	return setting.Layer{Level: level, Entries: map[setting.Key]setting.Entry{
		key.SettingKey(): {Value: on, Hold: hold},
	}}
}

func stateOf(states []module.State, key module.Key) module.State {
	for _, state := range states {
		if state.Module == key {
			return state
		}
	}
	return module.State{}
}

// UC-INS-21 checks 1, 3, 6 and 7, as the catalogue resolves them.
func TestModuleSwitches(t *testing.T) {
	cases := []struct {
		name   string
		layers []setting.Layer
		on     bool
		source setting.Level
		locked setting.Level
	}{
		{name: "check 1: a new workspace has nothing on", on: false, source: setting.LevelDefault},
		{name: "check 7: without a plan the workspace switches on",
			layers: []setting.Layer{at(setting.LevelWorkspace, module.Planning, true, setting.HoldSet)},
			on:     true, source: setting.LevelWorkspace},
		{name: "check 3: a hub narrows what the workspace has on",
			layers: []setting.Layer{
				at(setting.LevelWorkspace, module.Planning, true, setting.HoldSet),
				at(setting.LevelHub, module.Planning, false, setting.HoldSet),
			}, on: false, source: setting.LevelHub},
		{name: "check 3: a hub never widens",
			layers: []setting.Layer{at(setting.LevelHub, module.Planning, true, setting.HoldSet)},
			on:     false, source: setting.LevelDefault},
		{name: "check 6: a plan locks on",
			layers: []setting.Layer{
				at(setting.LevelPlan, module.Planning, true, setting.HoldLock),
				at(setting.LevelWorkspace, module.Planning, false, setting.HoldSet),
			}, on: true, source: setting.LevelPlan, locked: setting.LevelPlan},
		{name: "check 6: a plan that does not offer it locks off",
			layers: []setting.Layer{
				at(setting.LevelPlan, module.Planning, false, setting.HoldLock),
				at(setting.LevelWorkspace, module.Planning, true, setting.HoldSet),
			}, on: false, source: setting.LevelPlan, locked: setting.LevelPlan},
		{name: "the installation's lock comes first",
			layers: []setting.Layer{
				at(setting.LevelInstance, module.Planning, false, setting.HoldLock),
				at(setting.LevelPlan, module.Planning, true, setting.HoldLock),
			}, on: false, source: setting.LevelInstance, locked: setting.LevelInstance},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := stateOf(module.Resolve(tc.layers...), module.Planning)
			if state.On() != tc.on || state.Resolution.Source != tc.source || state.Resolution.LockedBy != tc.locked {
				t.Errorf("planning = on %v from %s locked by %q, want on %v from %s locked by %q",
					state.On(), state.Resolution.Source, state.Resolution.LockedBy, tc.on, tc.source, tc.locked)
			}
		})
	}
}

// UC-INS-21 check 5: one refusal code for a module that is off.
func TestRequireRefusesAModuleThatIsOff(t *testing.T) {
	on := module.Set{module.Planning}
	if err := on.Require(module.Planning); err != nil {
		t.Errorf("Require(on) = %v", err)
	}
	err := on.Require(module.Time)
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != module.OffCode || !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("Require(off) = %v, want %s", err, module.OffCode)
	}
}

func TestEveryModuleIsAValidCatalogueEntry(t *testing.T) {
	if _, err := setting.NewCatalogue(module.Definitions()...); err != nil {
		t.Fatal(err)
	}
	for _, key := range module.Keys() {
		if back, held := module.OfSettingKey(key.SettingKey()); !held || back != key {
			t.Errorf("%s does not round-trip through %s", key, key.SettingKey())
		}
	}
}

// Only the goals leave a device's copy while off: every other module's records are drawn by the
// base product's screens too.
func TestOnlyTheGoalsNarrowTheReplica(t *testing.T) {
	for _, key := range module.Keys() {
		if got, want := key.NarrowsReplica(), key == module.Goals; got != want {
			t.Errorf("%s.NarrowsReplica() = %v, want %v", key, got, want)
		}
	}
}

// A desk is entry types and their workflows, a post a page of the hub's space: the service desk
// cannot be on without Workflows, posts not without Knowledge, and no other module needs another.
func TestOnlyTheServiceDeskAndPostsNeedAnotherModule(t *testing.T) {
	want := map[module.Key]module.Key{module.ServiceDesk: module.Workflows, module.Posts: module.Knowledge}
	for _, key := range module.Keys() {
		got := key.Needs()
		if needed, ok := want[key]; ok {
			if len(got) != 1 || got[0] != needed {
				t.Errorf("%s needs %v, want [%s]", key, got, needed)
			}
			continue
		}
		if len(got) != 0 {
			t.Errorf("%s needs %v, want none", key, got)
		}
	}
}

// A module on whose needed module is off is the one state a switch may not leave behind.
func TestUnmetNamesTheModuleOnWithoutWhatItNeeds(t *testing.T) {
	cases := []struct {
		name      string
		workflows bool
		wantUnmet bool
		desk      bool
	}{
		{"desk and workflows on", true, false, true},
		{"desk on, workflows off", false, true, true},
		{"desk off, workflows off", false, false, false},
	}
	for _, c := range cases {
		layer := setting.Layer{Level: setting.LevelWorkspace, Entries: map[setting.Key]setting.Entry{
			module.ServiceDesk.SettingKey(): {Value: c.desk, Hold: setting.HoldSet},
			module.Workflows.SettingKey():   {Value: c.workflows, Hold: setting.HoldSet},
		}}
		on, needed, unmet := module.Unmet(module.Resolve(layer))
		if unmet != c.wantUnmet {
			t.Errorf("%s: unmet = %v, want %v", c.name, unmet, c.wantUnmet)
		}
		if unmet && (on != module.ServiceDesk || needed != module.Workflows) {
			t.Errorf("%s: unmet names %s needing %s", c.name, on, needed)
		}
	}
}

// A switch may name up to MaxChosenHubs hubs; one more is refused at the list's pointer, with the
// bound as the message's parameter.
func TestBoundChosenHubs(t *testing.T) {
	cases := []struct {
		name    string
		count   int
		refused bool
	}{
		{"none", 0, false},
		{"below the bound", module.MaxChosenHubs - 1, false},
		{"at the bound", module.MaxChosenHubs, false},
		{"above the bound", module.MaxChosenHubs + 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := module.BoundChosenHubs(tc.count, "/hub_ids")
			if !tc.refused {
				if err != nil {
					t.Fatalf("BoundChosenHubs(%d) = %v, want nil", tc.count, err)
				}
				return
			}
			var refusal *shared.Error
			if !errors.As(err, &refusal) || !errors.Is(err, shared.ErrValidation) ||
				refusal.DetailCode != module.TooManyHubsCode {
				t.Fatalf("BoundChosenHubs(%d) = %v, want %s", tc.count, err, module.TooManyHubsCode)
			}
			if got := refusal.Params["maximum"]; got != "200" {
				t.Errorf("maximum = %q, want 200", got)
			}
			if len(refusal.Fields) != 1 || refusal.Fields[0].Path != "/hub_ids" ||
				refusal.Fields[0].Code != module.TooManyHubsCode {
				t.Errorf("fields = %+v, want one at /hub_ids with %s", refusal.Fields, module.TooManyHubsCode)
			}
		})
	}
}
