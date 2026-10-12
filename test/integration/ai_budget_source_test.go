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
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/infrastructure/ai"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-AI-07 check 2 and UC-AI-06 check 6, through the real resolver, the real guard and the ledger
// as the app role: a call is counted against the budget of the source that answers it, on the
// workspace's own calendar day; the offered model's spent budget does not stop the workspace's own
// model, which only the workspace's own budget bounds.
func TestEachSourceIsCountedAgainstItsOwnBudgetOnTheWorkspacesDay(t *testing.T) {
	world := newResolverWorld(t)
	world.rule(t, domain.AiSourcesEither, domain.AiMinAny)
	ctx := context.Background()
	admin := adminPool(ctx, t)

	// A workspace far east of UTC: 10:30 UTC on the 1st is already the 2nd there.
	if _, err := admin.Exec(ctx, `UPDATE tenant SET default_time_zone = 'Pacific/Kiritimati' WHERE id = $1`,
		resolverTenant.String()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		done := context.WithoutCancel(ctx)
		_, _ = admin.Exec(done, `UPDATE tenant SET default_time_zone = 'UTC',
			settings = settings - 'ai_own_tokens_per_day' WHERE id = $1`, resolverTenant.String())
		_, _ = admin.Exec(done, `DELETE FROM usage_record WHERE tenant_id = $1`, resolverTenant.String())
	})

	now := time.Date(2026, 10, 1, 10, 30, 0, 0, time.UTC)
	store := postgres.NewQuotaRepository()
	guard := quotaservice.Guard{Store: store, Usage: store, Meter: store, Tenancy: env.TenancyMulti}
	budgeted := integrationservice.Budgeted{
		Providers: world.resolver, Budget: guard, UnitOfWork: world.uow, Clock: portclock.Fixed(now),
	}
	actor := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: resolverTenant}
	answers := func() string {
		t.Helper()
		provider, err := budgeted.For(ctx, actor)
		if err != nil {
			t.Fatalf("resolving: %v", err)
		}
		if provider.Capabilities().Kind == ai.NoopKind {
			return "nothing"
		}
		return "a model"
	}

	// The offered model spends its whole default budget today.
	inTenant(t, world.uow, resolverTenant, func(ctx context.Context) error {
		return guard.MeterAi(ctx, false, now, quotaservice.Defaults(env.TenancyMulti).AiTokensPerDay)
	})
	var period, metric string
	if err := admin.QueryRow(ctx,
		`SELECT period::text, metric FROM usage_record WHERE tenant_id = $1`, resolverTenant.String(),
	).Scan(&period, &metric); err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}
	if period != "2026-10-02" || metric != quotaservice.AiTokensPerDay {
		t.Errorf("the offered call was metered as %s on %s, want %s on the workspace's 2026-10-02",
			metric, period, quotaservice.AiTokensPerDay)
	}

	world.choose(t, domain.AiSourceOffered, resolverModel)
	if got := answers(); got != "nothing" {
		t.Errorf("the offered model past its budget answers %s", got)
	}
	world.choose(t, domain.AiSourceOwn, "")
	if got := answers(); got != "a model" {
		t.Errorf("the own model, which no installation budget bounds, answers %s", got)
	}

	// The workspace's own budget, set and spent: now the own model pauses too.
	if _, err := admin.Exec(ctx,
		`UPDATE tenant SET settings = jsonb_set(settings, '{ai_own_tokens_per_day}', '100') WHERE id = $1`,
		resolverTenant.String()); err != nil {
		t.Fatal(err)
	}
	if got := answers(); got != "a model" {
		t.Errorf("the own model under its own budget answers %s", got)
	}
	inTenant(t, world.uow, resolverTenant, func(ctx context.Context) error {
		return guard.MeterAi(ctx, true, now, 100)
	})
	if got := answers(); got != "nothing" {
		t.Errorf("the own model past its own budget answers %s", got)
	}

	// Both rows, side by side, as the workspace reads them (UC-AI-07 check 4).
	inTenant(t, world.uow, resolverTenant, func(ctx context.Context) error {
		standings, err := quotaservice.Standings(ctx, store, store, env.TenancyMulti, now)
		if err != nil {
			return err
		}
		used := map[string]int64{}
		for _, standing := range standings {
			if standing.Used != nil {
				used[standing.Quota] = *standing.Used
			}
		}
		if used[quotaservice.AiTokensPerDay] != quotaservice.Defaults(env.TenancyMulti).AiTokensPerDay ||
			used[quotaservice.AiOwnTokensPerDay] != 100 {
			t.Errorf("the standings read %v", used)
		}
		return nil
	})

	// The next day in the workspace's zone renews both.
	budgeted.Clock = portclock.Fixed(now.Add(24 * time.Hour))
	if got := answers(); got != "a model" {
		t.Errorf("the own model on the workspace's next day answers %s", got)
	}
}
