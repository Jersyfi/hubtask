// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package module

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

// UC-INS-21 check 6, the writers' half: the installation's and a plan's decisions resolve as
// their levels' layers, and a plan's decision under the installation's lock does not fit.
func TestDecisionsAsLayers(t *testing.T) {
	installation := Decisions{Planning: {On: true, Locked: true}, Goals: {On: true}}
	plan := Decisions{Planning: {On: false}, Goals: {On: false, Locked: true}}

	states := Resolve(installation.Layer(setting.LevelInstance), plan.Layer(setting.LevelPlan))
	for _, state := range states {
		switch state.Module {
		case Planning:
			if !state.On() || state.Resolution.LockedBy != setting.LevelInstance {
				t.Fatalf("planning: %+v", state.Resolution)
			}
		case Goals:
			if state.On() || state.Resolution.LockedBy != setting.LevelPlan {
				t.Fatalf("goals: %+v", state.Resolution)
			}
		default:
			if state.On() || state.Resolution.Source != setting.LevelDefault {
				t.Fatalf("%s: %+v", state.Module, state.Resolution)
			}
		}
	}

	if key, fits := plan.FitsUnder(installation); fits || key != Planning {
		t.Fatalf("FitsUnder = %s, %v", key, fits)
	}
	if _, fits := (Decisions{Goals: {On: true}}).FitsUnder(installation); !fits {
		t.Fatal("an open module refused")
	}
}

func TestNameRoundTrips(t *testing.T) {
	for _, key := range Keys() {
		if back, held := OfName(key.Name()); !held || back != key {
			t.Fatalf("%s: %s %v", key, back, held)
		}
	}
	if _, held := OfName("SERVICE_DESK"); held {
		t.Fatal("the contract's spelling read as a document name")
	}
}
