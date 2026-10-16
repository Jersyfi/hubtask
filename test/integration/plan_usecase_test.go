// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The plan use cases through the real unit of work: each statement under the scope its use case
// opens - a plan written and counted under the system scope, a workspace moved in its own scope
// with the plan read and held there, the journal and the trail written beside it. The service tests'
// fake unit of work runs any callback, so only this proves the scopes (known traps).
func TestPlansAndAMoveRunUnderTheirOwnScopes(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	writer := adminservice.PlanWriter{
		Instance: adminservice.InstanceWriter{
			Settings: postgres.NewInstanceSettingRepository(), Journal: postgres.NewInstanceJournal(pageCursors()),
			UnitOfWork: uow, Clock: clockadapter.System{}, IDs: ids,
		},
		Plans: postgres.NewPlanRepository(), Tenancy: env.TenancyMulti,
	}
	registry, err := usecase.NewRegistry(nil,
		adminservice.CreatePlan{Writer: writer}.Descriptor(),
		adminservice.ChangePlan{Writer: writer}.Descriptor(),
		adminservice.MoveTenantPlan{
			Writer: writer, Tenants: postgres.NewAdminTenantRepository(),
			Quotas: postgres.NewQuotaRepository(), Audit: postgres.NewAuditSink(ids),
		}.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: freshID(t), AccountID: freshID(t), AccountName: "Olga Operator",
		Scopes: []string{"admin:tenants"},
	}

	name := planName("scoped")
	created, err := registry.Invoke(ctx, adminservice.CreatePlanName, operator, usecase.Input{
		"name": name, "display_name": "Family", "left_out": []any{"WEBHOOKS"},
	})
	if err != nil {
		t.Fatalf("creating under the system scope: %v", err)
	}
	if created["workspaces"] != int64(0) {
		t.Errorf("a new plan counts %v workspaces", created["workspaces"])
	}
	if _, err := registry.Invoke(ctx, adminservice.ChangePlanName, operator, usecase.Input{
		"name": name, "display_name": "Family plus", "expected_version": 1, "left_out": []any{"WEBHOOKS"},
	}); err != nil {
		t.Fatalf("changing under the system scope: %v", err)
	}

	tenant := freshID(t)
	if _, err := admin.Exec(ctx, `INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Scoped')`,
		tenant.String(), "sc-"+name[len(name)-9:]); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `UPDATE tenant SET plan_id = NULL, deleted_at = now() WHERE id = $1`, tenant.String())
	})
	moved, err := registry.Invoke(ctx, adminservice.MoveTenantPlanName, operator, usecase.Input{
		"tenant_id": tenant.String(), "plan": name,
	})
	if err != nil {
		t.Fatalf("moving in the workspace's own scope: %v", err)
	}
	if resting, _ := moved["features_resting"].([]any); len(resting) != 1 {
		t.Errorf("the plan, read in the workspace's scope, rests %v", moved["features_resting"])
	}

	// UC-INS-13 check 6: the move is in the workspace's own trail.
	var trail int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = $2`,
		tenant.String(), string(adminservice.TenantPlanChangedAction)).Scan(&trail); err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	if trail != 1 {
		t.Errorf("the workspace's trail holds %d plan changes, want 1", trail)
	}
	var journal int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM instance_event WHERE tenant_id = $1 AND action = 'tenant.plan_changed'`,
		tenant.String()).Scan(&journal); err != nil {
		t.Fatalf("reading the journal: %v", err)
	}
	if journal != 1 {
		t.Errorf("the journal holds %d plan changes for the workspace, want 1", journal)
	}
}

// Provisioning puts a seeded workspace on its plan at version 1: nothing in the seeding raises the
// row's version, which is what provisioning's guarded move relies on.
func TestASeededWorkspaceIsStillAtItsFirstVersion(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	tenants := postgres.NewAdminTenantRepository()
	tenant := freshID(t)
	if _, err := admin.Exec(ctx, `INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Fresh')`,
		tenant.String(), "fr-"+planName("x")[2:11]); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `UPDATE tenant SET deleted_at = now() WHERE id = $1`, tenant.String())
	})
	if err := uow.Within(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
		record, err := tenants.Find(ctx)
		if err == nil && record.Version != 1 {
			t.Errorf("a new workspace is at version %d", record.Version)
		}
		moved, err := tenants.SetPlan(ctx, "", 1, time.Now())
		if err == nil && !moved {
			t.Error("the guarded write at version 1 was refused")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
