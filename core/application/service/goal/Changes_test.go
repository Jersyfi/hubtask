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

// UC-GOL-03 offline: a portfolio's writes tell offline devices under no hub, every project by its
// container and nothing computed for the writer; a deletion records the portfolios handed to its
// parent.
func TestAPortfoliosWritesAreInTheChangeLog(t *testing.T) {
	s, _, _, _ := newSteward(t)
	log := &changes{}
	s.Changes, s.HLC = log, stamps{}
	parent := ok(t, CreatePortfolio{s}.Descriptor(), ana, usecase.Input{
		"name": "Programme", "items": []any{map[string]any{"container_id": listA.String(), "status": "ON_TRACK"}},
	})
	child := ok(t, CreatePortfolio{s}.Descriptor(), ana, usecase.Input{"name": "Stream", "parent_id": parent.String("id")})
	ok(t, ReportPortfolioStatus{s}.Descriptor(), ana, usecase.Input{"portfolio_id": parent.String("id"), "status": "AT_RISK", "summary": "late"})
	ok(t, DeletePortfolio{s}.Descriptor(), ana, usecase.Input{"portfolio_id": parent.String("id"), "expected_version": 2})

	var seen []string
	for _, c := range log.recorded {
		seen = append(seen, c.Entity+"/"+string(c.Op))
		if !c.ContainerID.IsZero() {
			t.Errorf("a portfolio filed under %q, want none", c.ContainerID)
		}
	}
	want := []string{"portfolio/UPSERT", "portfolio/UPSERT", "portfolio/UPSERT", "portfolio/DELETE", "portfolio/UPSERT"}
	if len(seen) != len(want) {
		t.Fatalf("recorded %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("recorded %v, want %v", seen, want)
		}
	}
	first := log.recorded[0].Payload
	items, _ := first["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["container_id"] != listA.String() {
		t.Errorf("the projects = %v, want each by its container", first["items"])
	}
	if _, named := items[0].(map[string]any)["name"]; named {
		t.Errorf("a project's record carries a name read for the writer: %v", items[0])
	}
	if reported := log.recorded[2].Payload; reported["status"] != "AT_RISK" {
		t.Errorf("a status report recorded %v", reported)
	}
	if handed := log.recorded[4]; handed.EntityID != shared.ID(child.String("id")) || handed.Payload["parent_id"] != nil {
		t.Errorf("the child handed on = %+v, want it without a parent", handed)
	}
}
