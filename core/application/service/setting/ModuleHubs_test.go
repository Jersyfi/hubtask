// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	domain "github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	clients = shared.MustParseID("018f0000-0000-7000-8000-000000000011")
	office  = shared.MustParseID("018f0000-0000-7000-8000-000000000012")
	later   = shared.MustParseID("018f0000-0000-7000-8000-000000000013")
	latest  = shared.MustParseID("018f0000-0000-7000-8000-000000000014")
)

// everyHub answers a hub for every identifier but the collection's.
type everyHub struct{}

func (everyHub) Find(_ context.Context, id shared.ID) (work.Container, error) {
	if id == list {
		return work.Container{ID: list, TenantID: tenant, Type: work.ContainerCollection, ParentID: clients}, nil
	}
	return work.Container{ID: id, TenantID: tenant, Type: work.ContainerHub}, nil
}

// shaping refuses the right to shape the places named.
type shaping struct{ refused []shared.ID }

func (s shaping) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	if slices.Contains(s.refused, request.TargetID) {
		return shared.ErrForbidden
	}
	return nil
}

// workspaceWith is a workspace with the two hubs that exist before any switch.
func workspaceWith() (settingservice.SetModule, settingservice.ListModules, *memorySettings, *trail) {
	modules, store, sink := fixture()
	modules.Containers = everyHub{}
	store.hubs = []shared.ID{clients, office}
	return settingservice.SetModule{Modules: modules}, settingservice.ListModules{Modules: modules}, store, sink
}

// step is one move of a scenario: a switch, or a hub made, and what the hubs have afterwards.
type step struct {
	does    string
	command settingservice.SetModuleCommand
	makes   shared.ID
	refused string
	has     map[shared.ID][]module.Key
}

func workspaceSwitch(key module.Key, to settingservice.Switch, hubs ...shared.ID) settingservice.SetModuleCommand {
	return settingservice.SetModuleCommand{Module: key, Switch: to, HubIDs: hubs}
}

func hubSwitch(key module.Key, to settingservice.Switch, at shared.ID) settingservice.SetModuleCommand {
	return settingservice.SetModuleCommand{Module: key, Switch: to, HubID: at}
}

