// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-AI-06 check 2, UC-INS-13 check 2: a workspace reads its plan's AI layer in its own scope, and
// a workspace on no plan reads none of it (a cross-tenant negative test).
func TestAWorkspaceReadsItsPlansAiLayer(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	plans := postgres.NewPlanRepository()
	held := newPlan(t, planName("ai"))
	held.Values.Ai.Locks = map[integration.AiSwitch]bool{integration.AiSwitchSources: true}
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, held, "") })
	onPlan, onNone := seedWorkspace(ctx, t, "aiplan"), seedWorkspace(ctx, t, "ainone")
	if _, err := admin.Exec(ctx, `UPDATE tenant SET plan_id = $1 WHERE id = $2`, held.ID.String(), onPlan.String()); err != nil {
		t.Fatal(err)
	}

	read := func(id shared.ID) integration.AiLayer {
		t.Helper()
		var layer integration.AiLayer
		if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: id}, func(ctx context.Context) error {
			var err error
			layer, err = plans.PlanAi(ctx)
			return err
		}); err != nil {
			t.Fatalf("reading the plan's AI layer: %v", err)
		}
		return layer
	}
	if layer := read(onPlan); layer.Sources == nil || *layer.Sources != integration.AiSourcesOffered ||
		!layer.Locks[integration.AiSwitchSources] {
		t.Errorf("the workspace on the plan reads %+v", layer)
	}
	if layer := read(onNone); !layer.IsZero() {
		t.Errorf("a workspace on no plan reads %+v", layer)
	}
	other := newPlan(t, planName("aiother"))
	own := integration.AiSourcesOwn
	other.Values.Ai = integration.AiLayer{Sources: &own, Locks: map[integration.AiSwitch]bool{}}
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, other, "") })
	onOther := seedWorkspace(ctx, t, "aiother")
	if _, err := admin.Exec(ctx, `UPDATE tenant SET plan_id = $1 WHERE id = $2`, other.ID.String(), onOther.String()); err != nil {
		t.Fatal(err)
	}
	if layer := read(onOther); layer.Sources == nil || *layer.Sources != integration.AiSourcesOwn ||
		layer.Locks[integration.AiSwitchSources] {
		t.Errorf("a workspace on another plan reads %+v", layer)
	}
}
