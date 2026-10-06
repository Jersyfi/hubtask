// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// ADR-0078 §3 (SC-34) against the real database: an operator's opening of the password is written on
// one workspace's row and read back from it. Gate SG-3: both new repository methods are asked under
// the other workspace's scope and reach nothing of this one.
var (
	openingTenant    = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000034a1")
	openingBystander = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000034a2")
)

// Gate SG-3 for the two new statements: asked under one workspace's scope, they write that workspace's
// row and no other - an opening of the bystander leaves the tenant untouched, and a close under the
// bystander's scope does not end the tenant's opening. And a close at a moment before the end leaves
// an opening standing, which is what keeps the job of an earlier opening from ending a later one.
func TestTheOpeningsStatementsStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	_, _, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)
	for _, seed := range []struct {
		id   shared.ID
		slug string
	}{{openingTenant, "operator-opening"}, {openingBystander, "opening-bystander"}} {
		if _, err := admin.Exec(ctx, `
			INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, $2)
			ON CONFLICT (id) DO UPDATE SET status = 'ACTIVE', password_opened_until = NULL,
			  password_opened_requester = NULL, password_opened_reason = NULL`,
			seed.id.String(), seed.slug); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `
			UPDATE tenant SET password_opened_until = NULL, password_opened_requester = NULL,
			  password_opened_reason = NULL
			WHERE id IN ($1, $2)`, openingTenant.String(), openingBystander.String())
	})
	tenants := postgres.NewAdminTenantRepository()
	until := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	opening := domain.PasswordOpening{Until: until, Requester: "TICKET-4712", Reason: "down"}
	opened := func(tenant shared.ID) adminrepo.TenantRecord {
		t.Helper()
		var record adminrepo.TenantRecord
		inTenant(t, uow, tenant, func(ctx context.Context) error {
			found, err := tenants.Find(ctx)
			record = found
			return err
		})
		return record
	}

	inTenant(t, uow, openingTenant, func(ctx context.Context) error {
		moved, err := tenants.OpenPassword(ctx, opening, time.Now())
		if !moved {
			t.Error("the opening wrote nothing in its own workspace")
		}
		return err
	})
	if got := opened(openingTenant).PasswordOpening; !got.Until.Equal(until) || got.Requester != "TICKET-4712" {
		t.Errorf("the opening reads back as %+v", got)
	}
	if got := opened(openingBystander).PasswordOpening; !got.Until.IsZero() {
		t.Errorf("the opening reached the workspace beside it: %+v", got)
	}

	// A close under the bystander's scope ends nothing of the tenant's.
	inTenant(t, uow, openingBystander, func(ctx context.Context) error {
		closed, err := tenants.ClosePassword(ctx, time.Time{}, time.Now())
		if closed {
			t.Error("a close under the workspace beside it closed something")
		}
		return err
	})
	// A close at a moment before the end leaves it standing; one at the end ends it.
	inTenant(t, uow, openingTenant, func(ctx context.Context) error {
		closed, err := tenants.ClosePassword(ctx, until.Add(-time.Minute), time.Now())
		if closed {
			t.Error("a close before the end ended the opening")
		}
		return err
	})
	if opened(openingTenant).PasswordOpening.Until.IsZero() {
		t.Fatal("the opening went before its end")
	}
	inTenant(t, uow, openingTenant, func(ctx context.Context) error {
		closed, err := tenants.ClosePassword(ctx, until, time.Now())
		if !closed {
			t.Error("a close at the end left the opening")
		}
		return err
	})
	if got := opened(openingTenant).PasswordOpening; !got.Until.IsZero() || got.Requester != "" {
		t.Errorf("the closed opening reads back as %+v", got)
	}

	// A workspace that is leaving opens nothing.
	if _, err := admin.Exec(ctx, `UPDATE tenant SET status = 'PENDING_DELETION' WHERE id = $1`,
		openingBystander.String()); err != nil {
		t.Fatalf("marking the bystander as leaving: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `UPDATE tenant SET status = 'ACTIVE' WHERE id = $1`,
			openingBystander.String())
	})
	inTenant(t, uow, openingBystander, func(ctx context.Context) error {
		moved, err := tenants.OpenPassword(ctx, opening, time.Now())
		if moved {
			t.Error("a workspace pending deletion was opened")
		}
		return err
	})
}
