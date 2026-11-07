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

// A sprint, its entries and their estimates stay in their workspace (migration 0252,
// UC-PLN-06 check 6, UC-PLN-07 check 4); an entry is in one open sprint at a time and one
// sprint runs per collection.
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
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		locked, err := sprints.Lock(ctx, first.ID)
		if err != nil || locked.State != planning.SprintActive {
			t.Errorf("locked = %+v, %v", locked, err)
		}
		if n, err := sprints.Count(ctx, first.ID); err != nil || n != 1 {
			t.Errorf("count = %d, %v", n, err)
		}
		return nil
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
		if _, err := sprints.Lock(ctx, first.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace locked the sprint: %v", err)
		}
		if n, err := sprints.Count(ctx, first.ID); err != nil || n != 0 {
			t.Errorf("the other workspace counts the members: %d %v", n, err)
		}
		return sprints.ReturnOpen(ctx, first.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		open, err := sprints.OpenMemberships(ctx, tasks[:1])
		if err == nil && len(open) != 1 {
			t.Errorf("the other workspace returned the entries: %+v", open)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return sprints.ReturnOpen(ctx, first.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		open, err := sprints.OpenMemberships(ctx, tasks[:1])
		if err == nil && len(open) != 0 {
			t.Errorf("a returned entry is still held: %+v", open)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return sprints.Delete(ctx, first.ID)
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the other workspace deleted the sprint: %v", err)
	}
}

// The walk's sprints, open memberships and estimates stay in their workspace (offline-sync.md
// §3.1): the workspace next door walks none of them.
func TestTheSprintWalkStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	sprints := postgres.NewPlanningRepository().Sprints()
	snapshot := postgres.NewSnapshotRepository()
	_, collection, tasks := seedPlan(ctx, t, tenantA)
	now := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	start := planning.DayFrom(2026, time.November, 2)
	sprint, err := planning.NewSprint(planning.NewSprintInput{
		ID: freshID(t), TenantID: tenantA, CollectionID: collection, CreatedBy: authorA, Now: now,
		Fields: planning.SprintFields{Name: "Walked", StartOn: start, EndOn: start + 13},
	})
	if err != nil {
		t.Fatal(err)
	}
	estimate, _ := planning.NewEstimate(tasks[0], planning.Points, 3, "")
	estimate.UpdatedAt = now
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := sprints.Insert(ctx, sprint); err != nil {
			return err
		}
		if err := sprints.Add(ctx, sprint.ID, tasks[0], now, false); err != nil {
			return err
		}
		return sprints.SetEstimate(ctx, estimate)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	walked := func(tenant shared.ID) (sprintSeen, memberSeen, estimateSeen bool) {
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			all, err := snapshot.Sprints(ctx, "", 5000)
			if err != nil {
				return err
			}
			for _, s := range all {
				sprintSeen = sprintSeen || s.ID == sprint.ID
			}
			members, err := snapshot.SprintEntries(ctx, "", 5000)
			if err != nil {
				return err
			}
			for _, m := range members {
				memberSeen = memberSeen || (m.EntryID == tasks[0] && m.SprintID == sprint.ID && m.CollectionID == collection)
			}
			estimates, err := snapshot.EntryEstimates(ctx, "", 5000)
			for _, e := range estimates {
				estimateSeen = estimateSeen || (e.Estimate.EntryID == tasks[0] && e.Estimate.Value == 3 && e.CollectionID == collection)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return
	}
	if s, m, e := walked(tenantA); !s || !m || !e {
		t.Errorf("the workspace's walk: sprint %v, membership %v, estimate %v; want all three", s, m, e)
	}
	if s, m, e := walked(tenantB); s || m || e {
		t.Errorf("the workspace next door walked: sprint %v, membership %v, estimate %v", s, m, e)
	}
}

// UC-WRK-34 check 8: the running sprints a person is in are those holding an entry assigned to
// them, with the hub and how many they hold; a planned sprint, another person's or a removed
// entry does not count, and another workspace reads none of them.
func TestTheRunningSprintsOfAPersonStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	sprints := postgres.NewPlanningRepository().Sprints()
	hub, collection, tasks := seedPlan(ctx, t, tenantA)
	_, other, more := seedPlan(ctx, t, tenantA)
	admin := adminPool(ctx, t)
	for _, task := range []shared.ID{tasks[0], tasks[1], more[0]} {
		if _, err := admin.Exec(ctx, `UPDATE work_item SET assignee_id = $1 WHERE id = $2`, authorA.String(), task.String()); err != nil {
			t.Fatalf("assigning: %v", err)
		}
	}
	now := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	start := planning.DayFrom(2026, time.November, 2)
	newSprint := func(in shared.ID, name string, days int) planning.Sprint {
		s, err := planning.NewSprint(planning.NewSprintInput{
			ID: freshID(t), TenantID: tenantA, CollectionID: in, CreatedBy: authorA, Now: now,
			Fields: planning.SprintFields{Name: name, StartOn: start, EndOn: start + planning.Day(days)},
		})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	running, planned := newSprint(collection, "Running", 13), newSprint(other, "Planned", 13)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, s := range []planning.Sprint{running, planned} {
			if err := sprints.Insert(ctx, s); err != nil {
				return err
			}
		}
		for _, task := range tasks {
			if err := sprints.Add(ctx, running.ID, task, now, false); err != nil {
				return err
			}
		}
		if err := sprints.Add(ctx, planned.ID, more[0], now, false); err != nil {
			return err
		}
		if _, err := sprints.Remove(ctx, running.ID, tasks[1], now); err != nil {
			return err
		}
		started := running
		started.State, started.StartedAt = planning.SprintActive, &now
		return sprints.Update(ctx, started, running.Version)
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		mine, err := sprints.Mine(ctx, authorA, 20)
		if err != nil {
			return err
		}
		if len(mine) != 1 || mine[0].Sprint.ID != running.ID || mine[0].HubID != hub || mine[0].EntryID != tasks[0] || mine[0].Entries != 1 {
			t.Errorf("mine = %+v, want the running sprint with one entry", mine)
		}
		nobody, err := sprints.Mine(ctx, freshID(t), 20)
		if err == nil && len(nobody) != 0 {
			t.Errorf("a person without entries is in %+v", nobody)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		mine, err := sprints.Mine(ctx, authorA, 20)
		if err == nil && len(mine) != 0 {
			t.Errorf("the other workspace reads the sprints: %+v", mine)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
