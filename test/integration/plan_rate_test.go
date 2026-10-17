// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// UC-INS-10 check 1 for the request rate (multi-tenancy.md §4, §4.2): the per-token ceiling a
// credential carries resolves as every limit does - the operator's exception, then the workspace's
// plan unless the installation locks the rate, then the installation's own default - read in the
// authentication query that already runs.
func TestATokensRateResolvesThroughThePlanAndTheInstallation(t *testing.T) {
	ctx := context.Background()
	admin := adminPool(ctx, t)
	repo, uow := tokenRepository(ctx, t)
	hasher := security.NewTokenHasher(secret.New(installationSecret))

	workspace := seedWorkspace(ctx, t, "rated")
	account, tokenID := freshID(t), freshID(t)
	token := mintFor(t, workspace, 7)
	if _, err := admin.Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, display_name, status) VALUES ($1, $2, 'SERVICE_ACCOUNT', 'Rated', 'ACTIVE')`,
		account.String(), workspace.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO access_token (id, tenant_id, account_id, name, token_hash, token_prefix, scopes, expires_at)
		VALUES ($1, $2, $3, 'rated', $4, 'hbt_pat_', ARRAY['items:read'], now() + interval '1 day')`,
		tokenID.String(), workspace.String(), account.String(), hasher.Hash(token.Secret())); err != nil {
		t.Fatal(err)
	}
	family := newPlan(t, planName("rate"))
	family.Values.Quotas = identity.QuotaDefaults{
		Limits: map[string]int64{quotaservice.APIRequestsPerMinute: 120}, Locks: map[string]bool{},
	}
	inSystem(ctx, t, uow, func(ctx context.Context) error { return postgres.NewPlanRepository().Insert(ctx, family, "") })
	const key = "quota." + quotaservice.APIRequestsPerMinute
	t.Cleanup(func() {
		_, _ = admin.Exec(context.WithoutCancel(ctx), `DELETE FROM instance_setting WHERE key = $1`, key)
	})

	rate := func() int64 {
		t.Helper()
		var credential repository.Credential
		if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: workspace}, func(ctx context.Context) error {
			var err error
			credential, err = repo.FindByToken(ctx, token)
			return err
		}); err != nil {
			t.Fatalf("reading the token: %v", err)
		}
		return credential.TokenRatePerMinute
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}

	if got := rate(); got != 0 {
		t.Fatalf("nothing set anywhere answers a rate of %d", got)
	}
	exec(`INSERT INTO instance_setting (key, value, lock_origin, updated_at) VALUES ($1, '300', 'OPEN', now())
		ON CONFLICT (key) DO UPDATE SET value = '300', lock_origin = 'OPEN'`, key)
	if got := rate(); got != 300 {
		t.Errorf("the installation's default rate reads %d, want 300", got)
	}
	exec(`UPDATE tenant SET plan_id = $1 WHERE id = $2`, family.ID.String(), workspace.String())
	if got := rate(); got != 120 {
		t.Errorf("the plan's rate reads %d, want 120", got)
	}
	exec(`UPDATE instance_setting SET lock_origin = 'INSTANCE' WHERE key = $1`, key)
	if got := rate(); got != 300 {
		t.Errorf("under the installation's lock the rate reads %d, want the installation's 300", got)
	}
	exec(`UPDATE tenant SET settings = jsonb_set(coalesce(settings, '{}'::jsonb), '{quotas}', '{"api_requests_per_minute": 50}') WHERE id = $1`,
		workspace.String())
	if got := rate(); got != 50 {
		t.Errorf("the operator's exception reads %d, want 50", got)
	}
}
