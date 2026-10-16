// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A workspace carries its plan's sign-in and legal layers, read in its own scope; a workspace on no
// plan, or on another, never reads this one's (a cross-tenant negative test).
func TestAWorkspaceReadsItsOwnPlan(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	plans := postgres.NewPlanRepository()
	workspaces := postgres.NewWorkspaceSettingsRepository()
	family := newPlan(t, planName("res"))
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, family, "") })

	onPlan, onNone := seedWorkspace(ctx, t, "onplan"), seedWorkspace(ctx, t, "onnone")
	if _, err := admin.Exec(ctx, `UPDATE tenant SET plan_id = $1 WHERE id = $2`, family.ID.String(), onPlan.String()); err != nil {
		t.Fatal(err)
	}

	read := func(id shared.ID) identity.Workspace {
		t.Helper()
		var found identity.Workspace
		if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: id}, func(ctx context.Context) error {
			var err error
			found, err = workspaces.Find(ctx)
			return err
		}); err != nil {
			t.Fatalf("reading the workspace: %v", err)
		}
		return found
	}

	held := read(onPlan)
	if held.Plan.Name != family.Name || held.Plan.DisplayName != "Family" ||
		held.Plan.Policy.Patch.MinLength == nil || *held.Plan.Policy.Patch.MinLength != 16 ||
		!held.Plan.Policy.Locks[identity.SwitchMinLength] ||
		held.Plan.Legal.Links.Of(identity.LinkTerms) != "https://provider.example/family-terms" {
		t.Errorf("the workspace on the plan read %+v", held.Plan)
	}
	if other := read(onNone); other.Plan.Name != "" || other.Plan.Policy.Patch.MinLength != nil {
		t.Errorf("a workspace on no plan read %+v", other.Plan)
	}
}
