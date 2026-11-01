// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A hub's and a collection's screens are drawn for the modules on at the hub (UC-INS-21 check 3):
// a module the workspace has on and the hub narrowed off is not listed there, for a collection of
// the hub either, while the workspace still lists it.
func TestTheModulesOnAtAHubAreWhatItsScreensAreDrawnFor(t *testing.T) {
	ctx := context.Background()
	modules, _, _ := fixture()
	set := settingservice.SetModule{Modules: modules}
	if _, err := set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.Planning, Switch: settingservice.SwitchOn}); err != nil {
		t.Fatal(err)
	}
	if _, err := set.Execute(ctx, actor(), settingservice.SetModuleCommand{Module: module.Planning, Switch: settingservice.SwitchOff, HubID: hub}); err != nil {
		t.Fatal(err)
	}

	read := settingservice.ListContainerModules{Modules: modules}
	for _, place := range []shared.ID{hub, list} {
		on, err := read.Execute(ctx, actor(), place)
		if err != nil {
			t.Fatalf("%s: %v", place, err)
		}
		if slices.Contains(on, module.Planning) {
			t.Errorf("%s lists PLANNING, which its hub has off", place)
		}
	}
	workspace, err := modules.On(ctx, actor(), "")
	if err != nil || !workspace.Has(module.Planning) {
		t.Errorf("the workspace lost PLANNING: %v, %v", workspace, err)
	}
}

// Whoever may not read the place learns nothing about its modules, and a place that does not exist
// is not found.
func TestTheModulesOnAtAPlaceNeedItsRead(t *testing.T) {
	ctx := context.Background()
	modules, _, _ := fixture()
	modules.Authorizer = authorizer{refuse: true}
	if _, err := (settingservice.ListContainerModules{Modules: modules}).Execute(ctx, actor(), hub); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a reader without READ was answered: %v", err)
	}
	modules.Authorizer = authorizer{}
	missing := shared.MustParseID("018f0000-0000-7000-8000-0000000000ee")
	if _, err := (settingservice.ListContainerModules{Modules: modules}).Execute(ctx, actor(), missing); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing place answered %v", err)
	}
}
