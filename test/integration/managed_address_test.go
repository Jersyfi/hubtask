// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
)

// Migration 0131's shape: an ADDRESS row holds an address and may wait for its token; no other row
// holds an address, and no other row is without a token.
func TestAnAddressRequestIsTheOnlyPendingRowWithoutAToken(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := managedIn(t, tenantA, freshSignInName(t))
	pool := adminPool(ctx, t)

	insert := func(purpose string, address any, token any) error {
		_, err := pool.Exec(ctx, `
			INSERT INTO auth_pending (id, tenant_id, account_id, purpose, address, token_hash,
			                          created_at, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, now(), now() + interval '1 day')`,
			freshID(t).String(), tenantA.String(), account.ID.String(), purpose, address, token)
		return err
	}
	if err := insert("ADDRESS", "lena@example.org", nil); err != nil {
		t.Errorf("an address request without its token yet was refused: %v", err)
	}
	if err := insert("ADDRESS", nil, nil); err == nil {
		t.Error("an address request without an address was written")
	}
	if err := insert("RESET", "lena@example.org", []byte(freshID(t).String())); err == nil {
		t.Error("a reset link holding an address was written")
	}
	if err := insert("RESET", nil, nil); err == nil {
		t.Error("a reset link without a token was written")
	}
	if err := insert("RESET", nil, []byte(freshID(t).String())); err != nil {
		t.Errorf("a reset link as before was refused: %v", err)
	}
}
