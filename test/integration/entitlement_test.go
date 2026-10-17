// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// What a plan leaves out is read in the workspace's own scope, by the one-statement reader the
// guards use and by the workspace read the sign-in readers hold - the same set both ways. A
// workspace on no plan, or on another, never reads this one's (a cross-tenant negative test).
func TestAWorkspaceReadsWhatItsOwnPlanLeavesOut(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	plans := postgres.NewPlanRepository()
	workspaces := postgres.NewWorkspaceSettingsRepository()

	private := newPlan(t, planName("leftout"))
	private.Values.LeftOut = []entitlement.Feature{entitlement.OwnSignInProviders, entitlement.AutomationRules}
	business := newPlan(t, planName("whole"))
	business.Values.LeftOut = nil
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, private, "") })
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, business, "") })

	onPrivate := seedWorkspace(ctx, t, "leftout")
	onBusiness := seedWorkspace(ctx, t, "whole")
	onNone := seedWorkspace(ctx, t, "noplan")
	for workspace, on := range map[shared.ID]shared.ID{onPrivate: private.ID, onBusiness: business.ID} {
		if _, err := admin.Exec(ctx, `UPDATE tenant SET plan_id = $1 WHERE id = $2`, on.String(), workspace.String()); err != nil {
			t.Fatal(err)
		}
	}

	read := func(id shared.ID) (entitlement.LeftOut, entitlement.LeftOut) {
		t.Helper()
		var byReader, byWorkspace entitlement.LeftOut
		if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: id}, func(ctx context.Context) error {
			var err error
			if byReader, err = plans.LeftOut(ctx); err != nil {
				return err
			}
			found, err := workspaces.Find(ctx)
			byWorkspace = found.Plan.LeftOut
			return err
		}); err != nil {
			t.Fatalf("reading what the plan leaves out: %v", err)
		}
		return byReader, byWorkspace
	}

	want := entitlement.LeftOut{entitlement.AutomationRules, entitlement.OwnSignInProviders}
	if byReader, byWorkspace := read(onPrivate); !slices.Equal(byReader, want) || !slices.Equal(byWorkspace, want) {
		t.Errorf("the workspace on the plan read %v and %v, want %v", byReader, byWorkspace, want)
	}
	for name, other := range map[string]shared.ID{"on a plan leaving out nothing": onBusiness, "on no plan": onNone} {
		if byReader, byWorkspace := read(other); len(byReader) != 0 || len(byWorkspace) != 0 {
			t.Errorf("a workspace %s read %v and %v", name, byReader, byWorkspace)
		}
	}

	// The read the registry's gate and the capabilities answer by, in the actor's own workspace.
	gate := adminservice.EntitlementGate{Plans: plans, UnitOfWork: uow}
	for workspace, expected := range map[shared.ID]entitlement.LeftOut{onPrivate: want, onBusiness: nil, onNone: nil} {
		actor := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: workspace, AccountID: freshID(t)}
		got, err := gate.LeftOut(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, expected) {
			t.Errorf("the gate read %v in %s, want %v", got, workspace, expected)
		}
	}
}
