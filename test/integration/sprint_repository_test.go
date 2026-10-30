// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A sprint, its entries and their estimates stay in their workspace (migration 0245, UC-PLN-06
// check 6, UC-PLN-07 check 4); an entry is in one open sprint at a time and one sprint runs per
// collection.
func TestASprintStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	sprints := postgres.NewPlanningRepository().Sprints()
	_, collection, tasks := seedPlan(ctx, t, tenantA)
	now := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	start := planning.DayFrom(2026, time.November, 2)
	newSprint := func(name string) planning.Sprint {
		s, err := planning.NewSprint(planning.NewSprintInput{
			ID: freshID(t), TenantID: tenantA, CollectionID: collection, CreatedBy: authorA, Now: now,
			Fields: planning.SprintFields{Name: name, Goal: "Ship it", StartOn: start, EndOn: start + 13},
		})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	first, second := newSprint("One"), newSprint("Two")
	estimate, _ := planning.NewEstimate(tasks[0], planning.Points, 5, "")
	estimate.UpdatedAt = now
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, s := range []planning.Sprint{first, second} {
			if err := sprints.Insert(ctx, s); err != nil {
				return err
			}
		}
		if err := sprints.Add(ctx, first.ID, tasks[0], now, false); err != nil {
			return err
		}
		if err := sprints.SetEstimate(ctx, estimate); err != nil {
			return err
		}
		running := first
		running.State, running.StartedAt = planning.SprintActive, &now
		return sprints.Update(ctx, running, first.Version)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return sprints.Add(ctx, second.ID, tasks[0], now, false)
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("an entry joined a second open sprint: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		running := second
		running.State = planning.SprintActive
		return sprints.Update(ctx, running, second.Version)
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a second sprint ran in one collection: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		members, err := sprints.Entries(ctx, first.ID, 100)
		if err != nil {
			return err
		}
		if len(members) != 1 || members[0].EntryID != tasks[0] || members[0].CollectionID != collection {
			t.Errorf("members = %+v", members)
		}
		found, err := sprints.Find(ctx, first.ID)
		if err == nil && (found.State != planning.SprintActive || found.StartOn != start || found.Goal != "Ship it") {
			t.Errorf("sprint = %+v", found)
		}
		estimates, err := sprints.Estimates(ctx, tasks)
		if err == nil && estimates[tasks[0]].Value != 5 {
			t.Errorf("estimates = %+v", estimates)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := sprints.Find(ctx, first.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace reads the sprint: %v", err)
		}
		if listed, err := sprints.List(ctx, collection, "", 50); err != nil || len(listed) != 0 {
			t.Errorf("the other workspace lists sprints: %v %v", listed, err)
		}
		if members, err := sprints.Entries(ctx, first.ID, 100); err != nil || len(members) != 0 {
			t.Errorf("the other workspace reads the members: %v %v", members, err)
		}
		if estimates, err := sprints.Estimates(ctx, tasks); err != nil || len(estimates) != 0 {
			t.Errorf("the other workspace reads the estimates: %v %v", estimates, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return sprints.Delete(ctx, first.ID)
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the other workspace deleted the sprint: %v", err)
	}
}
