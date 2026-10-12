// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	automationservice "github.com/Jersyfi/hubtask/core/application/service/automation"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	recurrenceadapter "github.com/Jersyfi/hubtask/infrastructure/recurrence"
)

// wakes records the poller the cancel seeds, without a queue table to read it from.
type wakes struct{ queued []queue.Request }

func (w *wakes) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	w.queued = append(w.queued, request)
	return shared.MustParseID("01936f2a-7c1e-7000-8000-0000000115f1"), nil
}

// UC-INS-08 check 5, the acceptance: an operator cancels a pending deletion and finds the workspace
// intact - its status as before, every row and byte still there, the rules the request switched off
// on again, the one switched off by hand still off - and the grace job's pass that follows removes
// nothing. Again with a legal hold keeping the workspace pending past its deadline: the cancel is
// the way out, and the hold stays.
func TestACancelledDeletionLeavesTheWorkspaceIntact(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)

	cases := []struct {
		name     string
		deadline time.Duration
		held     bool
	}{
		{"during the grace", 30 * 24 * time.Hour, false},
		{"past the deadline under a legal hold", -48 * time.Hour, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tenant, rules := cancellableWorkspace(ctx, t, identity.TenantActive)
			if c.held {
				placeTenantHold(ctx, t, tenant)
			}
			footprint := func() adminrepo.Footprint {
				var counted adminrepo.Footprint
				within(ctx, t, uow, tenant, func(ctx context.Context) error {
					var err error
					counted, err = postgres.NewTenantPurge().Footprint(ctx)
					return err
				})
				counted.AuditEntries = 0 // the cancel writes its own entry; the trail is not the subject
				return counted
			}
			before := footprint()
			requestDeletion(ctx, t, uow, tenant, time.Now().Add(c.deadline).UTC())

			woken := &wakes{}
			cancel := adminservice.CancelTenantDeletion{
				Tenants: postgres.NewAdminTenantRepository(), Journal: postgres.NewInstanceJournal(pageCursors()),
				Automations: automationservice.PausedRules{
					Paused: postgres.NewPausedAutomationRules(), Expander: recurrenceadapter.New(), Jobs: woken,
				},
				Audit: postgres.NewAuditSink(generator{t}), UnitOfWork: uow,
				Clock: systemClock{}, IDs: generator{t},
			}
			operator := appshared.ActorContext{
				Kind: appshared.ActorUser, TenantID: freshID(t), AccountID: freshID(t),
				AccountName: "Root Operator", Scopes: []string{"admin:tenants"},
			}
			status, err := cancel.Execute(ctx, operator, tenant)
			if err != nil || status != identity.TenantActive {
				t.Fatalf("the cancel answered (%s, %v)", status, err)
			}

			var current string
			var purgeAfter *time.Time
			if err := admin.QueryRow(ctx, `SELECT status, purge_after FROM tenant WHERE id = $1`,
				tenant.String()).Scan(&current, &purgeAfter); err != nil || current != "ACTIVE" || purgeAfter != nil {
				t.Errorf("the workspace reads %s, deadline %v (%v)", current, purgeAfter, err)
			}
			for i, want := range []bool{true, true, false} {
				var enabled bool
				var next *time.Time
				if err := admin.QueryRow(ctx, `SELECT enabled, next_run_at FROM automation_rule WHERE id = $1`,
					rules[i].String()).Scan(&enabled, &next); err != nil || enabled != want {
					t.Errorf("rule %d reads enabled %v, want %v (%v)", i, enabled, want, err)
				}
				if i == 1 && (next == nil || !next.After(time.Now())) {
					t.Errorf("the schedule's next moment is %v, want one from now on", next)
				}
			}
			if len(woken.queued) != 1 || woken.queued[0].Kind != queue.KindAutomationSchedule {
				t.Errorf("the poller was woken with %+v", woken.queued)
			}
			var journal, trail int
			if err := admin.QueryRow(ctx, `SELECT count(*) FROM instance_event WHERE tenant_id = $1
				AND action = 'tenant.deletion_cancelled'`, tenant.String()).Scan(&journal); err != nil || journal != 1 {
				t.Errorf("the journal holds %d cancels (%v)", journal, err)
			}
			if err := admin.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE tenant_id = $1
				AND action = 'tenant.deletion_cancelled'`, tenant.String()).Scan(&trail); err != nil || trail != 1 {
				t.Errorf("the trail holds %d cancels (%v)", trail, err)
			}

			// The grace job's pass, due or not: it finds the workspace staying and removes nothing.
			store := &purgeStoreFake{}
			pass := adminservice.HardDeleteTenant{
				Tenants: postgres.NewAdminTenantRepository(), Purge: postgres.NewTenantPurge(),
				Journal: postgres.NewInstanceJournal(pageCursors()), Store: store,
				Holds:      postgres.NewLifecycleRepository(),
				UnitOfWork: uow, Clock: systemClock{}, IDs: generator{t},
			}
			outcome, err := pass.Execute(ctx, tenant, freshID(t))
			if err != nil || outcome.Deleted || outcome.RunAgainIn != 0 {
				t.Errorf("the pass after the cancel answered (%+v, %v)", outcome, err)
			}
			if len(store.deleted) != 0 {
				t.Errorf("the pass deleted bytes: %v", store.deleted)
			}
			if after := footprint(); after != before {
				t.Errorf("the workspace counts %+v after, %+v before", after, before)
			}
			if c.held {
				if n := rowsOfTenant(ctx, t, "legal_hold", tenant); n != 1 {
					t.Errorf("the workspace holds %d holds after the cancel, want 1", n)
				}
			}
		})
	}
}

// placeTenantHold places a hold on the whole workspace in the name of its person, and lifts it
// with the workspace's clean-up.
func placeTenantHold(ctx context.Context, t *testing.T, tenant shared.ID) {
	t.Helper()
	admin := adminPool(ctx, t)
	var person string
	if err := admin.QueryRow(ctx, `SELECT id FROM account WHERE tenant_id = $1`,
		tenant.String()).Scan(&person); err != nil {
		t.Fatalf("reading the person: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO legal_hold (id, tenant_id, scope_kind, reason, placed_by, placed_at)
		VALUES ($1, $2, 'TENANT', 'Pending litigation', $3, now())`,
		freshID(t).String(), tenant.String(), person); err != nil {
		t.Fatalf("placing the hold: %v", err)
	}
	t.Cleanup(func() {
		done := context.WithoutCancel(ctx)
		if _, err := adminPool(done, t).Exec(done,
			`DELETE FROM legal_hold WHERE tenant_id = $1`, tenant.String()); err != nil {
			t.Errorf("clearing the hold: %v", err)
		}
	})
}
