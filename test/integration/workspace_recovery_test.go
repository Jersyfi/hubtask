// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	automationservice "github.com/Jersyfi/hubtask/core/application/service/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	recurrenceadapter "github.com/Jersyfi/hubtask/infrastructure/recurrence"
)

// UC-INS-03's way back, the workspace half, against the real database and as the app role: the
// machine finds a locked workspace by its short name, calls its pending deletion off and resumes
// it, and the trail and the journal hold both acts as the machine's (UC-INS-08 checks 3, 5, 6).
// Again past the deadline under a legal hold, which keeps the workspace pending and stays.
func TestTheMachineDoorResumesAndCancels(t *testing.T) {
	ctx := context.Background()
	for _, held := range []bool{false, true} {
		name := "during the grace"
		if held {
			name = "past the deadline under a legal hold"
		}
		t.Run(name, func(t *testing.T) { machineDoor(ctx, t, held) })
	}
}

func machineDoor(ctx context.Context, t *testing.T, held bool) {
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)

	tenant, _ := cancellableWorkspace(ctx, t, identity.TenantSuspended)
	deadline := time.Hour
	if held {
		placeTenantHold(ctx, t, tenant)
		deadline = -48 * time.Hour
	}
	requestDeletion(ctx, t, uow, tenant, time.Now().Add(deadline).UTC())
	var slug string
	if err := admin.QueryRow(ctx, `SELECT slug FROM tenant WHERE id = $1`, tenant.String()).Scan(&slug); err != nil {
		t.Fatalf("reading the short name: %v", err)
	}

	tenants := postgres.NewAdminTenantRepository()
	journal := postgres.NewInstanceJournal(pageCursors())
	sink := postgres.NewAuditSink(generator{t})
	machine := adminservice.RecoverWorkspace{
		Tenants: tenants, UnitOfWork: uow,
		Resume: adminservice.ResumeTenant{LifecycleShift: adminservice.LifecycleShift{
			Tenants: tenants, Journal: journal, Audit: sink, UnitOfWork: uow,
			Clock: systemClock{}, IDs: generator{t},
		}},
		Cancel: adminservice.CancelTenantDeletion{
			Tenants: tenants, Journal: journal, Audit: sink, UnitOfWork: uow,
			Automations: automationservice.PausedRules{
				Paused: postgres.NewPausedAutomationRules(), Expander: recurrenceadapter.New(), Jobs: &wakes{},
			},
			Clock: systemClock{}, IDs: generator{t},
		},
	}

	// Resuming first is refused: the deletion is called off before anything else.
	if err := machine.ResumeBySlug(ctx, slug); err == nil {
		t.Fatal("a pending workspace was resumed")
	}
	status, err := machine.CancelDeletionBySlug(ctx, strings.ToUpper(slug))
	if err != nil || status != identity.TenantSuspended {
		t.Fatalf("the cancel answered (%s, %v), want SUSPENDED", status, err)
	}
	if err := machine.ResumeBySlug(ctx, slug); err != nil {
		t.Fatalf("resuming: %v", err)
	}

	var current string
	var purgeAfter *time.Time
	if err := admin.QueryRow(ctx, `SELECT status, purge_after FROM tenant WHERE id = $1`,
		tenant.String()).Scan(&current, &purgeAfter); err != nil || current != "ACTIVE" || purgeAfter != nil {
		t.Errorf("the workspace reads %s, deadline %v (%v)", current, purgeAfter, err)
	}
	for _, action := range []string{"tenant.deletion_cancelled", "tenant.resumed"} {
		var trail, journalled int
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1 AND action = $2
			AND actor_type = 'SYSTEM' AND actor_id IS NULL AND actor_label = 'machine'`,
			tenant.String(), action).Scan(&trail); err != nil || trail != 1 {
			t.Errorf("the trail holds %d %s by the machine (%v)", trail, action, err)
		}
		if err := admin.QueryRow(ctx, `SELECT count(*) FROM instance_event WHERE tenant_id = $1
			AND action = $2 AND actor_label = 'machine' AND tenant_slug = $3`,
			tenant.String(), action, slug).Scan(&journalled); err != nil || journalled != 1 {
			t.Errorf("the journal holds %d %s by the machine (%v)", journalled, action, err)
		}
	}
	if held {
		if n := rowsOfTenant(ctx, t, "legal_hold", tenant); n != 1 {
			t.Errorf("the workspace holds %d holds after the cancel, want 1", n)
		}
	}
}
