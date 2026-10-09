// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A legal hold stops a workspace's deletion (data-protection.md §5), against the real database: a
// workspace pending deletion under a hold keeps every row past its grace, and goes on the pass
// after the last hold is lifted; and a hold cannot be placed in a workspace the deletion request
// has moved, because the request read the holds under the shared lock placing has to wait for.

// heldWorkspace seeds a workspace of its own with a person and a hub, and answers both.
func heldWorkspace(ctx context.Context, t *testing.T) (tenant, person shared.ID) {
	t.Helper()
	tenant, person, hub := freshID(t), freshID(t), freshID(t)
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Under a hold')`,
			[]any{tenant.String(), "held-" + tenant.String()[24:]}},
		{`INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, 'Ines')`,
			[]any{person.String(), tenant.String()}},
		{`INSERT INTO container (id, tenant_id, type, name, order_key, created_by)
		  VALUES ($1, $2, 'HUB', 'Contracts', 'a0', $3)`,
			[]any{hub.String(), tenant.String(), person.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the workspace: %v\n%s", err, statement.sql)
		}
	}
	t.Cleanup(func() {
		done := context.WithoutCancel(ctx)
		for _, statement := range []string{
			`DELETE FROM container WHERE tenant_id = $1`,
			`DELETE FROM tenant WHERE id = $1`,
		} {
			if _, err := adminPool(done, t).Exec(done, statement, tenant.String()); err != nil {
				t.Errorf("clearing up: %v", err)
			}
		}
	})
	return tenant, person
}

func TestAWorkspaceUnderAHoldOutlivesItsGraceAndGoesAfterTheRelease(t *testing.T) {
	ctx := context.Background()
	tenant, person := heldWorkspace(ctx, t)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	tenants := postgres.NewAdminTenantRepository()
	admin := adminPool(ctx, t)

	hold := freshID(t)
	if _, err := admin.Exec(ctx, `
		INSERT INTO legal_hold (id, tenant_id, scope_kind, reason, placed_by, placed_at)
		VALUES ($1, $2, 'TENANT', 'Pending litigation', $3, now())`,
		hold.String(), tenant.String(), person.String()); err != nil {
		t.Fatalf("placing the hold: %v", err)
	}
	// Pending already, its grace over: an older binary's request during a rolling update, or a
	// workspace that was pending with a hold when the rule arrived.
	inTenant(t, uow, tenant, func(ctx context.Context) error {
		moved, err := tenants.RequestDeletion(ctx, time.Now().Add(-time.Hour).UTC(), time.Now().UTC())
		if err != nil || !moved {
			t.Fatalf("requesting the deletion (%v, %v)", moved, err)
		}
		return nil
	})

	deletion := adminservice.HardDeleteTenant{
		Tenants: tenants, Purge: postgres.NewTenantPurge(),
		Journal: postgres.NewInstanceJournal(pageCursors()), Store: &purgeStoreFake{},
		Holds:      postgres.NewLifecycleRepository(),
		UnitOfWork: uow, Clock: systemClock{}, IDs: generator{t},
	}
	held, err := deletion.Execute(ctx, tenant, freshID(t))
	if err != nil {
		t.Fatalf("the held pass: %v", err)
	}
	if held.Deleted || !held.Held || held.RunAgainIn != adminservice.HeldPurgeRetry {
		t.Errorf("the held pass answered %+v", held)
	}
	for _, table := range []string{"account", "container", "legal_hold"} {
		if n := rowsOfTenant(ctx, t, table, tenant); n != 1 {
			t.Errorf("%s holds %d rows of the held workspace, want 1", table, n)
		}
	}

	if _, err := admin.Exec(ctx, `
		UPDATE legal_hold SET released_at = now(), released_by = $2, released_reason = 'Settled'
		WHERE id = $1`, hold.String(), person.String()); err != nil {
		t.Fatalf("releasing the hold: %v", err)
	}
	released, err := deletion.Execute(ctx, tenant, freshID(t))
	if err != nil || !released.Deleted {
		t.Fatalf("the pass after the release answered (%+v, %v)", released, err)
	}
	var left int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM tenant WHERE id = $1`, tenant.String()).
		Scan(&left); err != nil || left != 0 {
		t.Errorf("the workspace is still there (%d, %v)", left, err)
	}
}

// The deletion request reads the holds under the shared lock and moves the workspace in the same
// transaction; a placement admitted a moment before waits for it, then finds the workspace leaving.
func TestAHoldPlacedWhileTheDeletionIsRequestedFindsTheWorkspaceLeaving(t *testing.T) {
	ctx := context.Background()
	tenant, _ := heldWorkspace(ctx, t)
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	tenants := postgres.NewAdminTenantRepository()

	leaving := make(chan bool, 1)
	failed := make(chan error, 1)
	var waited bool
	inTenant(t, uow, tenant, func(ctx context.Context) error {
		if _, err := lifecycleRepo().Active(ctx); err != nil {
			return err
		}
		if moved, err := tenants.RequestDeletion(ctx, time.Now().Add(30*24*time.Hour).UTC(), time.Now().UTC()); err != nil || !moved {
			t.Fatalf("requesting the deletion (%v, %v)", moved, err)
		}
		//nolint:contextcheck // a second transaction: the context handed in carries this one
		go func() {
			err := write(context.Background(), t, tenant, func(ctx context.Context) error {
				if err := holdRepo().Lock(ctx); err != nil {
					return err
				}
				answer, err := holdRepo().WorkspaceLeaving(ctx)
				leaving <- answer
				return err
			})
			if err != nil {
				failed <- err
			}
		}()
		select {
		case <-leaving:
			t.Error("the placement read the status while the deletion request held the shared lock")
		case <-time.After(500 * time.Millisecond):
			waited = true
		}
		return nil
	})
	if !waited {
		return
	}
	select {
	case answer := <-leaving:
		if !answer {
			t.Error("the placement found the workspace staying after the deletion request committed")
		}
	case err := <-failed:
		t.Fatalf("the placement's transaction: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the placement never ran after the deletion request committed")
	}
}
