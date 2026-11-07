// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The application role as the migrations make it raises nothing; a backfill's policy left behind
// on a tenant table is named until it is gone, and the owner connecting is named as what it is.
func TestTheReportNamesARoleOrPolicyPastRowLevelSecurity(t *testing.T) {
	ctx := context.Background()
	app := postgres.RoleIsolation{UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t))}
	if got := app.Warnings(ctx); len(got) != 0 {
		t.Fatalf("the application role raised %v", got)
	}

	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx, `CREATE POLICY probe_backfill ON work_item TO CURRENT_USER USING (true)`); err != nil {
		t.Fatalf("leaving a policy behind: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), `DROP POLICY IF EXISTS probe_backfill ON work_item`) })
	got := app.Warnings(ctx)
	if len(got) != 1 || got[0].Code != postgres.IsolationBypassedCode || got[0].Params["cause"] != "backfill_policy" {
		t.Errorf("with a leftover policy: %v", got)
	}

	owner := postgres.RoleIsolation{UnitOfWork: postgres.NewUnitOfWork(admin)}
	if got := owner.Warnings(ctx); len(got) < 2 {
		t.Errorf("the migrating role raised %v, want its own causes beside the policy", got)
	}
}