// UC-INS-21 checks 3 and 13: on at the workspace changes no hub that exists but the chosen ones
// and is what a hub made afterwards starts with; each hub decides for itself inside it; off at
// the workspace takes the module from every hub, and on again gives each hub the state it had.
func TestAWorkspaceSwitchLeavesEachHubItsState(t *testing.T) {
	none := []module.Key{}
	planning := []module.Key{module.Planning}
	desk := []module.Key{module.Workflows, module.ServiceDesk}
	scenarios := []struct {
		name    string
		prepare func(*memorySettings)
		steps   []step
	}{
		{
			name: "on with no hub chosen leaves every existing hub off and a hub made afterwards on",
			steps: []step{
				{does: "on", command: workspaceSwitch(module.Planning, settingservice.SwitchOn),
					has: map[shared.ID][]module.Key{clients: none, office: none}},
				{does: "a hub is made", makes: later,
					has: map[shared.ID][]module.Key{clients: none, office: none, later: planning}},
			},
		},
		{
			name: "the chosen hubs are on, the others unchanged",
			steps: []step{
				{does: "on, one chosen twice", command: workspaceSwitch(module.Planning, settingservice.SwitchOn, clients, clients),
					has: map[shared.ID][]module.Key{clients: planning, office: none}},
			},
		},
		{
			name: "each hub decides for itself inside what the workspace has on",
			steps: []step{
				{does: "a hub switches on under off", command: hubSwitch(module.Planning, settingservice.SwitchOn, office),
					refused: "setting.not_allowed"},
				{does: "on", command: workspaceSwitch(module.Planning, settingservice.SwitchOn, clients),
					has: map[shared.ID][]module.Key{clients: planning, office: none}},
				{does: "a hub switches on", command: hubSwitch(module.Planning, settingservice.SwitchOn, office),
					has: map[shared.ID][]module.Key{clients: planning, office: planning}},
				{does: "a hub switches off", command: hubSwitch(module.Planning, settingservice.SwitchOff, clients),
					has: map[shared.ID][]module.Key{clients: none, office: planning}},
			},
		},
		{
			name: "off takes it from every hub, on again gives each hub the state it had",
			steps: []step{
				{does: "on", command: workspaceSwitch(module.Planning, settingservice.SwitchOn, clients)},
				{does: "a hub is made while on", makes: later,
					has: map[shared.ID][]module.Key{clients: planning, office: none, later: planning}},
				{does: "off", command: workspaceSwitch(module.Planning, settingservice.SwitchOff),
					has: map[shared.ID][]module.Key{clients: none, office: none, later: none}},
				{does: "a hub is made while off", makes: latest},
				{does: "a hub switches on under off", command: hubSwitch(module.Planning, settingservice.SwitchOn, later),
					refused: "setting.not_allowed"},
				{does: "on again", command: workspaceSwitch(module.Planning, settingservice.SwitchOn),
					has: map[shared.ID][]module.Key{clients: planning, office: none, later: planning, latest: none}},
			},
		},
		{
			name: "a module brought along follows the same rule",
			steps: []step{
				{does: "the desk on, one chosen", command: workspaceSwitch(module.ServiceDesk, settingservice.SwitchOn, clients),
					has: map[shared.ID][]module.Key{clients: desk, office: none}},
				{does: "a hub is made", makes: later,
					has: map[shared.ID][]module.Key{later: desk}},
				{does: "the desk off", command: workspaceSwitch(module.ServiceDesk, settingservice.SwitchOff),
					has: map[shared.ID][]module.Key{clients: {module.Workflows}, office: none, later: {module.Workflows}}},
				{does: "workflows off", command: workspaceSwitch(module.Workflows, settingservice.SwitchOff),
					has: map[shared.ID][]module.Key{clients: none, office: none, later: none}},
				{does: "the desk on again", command: workspaceSwitch(module.ServiceDesk, settingservice.SwitchOn),
					has: map[shared.ID][]module.Key{clients: desk, office: none, later: desk}},
			},
		},
		{
			name: "workflows on already, the desk chosen for a hub that has workflows off",
			steps: []step{
				{does: "workflows on", command: workspaceSwitch(module.Workflows, settingservice.SwitchOn)},
				{does: "the desk on, one chosen", command: workspaceSwitch(module.ServiceDesk, settingservice.SwitchOn, office),
					has: map[shared.ID][]module.Key{clients: none, office: desk}},
			},
		},
		{
			name: "an installation's default reached by leaving it to the level above behaves alike",
			prepare: func(store *memorySettings) {
				store.instance.Entries[module.Planning.SettingKey()] = domain.Entry{Value: true, Hold: domain.HoldSet}
				workspace := settingrepo.Place{Level: domain.LevelWorkspace, ScopeID: tenant}
				store.rows[workspace] = map[domain.Key]domain.Entry{
					module.Planning.SettingKey(): {Value: false, Hold: domain.HoldSet},
				}
			},
			steps: []step{
				{does: "left to the installation, which has it on", command: workspaceSwitch(module.Planning, settingservice.SwitchInherit),
					has: map[shared.ID][]module.Key{clients: none, office: none}},
				{does: "a hub is made", makes: later,
					has: map[shared.ID][]module.Key{later: planning}},
				{does: "off", command: workspaceSwitch(module.Planning, settingservice.SwitchOff),
					has: map[shared.ID][]module.Key{clients: none, office: none, later: none}},
				{does: "left to the installation again", command: workspaceSwitch(module.Planning, settingservice.SwitchInherit),
					has: map[shared.ID][]module.Key{clients: none, office: none, later: planning}},
			},
		},
		{
			name: "a hub that stores the desk on keeps workflows when the workspace brings them back",
			prepare: func(store *memorySettings) {
				store.rows[settingrepo.Place{Level: domain.LevelHub, ScopeID: clients}] = map[domain.Key]domain.Entry{
					module.ServiceDesk.SettingKey(): {Value: true, Hold: domain.HoldSet},
				}
			},
			steps: []step{
				{does: "the desk on", command: workspaceSwitch(module.ServiceDesk, settingservice.SwitchOn),
					has: map[shared.ID][]module.Key{clients: desk, office: none}},
			},
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			set, read, store, _ := workspaceWith()
			if scenario.prepare != nil {
				scenario.prepare(store)
			}
			for _, move := range scenario.steps {
				if !move.makes.IsZero() {
					store.hubs = append(store.hubs, move.makes)
				} else if _, err := set.Execute(ctx, actor(), move.command); move.refused != "" {
					if !refusedWith(err, move.refused) {
						t.Fatalf("%s: %v, want %s", move.does, err, move.refused)
					}
				} else if err != nil {
					t.Fatalf("%s: %v", move.does, err)
				}
				for at, want := range move.has {
					states, err := read.Execute(ctx, actor(), at)
					if err != nil {
						t.Fatal(err)
					}
					got := []module.Key(module.SetOf(states))
					if !slices.Equal(got, want) {
						t.Errorf("after %q the hub %s has %v on, want %v", move.does, at, got, want)
					}
					if _, _, unmet := module.Unmet(states); unmet {
						t.Errorf("after %q the hub %s has a module on without the one it needs", move.does, at)
					}
				}
			}
		})
	}
}

