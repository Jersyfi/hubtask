// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type changes struct{ recorded []changelog.Change }

func (c *changes) Record(_ context.Context, change changelog.Change) error {
	c.recorded = append(c.recorded, change)
	return nil
}

type stamps struct{}

func (stamps) Next() shared.HLC { return shared.HLC{} }

// A goal's, a check-in's and a dashboard's writes tell offline devices, each filed under its hub;
// a goal's record carries no number computed for the writer (offline-sync.md §5).
func TestTheModulesWritesAreInTheChangeLog(t *testing.T) {
	s, _, _, _ := newSteward(t)
	log := &changes{}
	s.Changes, s.HLC = log, stamps{}
	periodID := period(t, s)
	g := ok(t, CreateGoal{s}.Descriptor(), ana, usecase.Input{"period_id": periodID, "hub_id": hubA.String(), "title": "Ship"})
	ok(t, CheckInGoal{s}.Descriptor(), ana, usecase.Input{"goal_id": g.String("id"), "status": "ON_TRACK", "confidence": 6})
	d := ok(t, CreateDashboard{s}.Descriptor(), ana, usecase.Input{"name": "Mine", "hub_id": hubA.String()})
	ok(t, DeleteDashboard{s}.Descriptor(), ana, usecase.Input{"dashboard_id": d.String("id"), "expected_version": 1})

	var seen []string
	for _, c := range log.recorded {
		seen = append(seen, c.Entity+"/"+string(c.Op))
		if c.Entity != PeriodEntity && c.ContainerID != hubA {
			t.Errorf("%s filed under %q, want its hub", c.Entity, c.ContainerID)
		}
		if c.Entity == GoalEntity {
			if _, computed := c.Payload["progress"]; computed {
				t.Errorf("a goal's record carries progress: %v", c.Payload)
			}
		}
	}
	want := []string{"goal_period/UPSERT", "goal/UPSERT", "goal_check_in/UPSERT", "goal/UPSERT", "dashboard/UPSERT", "dashboard/DELETE"}
	if len(seen) != len(want) {
		t.Fatalf("recorded %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("recorded %v, want %v", seen, want)
		}
	}
}
