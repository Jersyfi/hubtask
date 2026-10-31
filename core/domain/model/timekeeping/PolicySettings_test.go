// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
)

func TestThePolicyDefinitionsAreValid(t *testing.T) {
	for _, d := range PolicyDefinitions() {
		if err := d.Validate(); err != nil {
			t.Fatalf("%s: %v", d.Key, err)
		}
	}
}

func layerOf(level setting.Level, entries map[setting.Key]setting.Entry) setting.Layer {
	return setting.Layer{Level: level, Entries: entries}
}

// The time policy resolves through the installation, the plan and the workspace's own row: the
// nearest level that decides wins, a lock above binds the workspace, and a value above that is
// no day is read as none.
func TestTheTimePolicyResolvesThroughTheLevels(t *testing.T) {
	locked := day(t, "2026-09-30")
	row := Policy{LockedThrough: &locked, Visibility: VisibilityFull, Version: 3}
	aggregatedLocked := map[setting.Key]setting.Entry{VisibilityKey: {Value: "AGGREGATED", Hold: setting.HoldLock}}
	cases := []struct {
		name           string
		stored         Policy
		above          []setting.Layer
		wantLocked     string
		wantVisibility Visibility
		wantSource     setting.Level
		wantLockedBy   setting.Level
		decided        bool
	}{
		{name: "nothing set", stored: Policy{}, wantVisibility: VisibilityFull, wantSource: setting.LevelDefault},
		{name: "the workspace's row", stored: row, wantLocked: "2026-09-30", wantVisibility: VisibilityFull, wantSource: setting.LevelWorkspace, decided: true},
		{
			name: "a plan locks aggregated over the row", stored: row,
			above:      []setting.Layer{layerOf(setting.LevelPlan, aggregatedLocked)},
			wantLocked: "2026-09-30", wantVisibility: VisibilityAggregated, wantSource: setting.LevelPlan,
			wantLockedBy: setting.LevelPlan, decided: true,
		},
		{
			name: "the installation sets a default without a row", stored: Policy{},
			above: []setting.Layer{layerOf(setting.LevelInstance, map[setting.Key]setting.Entry{
				VisibilityKey: {Value: "AGGREGATED", Hold: setting.HoldSet}, LockedThroughKey: {Value: "2026-01-31", Hold: setting.HoldSet},
			})},
			wantLocked: "2026-01-31", wantVisibility: VisibilityAggregated, wantSource: setting.LevelInstance, decided: true,
		},
		{
			name: "a set default above yields to the row", stored: row,
			above:      []setting.Layer{layerOf(setting.LevelInstance, map[setting.Key]setting.Entry{VisibilityKey: {Value: "AGGREGATED", Hold: setting.HoldSet}})},
			wantLocked: "2026-09-30", wantVisibility: VisibilityFull, wantSource: setting.LevelWorkspace, decided: true,
		},
		{
			name: "a malformed day above is none", stored: Policy{},
			above:          []setting.Layer{layerOf(setting.LevelInstance, map[setting.Key]setting.Entry{LockedThroughKey: {Value: "soon", Hold: setting.HoldSet}})},
			wantVisibility: VisibilityFull, wantSource: setting.LevelDefault, decided: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := ResolvePolicy(c.stored, c.above...)
			if got := dayText(r.Policy.LockedThrough); got != c.wantLocked {
				t.Fatalf("locked through %q, want %q", got, c.wantLocked)
			}
			if r.Policy.Visibility != c.wantVisibility || r.Visibility.Source != c.wantSource || r.Visibility.LockedBy != c.wantLockedBy {
				t.Fatalf("visibility %s from %s locked by %q", r.Policy.Visibility, r.Visibility.Source, r.Visibility.LockedBy)
			}
			if r.Decided() != c.decided {
				t.Fatalf("decided %v", r.Decided())
			}
			if r.Policy.Version != c.stored.Version {
				t.Fatalf("version %d", r.Policy.Version)
			}
		})
	}
}

// A workspace may restate a value a level above locked, never change it.
func TestALockAboveRefusesAChangeOfTheWorkspacesPolicy(t *testing.T) {
	r := ResolvePolicy(Policy{}, layerOf(setting.LevelPlan, map[setting.Key]setting.Entry{
		VisibilityKey: {Value: "AGGREGATED", Hold: setting.HoldLock},
	}))
	locked := day(t, "2026-09-30")
	if err := r.Admits(Policy{Visibility: VisibilityAggregated, LockedThrough: &locked}); err != nil {
		t.Fatalf("restating: %v", err)
	}
	if err := r.Admits(Policy{Visibility: VisibilityFull}); code(err) != "setting.locked" {
		t.Fatalf("changing: %v", err)
	}
}
