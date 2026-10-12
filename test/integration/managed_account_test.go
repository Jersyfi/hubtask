// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// freshSignInName keeps the tests independent: a name is unique per workspace, and the tenants are
// shared across the suite.
func freshSignInName(t *testing.T) string {
	t.Helper()
	return "kid-" + freshID(t).String()[24:]
}

func managedIn(t *testing.T, tenant shared.ID, name string) identity.Account {
	t.Helper()
	account, err := identity.NewManagedAccount(freshID(t), tenant, name, "Lena", nil)
	if err != nil {
		t.Fatalf("building the account: %v", err)
	}
	if err := write(context.Background(), t, tenant, func(ctx context.Context) error {
		return postgres.NewAccountRepository().InsertManaged(ctx, account, "argon2id-hash", created)
	}); err != nil {
		t.Fatalf("writing the account: %v", err)
	}
	return account
}

// UC-ID-20 check 1, at the store: a managed account is written active with its name, the start
// password's hash and its mark, and is found again by the folded name.
func TestAManagedAccountIsWrittenAndFoundByItsName(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := managedIn(t, tenantA, freshSignInName(t))

	var found identity.Account
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		found, err = postgres.NewAccountRepository().FindBySignInName(ctx, account.SignInName)
		return err
	}); err != nil {
		t.Fatalf("finding by name: %v", err)
	}
	if found.ID != account.ID || found.Status != identity.AccountActive || found.Email != "" {
		t.Errorf("found %+v", found)
	}

	var (
		hash   *string
		marked *time.Time
	)
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT password_hash, start_password_at FROM account WHERE id = $1`, account.ID.String(),
	).Scan(&hash, &marked); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if hash == nil || *hash != "argon2id-hash" || marked == nil || !marked.Equal(created) {
		t.Errorf("the row holds %v, %v", hash, marked)
	}
}

// The cross-tenant negative test for the managed side of the account store: another workspace's
// name is not found, and the same name in two workspaces is two people.
func TestAManagedAccountOfAnotherTenantIsNotFound(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	name := freshSignInName(t)
	managedIn(t, tenantA, name)

	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := postgres.NewAccountRepository().FindBySignInName(ctx, name)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("finding by name across tenants: %v, want not found", err)
	}

	// Written in the other workspace under the same name: allowed, and it lands there.
	other := managedIn(t, tenantB, name)
	var owner string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT tenant_id::text FROM account WHERE id = $1`, other.ID.String()).Scan(&owner); err != nil {
		t.Fatalf("reading the second account: %v", err)
	}
	if owner != tenantB.String() {
		t.Errorf("the second account landed in %s", owner)
	}
}

// Two administrators taking one name at once: both pass the check, the index decides, and the loser
// hears "taken" rather than a driver error.
func TestASecondAccountUnderATakenNameIsAConflict(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	name := freshSignInName(t)
	managedIn(t, tenantA, name)

	second, err := identity.NewManagedAccount(freshID(t), tenantA, name, "Other", nil)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	err = write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewAccountRepository().InsertManaged(ctx, second, "argon2id-hash", created)
	})
	if !errors.Is(err, shared.ErrConflict) || shared.AsError(err).DetailCode != "accounts.sign_in_name_taken" {
		t.Errorf("the second account answered %v, want accounts.sign_in_name_taken", err)
	}
}
