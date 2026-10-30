// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	planrepo "github.com/Jersyfi/hubtask/core/application/repository/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// seedPlan writes a hub with one collection and two dated tasks, and answers the hub, the
// collection and the tasks.
func seedPlan(ctx context.Context, t *testing.T, tenant shared.ID) (shared.ID, shared.ID, []shared.ID) {
	t.Helper()
	admin := adminPool(ctx, t)
	hub, collection := freshID(t), freshID(t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		VALUES ($1, $2, 'HUB', $3, 'a0', $4)`, hub.String(), tenant.String(), freshName(t), authorA.String()); err != nil {
		t.Fatalf("seeding the hub: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO container (id, tenant_id, type, parent_id, name, order_key, created_by)
		VALUES ($1, $2, 'COLLECTION', $3, $4, 'a0', $5)`,
		collection.String(), tenant.String(), hub.String(), freshName(t), authorA.String()); err != nil {
		t.Fatalf("seeding the collection: %v", err)
	}
	var tasks []shared.ID
	for i, due := range []string{"2026-11-04T00:00:00Z", "2026-11-06T00:00:00Z"} {
		item := freshID(t)
		if _, err := admin.Exec(ctx, `
			INSERT INTO work_item (id, tenant_id, collection_id, type, path, depth, title, order_key,
			                       content_language, created_by, start_at, due_at)
			VALUES ($1, $2, $3, 'TASK', $4, 1, 'Planned', $5, 'en', $6, '2026-11-02T00:00:00Z', $7)`,
			item.String(), tenant.String(), collection.String(), "/"+item.String(), []string{"a0", "a1"}[i],
			authorA.String(), due); err != nil {
			t.Fatalf("seeding a task: %v", err)
		}
		tasks = append(tasks, item)
	}
	return hub, collection, tasks
}

// A plan's entries, their modes and its baselines stay in their workspace (migration 0244, M2).
func TestAPlanStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	plans := postgres.NewPlanningRepository()
	baselines := plans.Baselines()
	hub, _, tasks := seedPlan(ctx, t, tenantA)
	now := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	baseline, err := planning.NewBaseline(planning.NewBaselineInput{
		ID: freshID(t), TenantID: tenantA, PlanID: hub, TakenBy: authorA, Name: "Agreed", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := plans.SetSchedule(ctx, planrepo.EntrySchedule{EntryID: tasks[1], Mode: planning.Automatic, UpdatedAt: now}); err != nil {
			return err
		}
		copied, err := baselines.Take(ctx, baseline, planning.MaxBaselineEntries)
		if err == nil && copied != 2 {
			t.Errorf("the baseline copied %d entries, want 2", copied)
		}
		return err
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		entries, err := plans.Entries(ctx, hub, 100)
		if err != nil {
			return err
		}
		if len(entries) != 2 || entries[1].Mode != planning.Automatic || entries[0].Mode != planning.Manual {
			t.Errorf("entries = %+v", entries)
		}
		found, err := baselines.Find(ctx, baseline.ID)
		if err == nil && (len(found.Entries) != 2 || found.EntryCount != 2 || found.Name != "Agreed") {
			t.Errorf("baseline = %+v", found)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if entries, err := plans.Entries(ctx, hub, 100); err != nil || len(entries) != 0 {
			t.Errorf("the other workspace reads the plan: %v %v", entries, err)
		}
		if _, err := baselines.Find(ctx, baseline.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the other workspace reads the baseline: %v", err)
		}
		if modes, err := plans.Schedules(ctx, tasks); err != nil || len(modes) != 0 {
			t.Errorf("the other workspace reads the modes: %v %v", modes, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return baselines.Delete(ctx, baseline.ID)
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the other workspace deleted the baseline: %v", err)
	}
}
