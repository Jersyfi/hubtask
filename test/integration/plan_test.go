// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/plan"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Plans are installation-wide and never deleted, and the integration tests share one database
// (known traps), so every short name carries this run's suffix.
func planName(stem string) string {
	return fmt.Sprintf("%s-%d", stem, time.Now().UnixNano()%1_000_000_000)
}

func newPlan(t *testing.T, name string) plan.Plan {
	t.Helper()
	minLength := 16
	offered := integration.AiSourcesOffered
	now := time.Now().UTC().Truncate(time.Microsecond)
	return plan.Plan{
		ID: freshID(t), Name: name, DisplayName: "Family", Version: 1,
		CreatedAt: now, UpdatedAt: now,
		Values: plan.Values{
			Policy: identity.PolicyLayer{
				Patch: identity.PolicyPatch{MinLength: &minLength},
				Locks: map[identity.PolicySwitch]bool{identity.SwitchMinLength: true},
			},
			Legal: identity.LegalLayer{
				Links: identity.LegalLinks{}.With(identity.LinkTerms, "https://provider.example/family-terms"),
				Locks: map[identity.LegalLink]bool{},
			},
			Quotas:  identity.QuotaDefaults{Limits: map[string]int64{"items": 5000}, Locks: map[string]bool{"items": true}},
			Ai:      integration.AiLayer{Sources: &offered, Locks: map[integration.AiSwitch]bool{}},
			LeftOut: []plan.Feature{plan.FeatureWebhooks, plan.FeatureAutomationRules},
		},
	}
}

// A plan's values make the round trip through the installation level's key shape, and the
// features it leaves out through their own rows (UC-INS-13 check 1).
func TestAPlanRoundTripsThroughTheDatabase(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	plans := postgres.NewPlanRepository()
	written := newPlan(t, planName("round"))

	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, written, "") })

	var read plan.Plan
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		var err error
		read, err = plans.Find(ctx, written.Name)
		return err
	}); err != nil {
		t.Fatalf("reading the plan: %v", err)
	}
	if read.DisplayName != "Family" || read.Version != 1 {
		t.Errorf("read %+v", read)
	}
	if read.Values.Policy.Patch.MinLength == nil || *read.Values.Policy.Patch.MinLength != 16 ||
		!read.Values.Policy.Locks[identity.SwitchMinLength] {
		t.Errorf("the sign-in value came back as %+v", read.Values.Policy)
	}
	if read.Values.Quotas.Limits["items"] != 5000 || !read.Values.Quotas.Locked("items") {
		t.Errorf("the limit came back as %+v", read.Values.Quotas)
	}
	if read.Values.Legal.Links.Of(identity.LinkTerms) != "https://provider.example/family-terms" {
		t.Errorf("the terms came back as %+v", read.Values.Legal)
	}
	if read.Values.Ai.Sources == nil || *read.Values.Ai.Sources != integration.AiSourcesOffered {
		t.Errorf("the AI rule came back as %+v", read.Values.Ai)
	}
	// In the domain's order, whatever order they were written in.
	if len(read.Values.LeftOut) != 2 || read.Values.LeftOut[0] != plan.FeatureAutomationRules ||
		read.Values.LeftOut[1] != plan.FeatureWebhooks {
		t.Errorf("left out %v", read.Values.LeftOut)
	}
}

// A replace is a PUT, guarded on the version read.
func TestAPlanIsReplacedOnTheVersionItWasReadAt(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	plans := postgres.NewPlanRepository()
	written := newPlan(t, planName("replace"))
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, written, "") })

	changed := written
	changed.DisplayName = "Family plus"
	changed.Values = plan.Values{}
	stale := 7
	inSystem(ctx, t, uow, func(ctx context.Context) error {
		replaced, err := plans.Replace(ctx, changed, &stale, "", time.Now())
		if err == nil && replaced {
			t.Error("a stale version replaced the plan")
		}
		return err
	})
	current := 1
	inSystem(ctx, t, uow, func(ctx context.Context) error {
		replaced, err := plans.Replace(ctx, changed, &current, "", time.Now())
		if err == nil && !replaced {
			t.Error("the version read did not replace the plan")
		}
		return err
	})
	inSystem(ctx, t, uow, func(ctx context.Context) error {
		read, err := plans.Find(ctx, written.Name)
		if err == nil && (read.Version != 2 || read.Values.Quotas.Limits["items"] != 0 || len(read.Values.LeftOut) != 0) {
			t.Errorf("after the replace: version %d, values %+v", read.Version, read.Values)
		}
		return err
	})
}