// UC-INS-21 check 13: the hubs a switch names are the person's to shape, each of them, or nothing
// is switched (rule 2); and they go with switching on at the workspace only.
func TestTheChosenHubsAreRefusedWhole(t *testing.T) {
	ctx := context.Background()
	refusals := []struct {
		name    string
		command settingservice.SetModuleCommand
		barred  []shared.ID
		want    string
		is      error
	}{
		{name: "a hub the person may not shape", command: workspaceSwitch(module.Planning, settingservice.SwitchOn, clients, office),
			barred: []shared.ID{office}, is: shared.ErrForbidden},
		{name: "with off", command: workspaceSwitch(module.Planning, settingservice.SwitchOff, clients), want: "module.hubs_with_on_only"},
		{name: "with the level above", command: workspaceSwitch(module.Planning, settingservice.SwitchInherit, clients), want: "module.hubs_with_on_only"},
		{name: "with a hub's own switch", command: settingservice.SetModuleCommand{
			Module: module.Planning, Switch: settingservice.SwitchOn, HubID: clients, HubIDs: []shared.ID{office},
		}, want: "module.hubs_with_on_only"},
		{name: "a collection", command: workspaceSwitch(module.Planning, settingservice.SwitchOn, list), want: "module.hub_only"},
		{name: "no identifier", command: workspaceSwitch(module.Planning, settingservice.SwitchOn, ""), want: "usecase.field_required"},
	}
	for _, refusal := range refusals {
		t.Run(refusal.name, func(t *testing.T) {
			set, _, store, sink := workspaceWith()
			set.Modules.Authorizer = shaping{refused: refusal.barred}
			_, err := set.Execute(ctx, actor(), refusal.command)
			if refusal.is != nil && !errors.Is(err, refusal.is) || refusal.want != "" && !refusedWith(err, refusal.want) {
				t.Fatalf("the switch answered %v", err)
			}
			if len(store.rows) != 0 || len(sink.entries) != 0 {
				t.Errorf("a refused switch stored %v and wrote %d trail entries", store.rows, len(sink.entries))
			}
		})
	}
}

// UC-INS-21 check 8: the trail says how many hubs were given their state as their own, and which
// hubs were chosen - a count and identifiers, nothing a person wrote.
func TestTheTrailCountsTheHubsAndNamesTheChosen(t *testing.T) {
	ctx := context.Background()
	set, _, store, sink := workspaceWith()
	store.hubs = append(store.hubs, later)
	if _, err := set.Execute(ctx, actor(), workspaceSwitch(module.Planning, settingservice.SwitchOn, clients)); err != nil {
		t.Fatal(err)
	}
	// Every row written for a hub carries an id of the generator's (arc42 §2.3), none the store's.
	if len(store.pinned) != 3 {
		t.Fatalf("the switch handed the store %d hubs to write, want every hub storing nothing", len(store.pinned))
	}
	for _, row := range store.pinned {
		if row.RowID != (ids{}).NewID() {
			t.Errorf("the hub %s is written under the row id %q, not one of the generator's", row.HubID, row.RowID)
		}
	}
	if len(sink.entries) != 1 {
		t.Fatalf("the switch wrote %d trail entries, want 1", len(sink.entries))
	}
	changes := sink.entries[0].Changes
	if kept, _ := changes["hubs_kept"].(map[string]any); kept["to"] != 2 {
		t.Errorf("the trail counts %v hubs kept, want the 2 not chosen", changes["hubs_kept"])
	}
	chosen, _ := changes["hub_ids"].(map[string]any)
	if ids, _ := chosen["to"].([]string); !slices.Equal(ids, []string{clients.String()}) {
		t.Errorf("the trail names %v as chosen", changes["hub_ids"])
	}

	// A hub's own switch, and a workspace's that turns nothing, reach no other hub.
	if _, err := set.Execute(ctx, actor(), hubSwitch(module.Planning, settingservice.SwitchOff, clients)); err != nil {
		t.Fatal(err)
	}
	if _, err := set.Execute(ctx, actor(), workspaceSwitch(module.Planning, settingservice.SwitchOn)); err != nil {
		t.Fatal(err)
	}
	for _, entry := range sink.entries[1:] {
		if _, counted := entry.Changes["hubs_kept"]; counted {
			t.Errorf("a switch that turned nothing at the workspace counts hubs: %v", entry.Changes)
		}
	}
}

