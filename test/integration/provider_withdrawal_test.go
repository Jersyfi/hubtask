// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// ADR-0076 (SC-20) with ADR-0077 §1 (SC-26): an offered provider answers the number of workspaces that
// switched it on - counted from their own switches where the installation reads it, so it is true
// after every write to them, and a workspace reads none - and the date its offer ends. Gate SG-3:
// only the installation's scope sets a withdrawal.

var withdrawnRow = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd31")

func TestAnOfferedProvidersCountIsCountedAndItsWithdrawalIsTheInstallations(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)
	providers := postgres.NewIdentityProviderRepository()
	app := appPool(ctx, t)
	uow := postgres.NewUnitOfWork(app)
	admin := adminPool(ctx, t)
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DELETE FROM identity_provider WHERE id = $1`, withdrawnRow.String())
	})
	now := time.Now().UTC()

	offered, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		ID: withdrawnRow, Issuer: "https://id.withdrawal.example", ClientID: "hubtask-platform",
		Provisioning: string(domain.ProvisionInvitedOnly), Enabled: true, Now: now,
	})
	if err != nil {
		t.Fatalf("building the offer: %v", err)
	}
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		_, err := providers.Insert(ctx, offered, cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte("s")})
		return err
	}); err != nil {
		t.Fatalf("offering it: %v", err)
	}

	// The count is counted where it is read (ADR-0077 §1, SC-26): from the workspaces' own switches,
	// so every write that changes them - a switch, a deletion for good, a restore or an import that
	// writes the settings whole - leaves it true. Two workspaces of this test's own.
	countA := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000026a1")
	countB := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000026a2")
	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone)
		VALUES ($1, 'count-a', 'Count A', 'en', 'UTC'), ($2, 'count-b', 'Count B', 'en', 'UTC')
		ON CONFLICT (id) DO UPDATE SET status = 'ACTIVE', purge_after = NULL, deleted_at = NULL`, countA.String(), countB.String()); err != nil {
		t.Fatalf("seeding the workspaces: %v", err)
	}
	switchOn := func(tenant shared.ID, on bool) {
		t.Helper()
		taken := "[]"
		if on {
			taken = `["` + withdrawnRow.String() + `"]`
		}
		if _, err := admin.Exec(ctx, `
			UPDATE tenant SET settings = jsonb_set(coalesce(settings, '{}'::jsonb), '{offered_providers}', $2::jsonb)
			WHERE id = $1`, tenant.String(), taken); err != nil {
			t.Fatalf("switching: %v", err)
		}
	}
	count := func(scope persistence.Scope) int {
		t.Helper()
		var counted int
		if err := uow.Within(ctx, scope, func(ctx context.Context) error {
			found, err := providers.Find(ctx, withdrawnRow)
			counted = found.OfferedWorkspaces
			return err
		}); err != nil {
			t.Fatalf("reading the count: %v", err)
		}
		return counted
	}

	switchOn(countA, true)
	switchOn(countB, true)
	if got := count(persistence.SystemScope()); got != 2 {
		t.Errorf("two workspaces switched it on and the count is %d", got)
	}
	// A workspace reads no count at all: it learns nothing of the others (P-01) - not through the
	// provider statements, and not by calling the function itself, which counts only outside one.
	if got := count(persistence.Scope{TenantID: countA}); got != 0 {
		t.Errorf("a workspace reads a count of %d", got)
	}
	inRawTenant(ctx, t, app, countA.String(), false, func(tx pgx.Tx) {
		var direct int
		if err := tx.QueryRow(ctx, `SELECT count_provider_offers($1)`, withdrawnRow.String()).Scan(&direct); err != nil {
			t.Errorf("calling the function from a workspace: %v", err)
		} else if direct != 0 {
			t.Errorf("a workspace calling the function directly counts %d", direct)
		}
	})
	// Waiting to be deleted, and still restorable within its grace (H-06): it still uses it.
	if _, err := admin.Exec(ctx, `
		UPDATE tenant SET status = 'PENDING_DELETION', purge_after = now() + interval '30 days'
		WHERE id = $1`, countB.String()); err != nil {
		t.Fatalf("requesting the deletion: %v", err)
	}
	if got := count(persistence.SystemScope()); got != 2 {
		t.Errorf("a workspace pending deletion is not counted: %d, want 2", got)
	}
	// Deleted for good - the row removed, as the hard delete does (HardDeleteTenant): it no longer
	// uses anything.
	if _, err := admin.Exec(ctx, `DELETE FROM tenant WHERE id = $1`, countB.String()); err != nil {
		t.Fatalf("deleting for good: %v", err)
	}
	if got := count(persistence.SystemScope()); got != 1 {
		t.Errorf("after a deletion for good the count is %d, want 1", got)
	}
	// Restored or imported with settings that do not take it: the number follows.
	switchOn(countA, false)
	if got := count(persistence.SystemScope()); got != 0 {
		t.Errorf("after the settings were written whole the count is %d, want 0", got)
	}

	// The withdrawal is the installation's: a workspace's scope sets nothing.
	at := now.Add(14 * 24 * time.Hour).Truncate(time.Second)
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error {
		if _, found, err := providers.SetWithdrawal(ctx, withdrawnRow, at, now); err != nil || found {
			t.Errorf("a workspace announced the installation's withdrawal: (%v, %v)", found, err)
		}
		return nil
	})
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		withdrawn, found, err := providers.SetWithdrawal(ctx, withdrawnRow, at, now)
		if err != nil || !found {
			t.Fatalf("announcing the withdrawal: (%v, %v)", found, err)
		}
		if !withdrawn.WithdrawAt.Equal(at) {
			t.Errorf("the withdrawal reads %v, want %v", withdrawn.WithdrawAt, at)
		}
		return nil
	}); err != nil {
		t.Fatalf("installation scope: %v", err)
	}
	// Every workspace reads the date.
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error {
		found, err := providers.Find(ctx, withdrawnRow)
		if err != nil || !found.WithdrawAt.Equal(at) {
			t.Errorf("a workspace reads the withdrawal as %v (%v)", found.WithdrawAt, err)
		}
		return nil
	})
	// Cancelled: offered again without an end.
	if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		kept, found, err := providers.SetWithdrawal(ctx, withdrawnRow, time.Time{}, now)
		if err != nil || !found || !kept.WithdrawAt.IsZero() || !kept.Enabled {
			t.Errorf("cancelling answered (%+v, %v, %v)", kept, found, err)
		}
		return nil
	}); err != nil {
		t.Fatalf("installation scope: %v", err)
	}

	// ADR-0077 §2 (SC-27): the statement that deletes asks itself whether the offer still stands and
	// a workspace uses it, whatever the caller looked at before. Offered and used, it stays.
	switchOn(countA, true)
	remove := func() bool {
		t.Helper()
		var removed bool
		if err := uow.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
			var err error
			removed, err = providers.Delete(ctx, withdrawnRow, time.Now().UTC())
			return err
		}); err != nil {
			t.Fatalf("removing: %v", err)
		}
		return removed
	}
	if remove() {
		t.Fatal("an offered provider a workspace uses was removed")
	}
	// Its day reached, it goes - the workspace's switch is still on, and no longer read.
	if _, err := admin.Exec(ctx, `UPDATE identity_provider SET withdraw_at = now() - interval '1 minute' WHERE id = $1`,
		withdrawnRow.String()); err != nil {
		t.Fatalf("reaching the day: %v", err)
	}
	if !remove() {
		t.Error("a provider whose offer has ended was not removed")
	}
}
