// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Gate SG-3: HasManaged. The card's question is answered from the workspace's own rows: a managed
// account in one workspace says nothing about another, and an anonymised one has left the question.
func TestWhetherAWorkspaceHasManagedAccountsStaysInIt(t *testing.T) {
	ctx := context.Background()
	tenant, _, _ := restrictedTenant(ctx, t)
	other, _, _ := restrictedTenant(ctx, t)
	account := managedIn(t, tenant, freshSignInName(t))

	ask := func(in shared.ID) bool {
		t.Helper()
		var present bool
		if err := write(ctx, t, in, func(ctx context.Context) error {
			var err error
			present, err = postgres.NewAccountRepository().HasManaged(ctx)
			return err
		}); err != nil {
			t.Fatalf("asking: %v", err)
		}
		return present
	}
	if !ask(tenant) {
		t.Error("the workspace with a managed account answered no")
	}
	if ask(other) {
		t.Error("another workspace answered yes for the first one's account")
	}

	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE account SET status = 'ANONYMIZED', sign_in_name = NULL WHERE id = $1`, account.ID.String()); err != nil {
		t.Fatalf("anonymising: %v", err)
	}
	if ask(tenant) {
		t.Error("an anonymised account still counts as a managed one")
	}
}
