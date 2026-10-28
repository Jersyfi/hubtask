// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"errors"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/plan"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INS-21 check 6, the writers' half: the installation's document and a plan's carry a
// `module` area, read and answered in the shape of every other area, and a plan's decision under
// the installation's lock is refused.
func TestTheModuleAreaRoundTrips(t *testing.T) {
	level, err := instanceLevelFrom(usecase.Input{
		"module": map[string]any{
			"planning":     map[string]any{"value": true, "locked": true},
			"service_desk": map[string]any{"value": false},
			"SERVICE_DESK": map[string]any{"value": true},
			"unknown":      map[string]any{"value": true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := module.Decisions{module.Planning: {On: true, Locked: true}, module.ServiceDesk: {On: false}}
	if len(level.Modules) != len(want) || level.Modules[module.Planning] != want[module.Planning] ||
		level.Modules[module.ServiceDesk] != want[module.ServiceDesk] {
		t.Fatalf("modules = %+v", level.Modules)
	}

	area, _ := instanceLevelOutput(level)["module"].(usecase.Output)
	if len(area) != len(module.Keys()) {
		t.Fatalf("area answers %d modules, want every one", len(area))
	}
	planning, _ := area["planning"].(usecase.Output)
	if planning["value"] != true || planning["locked"] != true || planning["kind"] != "FLAG" {
		t.Fatalf("planning = %+v", planning)
	}
	if goals, _ := area["goals"].(usecase.Output); goals["set"] != false || goals["value"] != nil {
		t.Fatalf("an undecided module answered a value: %+v", goals)
	}

	if _, err := instanceLevelFrom(usecase.Input{
		"module": map[string]any{"planning": map[string]any{"value": "yes"}},
	}); err == nil {
		t.Fatal("a module switched by a word was accepted")
	}

	cmd, err := planCommandFrom(usecase.Input{
		"name": "team", "display_name": "Team",
		"module": map[string]any{"planning": map[string]any{"value": false, "locked": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Values.Modules[module.Planning] != (module.Decision{On: false, Locked: true}) {
		t.Fatalf("plan modules = %+v", cmd.Values.Modules)
	}
	out := planOutput(PlanView{Plan: plan.Plan{Values: cmd.Values}})
	if _, held := out["module"]; !held {
		t.Fatal("a plan's document answers no module area")
	}
	err = cmd.Values.FitsUnder(aboveOf(identityrepo.InstanceLevel{Modules: level.Modules}))
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "admin.plan_under_installation_lock" {
		t.Fatalf("a plan under the installation's module lock: %v", err)
	}
}
