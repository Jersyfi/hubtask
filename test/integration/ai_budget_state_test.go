// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	integrationservice "github.com/Jersyfi/hubtask/core/application/service/integration"
	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-AI-04 check 9's "said before asking", against PostgreSQL as the application role: the ledger
// the budget wrapper meters, the guard's ceiling from the workspace's settings, and the choice the
// workspace made - read by somebody who holds no role at all, because none is asked.
func TestTheBudgetsStateIsReadFromTheLedgerOfTheChosenSource(t *testing.T) {
	world := newResolverWorld(t)
	world.rule(t, domain.AiSourcesEither, domain.AiMinAny)
	ctx := context.Background()
	admin := adminPool(ctx, t)
	t.Cleanup(func() {
		done := context.WithoutCancel(ctx)
		_, _ = admin.Exec(done, `UPDATE tenant SET settings = settings - 'ai_own_tokens_per_day' WHERE id = $1`,
			resolverTenant.String())
		_, _ = admin.Exec(done, `DELETE FROM usage_record WHERE tenant_id = $1`, resolverTenant.String())
	})
	if _, err := admin.Exec(ctx,
		`UPDATE tenant SET settings = jsonb_set(settings, '{ai_own_tokens_per_day}', '100') WHERE id = $1`,
		resolverTenant.String()); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 1, 10, 30, 0, 0, time.UTC)
	store := postgres.NewQuotaRepository()
	guard := quotaservice.Guard{Store: store, Usage: store, Meter: store, Tenancy: env.TenancyMulti}
	registry, err := usecase.NewRegistry(nil, integrationservice.ReadAiBudgetState{
		Choices: postgres.NewAiChoiceRepository(), Budget: guard,
		UnitOfWork: world.uow, Clock: portclock.Fixed(now),
	}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	// Somebody of the workspace with no membership anywhere: a guest or an auditor reads the same.
	nobody := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: resolverTenant, AccountID: shared.ID("0192f000-0000-7000-8000-0000000b0d9e"),
		Scopes: []string{"items:read"},
	}
	stateNow := func() (string, any) {
		t.Helper()
		out, err := registry.Invoke(ctx, integrationservice.ReadAiBudgetStateName, nobody, usecase.Input{})
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		return out.String("state"), out["renews_at"]
	}

	world.choose(t, domain.AiSourceOwn, "")
	if state, renews := stateNow(); state != "OK" || renews == nil {
		t.Errorf("an untouched own budget reads %s, renews %v", state, renews)
	}

	inTenant(t, world.uow, resolverTenant, func(ctx context.Context) error {
		return guard.MeterAi(ctx, true, now, 91)
	})
	if state, _ := stateNow(); state != "NEARLY_SPENT" {
		t.Errorf("91 of 100 reads %s, want NEARLY_SPENT", state)
	}

	inTenant(t, world.uow, resolverTenant, func(ctx context.Context) error {
		return guard.MeterAi(ctx, true, now, 9)
	})
	if state, renews := stateNow(); state != "SPENT" || renews != "2026-10-02T00:00:00Z" {
		t.Errorf("100 of 100 reads %s, renews %v; want SPENT at the workspace's midnight", state, renews)
	}

	// The offered model's budget is another ledger row: nothing of it was spent.
	world.choose(t, domain.AiSourceOffered, resolverModel)
	if state, _ := stateNow(); state != "OK" {
		t.Errorf("the offered model, unspent, reads %s", state)
	}

	world.choose(t, domain.AiSourceNone, "")
	if state, renews := stateNow(); state != "OFF" || renews != nil {
		t.Errorf("no source reads %s, renews %v; want OFF and null", state, renews)
	}
	world.choose(t, domain.AiSourceOwn, "")
}
