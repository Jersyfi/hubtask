// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-INS-10 check 5 (multi-tenancy.md §4, §4.2): the people a workspace holds - active, restricted
// and managed accounts and open invitations, never service accounts, disabled or anonymised ones, or
// an invitation whose link ran out - counted in its own scope; two creations at the last place
// admit one; a limit lowered below the count disables nobody.
func TestThePeopleLimitCountsAndAdmitsOneAtTheLastPlace(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	admin := adminPool(ctx, t)
	store := postgres.NewQuotaRepository()
	accounts := postgres.NewAccountRepository()
	guard := quotaservice.Guard{Store: store, Usage: store, Meter: store, Tenancy: env.TenancyMulti}

	workspace, other := seedWorkspace(ctx, t, "people"), seedWorkspace(ctx, t, "neighbours")
	person := func(tenant shared.ID, kind, status string, expires string) {
		t.Helper()
		if _, err := admin.Exec(ctx, fmt.Sprintf(`
			INSERT INTO account (id, tenant_id, kind, display_name, status, redemption_expires_at)
			VALUES ($1, $2, '%s', 'P', '%s', %s)`, kind, status, expires), freshID(t).String(), tenant.String()); err != nil {
			t.Fatal(err)
		}
	}
	person(workspace, "USER", "ACTIVE", "NULL")
	person(workspace, "USER", "RESTRICTED", "NULL")
	person(workspace, "USER", "INVITED", "now() + interval '1 day'")
	person(workspace, "USER", "INVITED", "now() - interval '1 day'")
	person(workspace, "USER", "DISABLED", "NULL")
	person(workspace, "USER", "ANONYMIZED", "NULL")
	person(workspace, "SERVICE_ACCOUNT", "ACTIVE", "NULL")
	person(other, "USER", "ACTIVE", "NULL")
	person(other, "USER", "ACTIVE", "NULL")

	counted := func(tenant shared.ID) int64 {
		t.Helper()
		var people int64
		if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenant}, func(ctx context.Context) error {
			var err error
			people, err = store.Accounts(ctx)
			return err
		}); err != nil {
			t.Fatalf("counting: %v", err)
		}
		return people
	}
	if got := counted(workspace); got != 3 {
		t.Errorf("the workspace holds %d people, want 3 (active, restricted, one open invitation)", got)
	}
	if got := counted(other); got != 2 {
		t.Errorf("the neighbouring workspace counts %d, want its own 2", got)
	}

	// A limit at the next place: the first creation holds the lock until it commits, so the second
	// counts the first and is refused.
	if _, err := admin.Exec(ctx, `UPDATE tenant SET settings = jsonb_set(coalesce(settings, '{}'::jsonb), '{quotas}', '{"accounts": 4}') WHERE id = $1`,
		workspace.String()); err != nil {
		t.Fatal(err)
	}
	invite := func(ctx context.Context, n int) error {
		if err := guard.Accounts(ctx, workspace.String(), 1); err != nil {
			return err
		}
		invited, err := identity.Invite(freshID(t), workspace, fmt.Sprintf("last%d-%d@example.org", n, time.Now().UnixNano()), "Last", nil, nil)
		if err != nil {
			return err
		}
		return accounts.Insert(ctx, invited)
	}
	firstCounted, release := make(chan struct{}), make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- uow.Within(ctx, persistence.Scope{TenantID: workspace}, func(ctx context.Context) error {
			err := invite(ctx, 1)
			close(firstCounted)
			if err != nil {
				return err
			}
			<-release
			return nil
		})
	}()
	<-firstCounted
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- uow.Within(ctx, persistence.Scope{TenantID: workspace}, func(ctx context.Context) error {
			return invite(ctx, 2)
		})
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("the second creation finished while the first held the last place: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("the first creation at the last place was refused: %v", err)
	}
	if got := shared.AsError(<-secondDone); got == nil || got.DetailCode != "capacity.accounts" {
		t.Errorf("the second creation at the last place answered %v, want capacity.accounts", got)
	}

	// A renewal of an invitation whose link ran out takes a place again and is refused at the limit,
	// asked before the new link is written - in the mail mode the job writes it later. One still
	// open already counts and is renewed.
	if _, err := admin.Exec(ctx, `UPDATE tenant SET settings = jsonb_set(settings, '{quotas}', '{"accounts": 4}') WHERE id = $1`,
		workspace.String()); err != nil {
		t.Fatal(err)
	}
	var lapsed, open string
	if err := admin.QueryRow(ctx, `SELECT id FROM account WHERE tenant_id = $1 AND status = 'INVITED'
		AND redemption_expires_at < now()`, workspace.String()).Scan(&lapsed); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, `SELECT id FROM account WHERE tenant_id = $1 AND status = 'INVITED'
		AND redemption_expires_at > now()`, workspace.String()).Scan(&open); err != nil {
		t.Fatal(err)
	}
	renewing := func(account string) error {
		t.Helper()
		var refusal error
		if err := uow.Within(ctx, persistence.Scope{TenantID: workspace}, func(ctx context.Context) error {
			refusal = guard.Renewal(ctx, workspace.String(), account)
			return nil
		}); err != nil {
			t.Fatalf("the renewal's transaction: %v", err)
		}
		return refusal
	}
	if got := shared.AsError(renewing(lapsed)); got == nil || got.DetailCode != "capacity.accounts" {
		t.Errorf("renewing a lapsed invitation at the limit answered %v, want capacity.accounts", got)
	}
	if err := renewing(open); err != nil {
		t.Errorf("renewing an open invitation at the limit answered %v", err)
	}

	// Lowered below the count: nobody is disabled, removed or hidden.
	if _, err := admin.Exec(ctx, `UPDATE tenant SET settings = jsonb_set(settings, '{quotas}', '{"accounts": 1}') WHERE id = $1`,
		workspace.String()); err != nil {
		t.Fatal(err)
	}
	if got := counted(workspace); got != 4 {
		t.Errorf("after the limit was lowered the workspace holds %d people, want all 4", got)
	}
}
