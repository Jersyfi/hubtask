// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Mail addresses against a real database (ADR-0092): the door names one tenant by code and never
// lists, a replaced address answers nothing, one place has one live address, and every method is
// bounded by row level security (gate SG-3).

func mintAddress(
	ctx context.Context, t *testing.T, tenant shared.ID, target domain.MailTarget, seed byte,
) domain.MailAddress {
	t.Helper()
	address, err := domain.NewMailAddress(freshID(t), tenant, target,
		[]byte{seed, seed, seed, seed, seed, seed, seed, seed, seed, byte(time.Now().UnixNano())},
		"", time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.NewMailAddressRepository().Replace(ctx, address, address.CreatedAt)
	}); err != nil {
		t.Fatalf("storing the address: %v", err)
	}
	return address
}

func tenantOfCode(ctx context.Context, t *testing.T, code string) (shared.ID, error) {
	t.Helper()
	var found shared.ID
	err := postgres.NewUnitOfWork(appPool(ctx, t)).WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			var err error
			found, err = postgres.NewMailAddressRepository().TenantOf(ctx, code)
			return err
		})
	return found, err
}

func TestTheMailDoorNamesOneWorkspaceByCode(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	collection := domain.MailTarget{Kind: domain.TargetCollection, ID: freshID(t)}
	a := mintAddress(ctx, t, tenantA, collection, 11)
	b := mintAddress(ctx, t, tenantB, domain.MailTarget{Kind: domain.TargetJumble}, 12)

	if found, err := tenantOfCode(ctx, t, a.Code); err != nil || found != tenantA {
		t.Errorf("a's code named %q, %v", found, err)
	}
	if found, err := tenantOfCode(ctx, t, b.Code); err != nil || found != tenantB {
		t.Errorf("b's code named %q, %v", found, err)
	}
	if _, err := tenantOfCode(ctx, t, "aaaaaaaaaaaaaaaa"); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an unknown code answered %v", err)
	}

	// Replaced: the old code names nobody, the new one the same workspace.
	renewed := mintAddress(ctx, t, tenantA, collection, 13)
	if _, err := tenantOfCode(ctx, t, a.Code); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a replaced code answered %v", err)
	}
	var live domain.MailAddress
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		live, err = postgres.NewMailAddressRepository().Live(ctx, collection)
		return err
	}); err != nil || live.Code != renewed.Code {
		t.Errorf("the live address is %+v, %v", live, err)
	}
}

func TestAMailAddressIsBoundedByItsWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	target := domain.MailTarget{Kind: domain.TargetItem, ID: freshID(t)}
	a := mintAddress(ctx, t, tenantA, target, 21)
	repo := postgres.NewMailAddressRepository()

	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := repo.Live(ctx, target); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("Live under another workspace answered %v", err)
		}
		if _, err := repo.ByCode(ctx, a.Code); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("ByCode under another workspace answered %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		opened, _ := a.WithPolicy(domain.SendersAnyone, authorB)
		if err := repo.SetPolicy(ctx, opened); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("SetPolicy under another workspace answered %v", err)
		}
		if count, err := repo.CountDelivery(ctx, a.ID, time.Now()); err == nil && count != 0 {
			t.Errorf("CountDelivery under another workspace counted %d", count)
		}
		return nil
	}); err != nil && !errors.Is(err, shared.ErrNotFound) {
		t.Fatal(err)
	}

	// In its own workspace: the policy sticks and the hour counts.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		opened, _ := a.WithPolicy(domain.SendersAnyone, authorA)
		if err := repo.SetPolicy(ctx, opened); err != nil {
			return err
		}
		for want := 1; want <= 3; want++ {
			count, err := repo.CountDelivery(ctx, a.ID, time.Now())
			if err != nil {
				return err
			}
			if count != want {
				t.Errorf("delivery %d counted %d", want, count)
			}
		}
		stored, err := repo.ByCode(ctx, a.Code)
		if err != nil {
			return err
		}
		if stored.Policy != domain.SendersAnyone || stored.OpenedBy != authorA {
			t.Errorf("read back %+v", stored)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAJumbleEntryKeepsWhereTheMailWasMeantFor(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	meant := domain.MailTarget{Kind: domain.TargetCollection, ID: freshID(t)}
	entry := seedJumbleEntry(ctx, t, tenantA, func(in *domain.NewEntryInput) {
		in.Channel = domain.ChannelEmail
		in.MeantFor = meant
	})
	var stored domain.Entry
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = jumbleRepo().Find(ctx, entry.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if stored.MeantFor != meant {
		t.Errorf("meant for %+v, want %+v", stored.MeantFor, meant)
	}
}
