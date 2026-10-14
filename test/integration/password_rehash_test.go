// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// rehashAccountIn seeds an account of the test's own with a stored hash, a password moment and a
// start-password mark, so that what the rehash must leave alone is there to be left alone.
func rehashAccountIn(ctx context.Context, t *testing.T, tenant shared.ID, hash string) shared.ID {
	t.Helper()
	id := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO account (id, tenant_id, kind, email, display_name, password_hash, status,
		                     password_set_at, start_password_at, updated_at)
		VALUES ($1, $2, 'USER', $3, 'Rehash', $4, 'ACTIVE', $5, $5, $5)`,
		id.String(), tenant.String(), "rehash-"+id.String()+"@example.org", hash, created); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	return id
}

type storedPassword struct {
	hash             string
	setAt, startAt   time.Time
	updatedAt        time.Time
	version, history int
}

func storedPasswordOf(ctx context.Context, t *testing.T, account shared.ID) storedPassword {
	t.Helper()
	var read storedPassword
	if err := adminPool(ctx, t).QueryRow(ctx, `
		SELECT a.password_hash, a.password_set_at, a.start_password_at, a.updated_at, a.version,
		       (SELECT count(*) FROM account_password_history h WHERE h.account_id = a.id)
		FROM account a WHERE a.id = $1`, account.String()).Scan(
		&read.hash, &read.setAt, &read.startAt, &read.updatedAt, &read.version, &read.history); err != nil {
		t.Fatalf("reading the account: %v", err)
	}
	return read
}

// Gate SG-3: RehashPassword. Under another workspace's context the statement matches nothing; in
// its own it writes only while the stored hash is the one presented as current, and moves nothing
// but the hash and updated_at - the same password keeps its age, its mark, its history and its
// version.
func TestARehashWritesOnlyTheCurrentHashAndOnlyInItsTenant(t *testing.T) {
	ctx := context.Background()
	sessionFixtures(ctx, t)
	_, _, signIn, uow := sessionStores(ctx, t)
	account := rehashAccountIn(ctx, t, tenantA, "old-cost-hash")
	before := storedPasswordOf(ctx, t, account)
	at := created.Add(48 * time.Hour)

	inTenant(t, uow, tenantB, func(ctx context.Context) error {
		changed, err := signIn.RehashPassword(ctx, account, "old-cost-hash", "from-b", at)
		if err != nil || changed {
			t.Errorf("another tenant's account was rehashed from here (%v, %v)", changed, err)
		}
		return nil
	})
	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		changed, err := signIn.RehashPassword(ctx, account, "a-hash-changed-meanwhile", "stale", at)
		if err != nil || changed {
			t.Errorf("a stale current hash overwrote the stored one (%v, %v)", changed, err)
		}
		return nil
	})
	if got := storedPasswordOf(ctx, t, account); got != before {
		t.Fatalf("a refused rehash changed the row: %+v, was %+v", got, before)
	}

	inTenant(t, uow, tenantA, func(ctx context.Context) error {
		changed, err := signIn.RehashPassword(ctx, account, "old-cost-hash", "current-cost-hash", at)
		if err != nil || !changed {
			t.Errorf("the own account's rehash did not write (%v, %v)", changed, err)
		}
		return nil
	})
	after := storedPasswordOf(ctx, t, account)
	if after.hash != "current-cost-hash" || !after.updatedAt.Equal(at) {
		t.Errorf("stored %q at %v, want the new hash at %v", after.hash, after.updatedAt, at)
	}
	if !after.setAt.Equal(before.setAt) || !after.startAt.Equal(before.startAt) ||
		after.version != before.version || after.history != before.history {
		t.Errorf("the rehash moved what belongs to the password: %+v, was %+v", after, before)
	}
}
