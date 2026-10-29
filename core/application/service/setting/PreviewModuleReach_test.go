// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting_test

import (
	"context"
	"errors"
	"testing"

	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// people answers twelve in the workspace and five within the hub, and records where it was asked.
type people struct{ asked []shared.ID }

func (p *people) PeopleReaching(_ context.Context, hubID shared.ID) (int, error) {
	p.asked = append(p.asked, hubID)
	if hubID.IsZero() {
		return 12, nil
	}
	return 5, nil
}

// S-09's gap: switching a module off says how many people it leaves without it - everyone who
// reaches the workspace, or the hub - and nobody where the module is off already.
func TestAModuleSwitchedOffCountsThePeopleItLeavesWithoutIt(t *testing.T) {
	ctx := context.Background()
	modules, _, sink := fixture()
	counted := &people{}
	preview := settingservice.PreviewModuleReach{Modules: modules, People: counted}
	registry, err := usecase.NewRegistry(nil, preview.Descriptor())
	if err != nil {
		t.Fatal(err)
	}

	out, err := registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(), usecase.Input{"module": "PLANNING"})
	if err != nil {
		t.Fatal(err)
	}
	if out["on"] != false || out["people"] != 0 || len(counted.asked) != 0 {
		t.Errorf("a module off already reached %v, asked %v", out, counted.asked)
	}

	if _, err := (settingservice.SetModule{Modules: modules}).Execute(ctx, actor(), settingservice.SetModuleCommand{
		Module: module.Planning, Switch: settingservice.SwitchOn,
	}); err != nil {
		t.Fatal(err)
	}
	entries := len(sink.entries)
	out, err = registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(), usecase.Input{"module": "PLANNING"})
	if err != nil || out["on"] != true || out["people"] != 12 {
		t.Errorf("the workspace answered %v, %v", out, err)
	}
	out, err = registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(),
		usecase.Input{"module": "PLANNING", "hub_id": hub.String()})
	if err != nil || out["people"] != 5 || out["hub_id"] != hub.String() {
		t.Errorf("the hub answered %v, %v", out, err)
	}
	if len(counted.asked) != 2 || !counted.asked[0].IsZero() || counted.asked[1] != hub {
		t.Errorf("asked %v", counted.asked)
	}
	if len(sink.entries) != entries {
		t.Errorf("the preview wrote %d audit entries", len(sink.entries)-entries)
	}

	_, err = registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(),
		usecase.Input{"module": "PLANNING", "hub_id": list.String()})
	if !refusedWith(err, "module.hub_only") {
		t.Errorf("a collection was answered: %v", err)
	}
}

// The preview is asked as the switch is.
func TestAModuleReachIsRefusedAsTheSwitchWouldBe(t *testing.T) {
	modules, _, _ := fixture()
	modules.Authorizer = authorizer{refuse: true}
	_, err := settingservice.PreviewModuleReach{Modules: modules, People: &people{}}.
		Execute(context.Background(), actor(), module.Planning, "", settingservice.SwitchOff)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("err = %v", err)
	}
}

// S-09's gap: leaving the workspace's module to the installation, whose default is off, counts
// the workspace's people; leaving a hub's to a workspace that has it on counts nobody.
func TestAModuleLeftToTheLevelAboveCountsOnlyWhenItTurnsOff(t *testing.T) {
	ctx := context.Background()
	modules, _, _ := fixture()
	preview := settingservice.PreviewModuleReach{Modules: modules, People: &people{}}
	registry, err := usecase.NewRegistry(nil, preview.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	set := settingservice.SetModule{Modules: modules}
	for _, at := range []shared.ID{shared.ID(""), hub} {
		if _, err := set.Execute(ctx, actor(), settingservice.SetModuleCommand{
			Module: module.Planning, Switch: settingservice.SwitchOn, HubID: at,
		}); err != nil {
			t.Fatal(err)
		}
	}

	out, err := registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(),
		usecase.Input{"module": "PLANNING", "switch": "INHERIT"})
	if err != nil || out["on"] != true || out["people"] != 12 {
		t.Errorf("leaving it to an installation that has it off answered %v, %v", out, err)
	}
	out, err = registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(),
		usecase.Input{"module": "PLANNING", "hub_id": hub.String(), "switch": "INHERIT"})
	if err != nil || out["on"] != true || out["people"] != 0 {
		t.Errorf("leaving it to a workspace that has it on answered %v, %v", out, err)
	}

	_, err = registry.Invoke(ctx, settingservice.PreviewModuleReachName, actor(),
		usecase.Input{"module": "PLANNING", "switch": "ON"})
	if err == nil {
		t.Error("a switch on was counted")
	}
}
