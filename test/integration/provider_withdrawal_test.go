// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// ADR-0076 (SC-20): an offered provider carries the count of workspaces that switched it on - moved
// by a workspace's own switch, inside that workspace's transaction, and by nothing else - and the
// date its offer ends. Gate SG-3: a workspace moves the count of an installation's row and of
// nothing else, and only the installation's scope sets a withdrawal.

var withdrawnRow = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd31")

func TestAnOfferedProvidersCountMovesWithTheSwitchAndItsWithdrawalIsTheInstallations(t *testing.T) {
	ctx := context.Background()
	seedIdentityProviderTenants(ctx, t)
	providers := postgres.NewIdentityProviderRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
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

	count := func() int {
		t.Helper()
		var counted int
		if err := admin.QueryRow(ctx, `SELECT offered_workspaces FROM identity_provider WHERE id = $1`,
			withdrawnRow.String()).Scan(&counted); err != nil {
			t.Fatalf("reading the count: %v", err)
		}
		return counted
	}

	// Two workspaces switch it on, one switches it off again: the count follows, from inside each
	// workspace's own transaction, although neither may write the installation's row.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error { return providers.MoveOfferCount(ctx, withdrawnRow, 1) })
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error { return providers.MoveOfferCount(ctx, withdrawnRow, 1) })
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error { return providers.MoveOfferCount(ctx, withdrawnRow, -1) })
	if got := count(); got != 1 {
		t.Errorf("the count is %d, want one workspace", got)
	}
	// It never goes below zero, and nothing but one step at a time moves it.
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error { return providers.MoveOfferCount(ctx, withdrawnRow, -1) })
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error { return providers.MoveOfferCount(ctx, withdrawnRow, -1) })
	inTenant(t, uow, idpTenantA, func(ctx context.Context) error { return providers.MoveOfferCount(ctx, withdrawnRow, 7) })
	if got := count(); got != 0 {
		t.Errorf("the count is %d, want zero - not below, and not moved by seven", got)
	}

	// Gate SG-3: a workspace's own row has no count anybody moves - not its own, not another's.
	inTenant(t, uow, idpTenantB, func(ctx context.Context) error { return providers.MoveOfferCount(ctx, idpRowA, 1) })
	var ownCount int
	if err := admin.QueryRow(ctx, `SELECT offered_workspaces FROM identity_provider WHERE id = $1`,
		idpRowA.String()).Scan(&ownCount); err == nil && ownCount != 0 {
		t.Errorf("a workspace moved another workspace's row: %d", ownCount)
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
}