// Two creates of one name at once: one plan, and the other refused as taken - not a 500
// (UC-INS-13 check 9, the name being what a platform maps its products to).
func TestTwoCreatesOfOneNameMakeOnePlan(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	plans := postgres.NewPlanRepository()
	name := planName("race")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
				return plans.Insert(ctx, newPlan(t, name), "")
			})
		}()
	}
	wg.Wait()
	taken := 0
	for _, err := range errs {
		var refusal *shared.Error
		switch {
		case err == nil:
		case errors.As(err, &refusal) && refusal.DetailCode == "admin.plan_name_taken":
			taken++
		default:
			t.Errorf("a create answered %v", err)
		}
	}
	if taken != 1 {
		t.Errorf("%d of two creates were refused as taken, want 1", taken)
	}
}

// A removed plan's name stays taken, and a plan a workspace is on is not removed - which only
// the door can see, across tenant_self (UC-INS-13 check 8).
func TestAPlanInUseIsNotRemovedAndARemovedNameStaysTaken(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	plans := postgres.NewPlanRepository()
	used := newPlan(t, planName("used"))
	free := newPlan(t, planName("free"))
	inSystem(ctx, t, uow, func(ctx context.Context) error {
		if err := plans.Insert(ctx, used, ""); err != nil {
			return err
		}
		return plans.Insert(ctx, free, "")
	})

	tenant := freshID(t)
	if _, err := admin.Exec(ctx, `INSERT INTO tenant (id, slug, display_name, plan_id) VALUES ($1, $2, 'On a plan', $3)`,
		tenant.String(), "plan-"+used.Name[len(used.Name)-9:], used.ID.String()); err != nil {
		t.Fatalf("seeding a workspace on the plan: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `UPDATE tenant SET plan_id = NULL, deleted_at = now() WHERE id = $1`, tenant.String())
	})

	remove := func(target plan.Plan) adminrepo.RemoveOutcome {
		var outcome adminrepo.RemoveOutcome
		inSystem(ctx, t, uow, func(ctx context.Context) error {
			var err error
			outcome, err = plans.Remove(ctx, target.ID, "", time.Now())
			return err
		})
		return outcome
	}
	if outcome := remove(used); outcome != adminrepo.PlanInUse {
		t.Errorf("removing a plan a workspace is on answered %s", outcome)
	}
	if outcome := remove(free); outcome != adminrepo.PlanRemoved {
		t.Errorf("removing a free plan answered %s", outcome)
	}

	err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		return plans.Insert(ctx, newPlan(t, free.Name), "")
	})
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != "admin.plan_name_taken" {
		t.Errorf("re-creating a removed plan's name answered %v", err)
	}

	// The count the control plane answers, through its own enumerator.
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		counts, err := plans.Workspaces(ctx)
		if err == nil && counts[used.Name] != 1 {
			t.Errorf("workspaces on the plan: %d, want 1", counts[used.Name])
		}
		return err
	}); err != nil {
		t.Fatalf("counting: %v", err)
	}
}

// The application role holds no DELETE on plan: a short name is never reused by construction.
func TestTheApplicationRoleCannotDeleteAPlan(t *testing.T) {
	ctx := context.Background()
	app := appPool(ctx, t)
	if _, err := app.Exec(ctx, `DELETE FROM plan WHERE false`); err == nil {
		t.Error("the application role may delete from plan")
	}
}
