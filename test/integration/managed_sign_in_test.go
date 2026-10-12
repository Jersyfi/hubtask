// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Gate SG-3: FindForSignIn by a sign-in name. The credential check's read finds a managed account by
// its folded name, in its own workspace only, and a name never reaches an address's row.
func TestASignInNameSignsInOnlyInItsOwnTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	_, _, signIn, uow := sessionStores(ctx, t)
	name := freshSignInName(t)
	account := managedIn(t, tenantA, name)

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		found, err := signIn.FindForSignIn(ctx, name)
		if err != nil {
			t.Fatalf("the own name was not found: %v", err)
		}
		if found.Account.ID != account.ID || found.Account.SignInName != name || found.PasswordHash.IsEmpty() {
			t.Errorf("found %+v", found.Account)
		}
		// The address of an account in the same workspace is not a name, and a name is no address.
		if _, err := signIn.FindForSignIn(ctx, name+"@example.org"); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("the name as an address answered %v, want not found", err)
		}
		return nil
	})
	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		if _, err := signIn.FindForSignIn(ctx, name); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("another tenant's name answered %v, want not found", err)
		}
		return nil
	})
}

// A password set through any door is the person's own: the start password's mark goes with it, so
// the next sign-in owes no change.
func TestSettingAPasswordClearsTheStartPasswordsMark(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := managedIn(t, tenantA, freshSignInName(t))

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		changed, err := postgres.NewPasswordRepository().SetPassword(ctx, account.ID, "chosen-hash", created.Add(time.Hour))
		if err == nil && !changed {
			t.Error("the password was not written")
		}
		return err
	}); err != nil {
		t.Fatalf("setting the password: %v", err)
	}
	var marked *time.Time
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT start_password_at FROM account WHERE id = $1`, account.ID.String()).Scan(&marked); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if marked != nil {
		t.Errorf("the start password's mark survived the person's own password: %v", marked)
	}
}
