// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// configuration.md §6: a goal period a template brought to the workspace is marked once per key,
// listed with its name and days, and invisible to the workspace next door (migration 0440).
func TestAGoalSetIsMarkedOnceAndStaysInItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	admin := adminPool(ctx, t)
	quarter, other := freshID(t), freshID(t)
	for _, id := range []shared.ID{quarter, other} {
		if _, err := admin.Exec(ctx, `INSERT INTO goal_period (id, tenant_id, name, start_on, end_on, state, created_at, updated_at)
			VALUES ($1, $2, 'Q4 2026', '2026-10-01', '2026-12-31', 'OPEN', now(), now())`, id.String(), tenantA.String()); err != nil {
			t.Fatalf("seeding the period: %v", err)
		}
	}
	sets := postgres.GoalSetRepository{}
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if marked, err := sets.Mark(ctx, quarter, "customer_satisfaction", at); err != nil || !marked {
			t.Errorf("mark %v, %v", marked, err)
		}
		if marked, err := sets.Mark(ctx, other, "customer_satisfaction", at); err != nil || marked {
			t.Errorf("a key marked twice: %v, %v", marked, err)
		}
		listed, err := sets.List(ctx)
		if err != nil || len(listed) != 1 || listed[0].PeriodID != quarter || listed[0].Key != "customer_satisfaction" ||
			listed[0].Name != "Q4 2026" || listed[0].StartOn.Format(time.DateOnly) != "2026-10-01" ||
			listed[0].EndOn.Format(time.DateOnly) != "2026-12-31" {
			t.Errorf("listed %+v, %v", listed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		if listed, err := sets.List(ctx); err != nil || len(listed) != 0 {
			t.Errorf("the workspace next door listed %+v, %v", listed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The mark names its period within its own workspace: another's period is no period there.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := sets.Mark(ctx, other, "team_health", at)
		return err
	}); err == nil {
		t.Error("the workspace next door marked a period of another workspace")
	}
}
