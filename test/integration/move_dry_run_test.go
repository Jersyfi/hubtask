// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A move's dry run against the database: it answers the plan the workspace would be on, and the
// workspace's row, its version, the journal, its trail and the platform's feed are as they were -
// the transaction it ran in did not commit (UC-INS-13 check 7, multi-tenancy.md §4.3). The move
// after it writes each once, so the counts read the tables the move writes.
func TestAMovesDryRunWritesNothing(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	plans := postgres.NewPlanRepository()
	tenants := postgres.NewAdminTenantRepository()
	family := newPlan(t, planName("dry"))
	inSystem(ctx, t, uow, func(ctx context.Context) error { return plans.Insert(ctx, family, "") })

	tenant := freshID(t)
	if _, err := admin.Exec(ctx, `INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Dry')`,
		tenant.String(), "dry-"+family.Name[len(family.Name)-9:]); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `UPDATE tenant SET plan_id = NULL, deleted_at = now() WHERE id = $1`, tenant.String())
	})
	read := func() (string, int) {
		t.Helper()
		var plan string
		var version int
		if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
			record, err := tenants.Find(ctx)
			plan, version = record.PlanName, record.Version
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return plan, version
	}
	journalled := func() int {
		t.Helper()
		var count int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM instance_event WHERE tenant_id = $1`, tenant.String()).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	trailed := func() int {
		t.Helper()
		var count int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1`, tenant.String()).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	_, before := read()
	entries, trail, feed := journalled(), trailed(), lastPlatformNumber(ctx, t)

	move := adminservice.MoveTenantPlan{
		Writer: adminservice.PlanWriter{
			Instance: adminservice.InstanceWriter{
				Settings: postgres.NewInstanceSettingRepository(), Journal: postgres.NewInstanceJournal(pageCursors()),
				UnitOfWork: uow, Clock: clockadapter.System{}, IDs: clockadapter.NewUUIDv7(clockadapter.System{}), Tenancy: env.TenancyMulti,
			},
			Plans: plans, Tenancy: env.TenancyMulti,
		},
		Tenants: tenants, Quotas: postgres.NewQuotaRepository(), Usage: postgres.NewQuotaRepository(),
		Audit:    postgres.NewAuditSink(clockadapter.NewUUIDv7(clockadapter.System{})),
		Platform: platformRecorder(clockadapter.NewUUIDv7(clockadapter.System{})),
	}
	operator := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantA, AccountID: freshID(t), Scopes: []string{"admin:tenants"}}
	moved, err := move.Execute(ctx, operator, adminservice.MoveTenantPlanCommand{
		TenantID: tenant, Plan: family.Name, ExpectedVersion: &before, DryRun: true,
	})
	if err != nil {
		t.Fatalf("the dry run: %v", err)
	}
	if moved.Tenant.PlanName != family.Name {
		t.Errorf("the dry run answered the workspace on %q", moved.Tenant.PlanName)
	}
	if plan, version := read(); plan != "" || version != before {
		t.Errorf("after the dry run: plan %q, version %d (was none, %d)", plan, version, before)
	}
	if journalled() != entries || trailed() != trail {
		t.Errorf("the dry run wrote %d journal and %d trail entries", journalled()-entries, trailed()-trail)
	}
	if events := platformEventsOf(ctx, t, uow, feed, tenant); len(events) != 0 {
		t.Errorf("the dry run told the platform %d events", len(events))
	}

	// The same move for real, on the version read before the dry run: it moves, once.
	if _, err := move.Execute(ctx, operator, adminservice.MoveTenantPlanCommand{
		TenantID: tenant, Plan: family.Name, ExpectedVersion: &before,
	}); err != nil {
		t.Fatalf("the move: %v", err)
	}
	if plan, version := read(); plan != family.Name || version != before+1 {
		t.Errorf("after the move: plan %q, version %d", plan, version)
	}
	if journalled() != entries+1 || trailed() != trail+1 {
		t.Errorf("the move wrote %d journal and %d trail entries, want one each", journalled()-entries, trailed()-trail)
	}
	if events := platformEventsOf(ctx, t, uow, feed, tenant); len(events) != 1 {
		t.Errorf("the move told the platform %d events, want one", len(events))
	}
}