// UC-INS-21 check 12: each hub a workspace's switch wrote a value into gets its own record, so
// the record a device holds of a hub is never older than the hub's stored state.
func TestAWorkspaceSwitchRecordsEachHubItWrote(t *testing.T) {
	ctx := context.Background()
	set, _, store, _ := workspaceWith()
	log := &changes{}
	set.Modules.Changes, set.Modules.HLC = log, hlc{}

	if _, err := set.Execute(ctx, actor(), workspaceSwitch(module.ServiceDesk, settingservice.SwitchOn, clients)); err != nil {
		t.Fatal(err)
	}
	recorded := map[shared.ID]map[string]any{}
	for _, change := range log.recorded {
		if _, twice := recorded[change.EntityID]; twice {
			t.Errorf("%s is recorded twice for one switch", change.EntityID)
		}
		if change.EntityID != tenant && change.ContainerID != change.EntityID {
			t.Errorf("the hub's record is filed under %q", change.ContainerID)
		}
		recorded[change.EntityID] = change.Payload
	}
	if len(recorded) != 3 {
		t.Fatalf("the switch recorded %d places, want the workspace and its two hubs", len(recorded))
	}
	if row := switchOf(t, recorded[clients], module.ServiceDesk); row["on"] != true {
		t.Errorf("the chosen hub's record says %v", row)
	}
	if row := switchOf(t, recorded[clients], module.Workflows); row["on"] != true {
		t.Errorf("the chosen hub's record says %v of the module brought along", row)
	}
	if row := switchOf(t, recorded[office], module.ServiceDesk); row["on"] != false {
		t.Errorf("the other hub's record says %v", row)
	}

	// A hub made afterwards stores nothing and is written, and recorded, when the workspace
	// switches off.
	store.hubs = append(store.hubs, later)
	log.recorded = nil
	if _, err := set.Execute(ctx, actor(), workspaceSwitch(module.ServiceDesk, settingservice.SwitchOff)); err != nil {
		t.Fatal(err)
	}
	if len(log.recorded) != 2 || log.recorded[0].EntityID != tenant || log.recorded[1].EntityID != later {
		t.Fatalf("switching off recorded %+v, want the workspace and the hub it wrote", log.recorded)
	}
	if row := switchOf(t, log.recorded[1].Payload, module.ServiceDesk); row["on"] != false {
		t.Errorf("the hub's record says %v under a workspace that has the module off", row)
	}
}

// UC-INS-21 check 8: a module brought along is named in the trail with the hubs it was switched
// on in, also where the workspace had it on already and only the chosen hubs gain it.
func TestTheTrailNamesAModuleBroughtAlongIntoAChosenHub(t *testing.T) {
	ctx := context.Background()
	set, _, _, sink := workspaceWith()
	if _, err := set.Execute(ctx, actor(), workspaceSwitch(module.Workflows, settingservice.SwitchOn)); err != nil {
		t.Fatal(err)
	}
	sink.entries = nil
	if _, err := set.Execute(ctx, actor(), workspaceSwitch(module.ServiceDesk, settingservice.SwitchOn, office)); err != nil {
		t.Fatal(err)
	}
	named := map[module.Key][]string{}
	for _, entry := range sink.entries {
		chosen, _ := entry.Changes["hub_ids"].(map[string]any)
		ids, _ := chosen["to"].([]string)
		for _, key := range []module.Key{module.Workflows, module.ServiceDesk} {
			if _, names := entry.Changes[string(key.SettingKey())]; names {
				named[key] = ids
			}
		}
	}
	for _, key := range []module.Key{module.Workflows, module.ServiceDesk} {
		if !slices.Equal(named[key], []string{office.String()}) {
			t.Errorf("the trail names %v as the hubs %s was switched on in, want the chosen hub", named[key], key)
		}
	}
}
