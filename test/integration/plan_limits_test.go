// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-INS-10 check 1, UC-INS-13 checks 2 and 4 (multi-tenancy.md §4.2): a workspace's limits resolve
// through its plan, read in its own scope; a plan's change is in force at the next request with no
// job; a creation past a lowered plan limit is refused and nothing is removed; a workspace on no
// plan reads none of it (a cross-tenant negative test).
func TestAWorkspacesLimitsResolveThroughItsPlan(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	plans := postgres.NewPlanRepository()
	store := postgres.NewQuotaRepository()
	guard := quotaservice.Guard{Store: store, Usage: store, Meter: store, Tenancy: env.TenancyMulti}

	small := newPlan(t, planName("limits"))
	small.Values.Quotas = identity.QuotaDefaults{
		Limits: map[string]int64{quotaservice.Items: 1}, Locks: map[string]bool{quotaservice.Items: true},
	}
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, small, "") })
	onPlan, onNone := seedWorkspace(ctx, t, "limited"), seedWorkspace(ctx, t, "unlimited")
	if _, err := admin.Exec(ctx, `UPDATE tenant SET plan_id = $1 WHERE id = $2`, small.ID.String(), onPlan.String()); err != nil {
		t.Fatal(err)
	}

	creating := func(id shared.ID, adding int64) error {
		t.Helper()
		var refusal error
		if err := uow.Within(ctx, persistence.Scope{TenantID: id}, func(ctx context.Context) error {
			refusal = guard.Items(ctx, id.String(), adding)
			return nil
		}); err != nil {
			t.Fatalf("the guard's transaction: %v", err)
		}
		return refusal
	}
	standing := func(id shared.ID) quotaservice.Standing {
		t.Helper()
		var found quotaservice.Standing
		if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: id}, func(ctx context.Context) error {
			standings, err := quotaservice.Standings(ctx, store, store, env.TenancyMulti, time.Now())
			for _, s := range standings {
				if s.Quota == quotaservice.Items {
					found = s
				}
			}
			return err
		}); err != nil {
			t.Fatalf("reading the standings: %v", err)
		}
		return found
	}

	err := creating(onPlan, 2)
	if got := shared.AsError(err); got == nil || got.DetailCode != "capacity.items" || got.Params["limit"] != "1" {
		t.Errorf("a creation past the plan's limit answered %v, want capacity.items at 1", err)
	}
	if items := standing(onPlan); items.Limit != 1 || items.Source != quotaservice.SourcePlan ||
		items.Lock != quotaservice.LockPlan || items.Used == nil || *items.Used != 0 {
		t.Errorf("the workspace on the plan reads %+v", items)
	}
	if err := creating(onNone, 2); err != nil {
		t.Errorf("a workspace on no plan met the plan's limit: %v", err)
	}
	if items := standing(onNone); items.Source != quotaservice.SourceDefault || items.Lock != "" {
		t.Errorf("a workspace on no plan reads %+v", items)
	}

	// The plan changed: in force at the next request, nothing visited the workspace.
	if _, err := admin.Exec(ctx, `UPDATE plan_setting SET value = '10' WHERE plan_id = $1 AND key = 'quota.items'`,
		small.ID.String()); err != nil {
		t.Fatal(err)
	}
	if err := creating(onPlan, 2); err != nil {
		t.Errorf("after the plan was raised the creation answered %v", err)
	}
}
