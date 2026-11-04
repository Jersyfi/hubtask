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

// The mark of an address the mail server refused for good: written, kept at its first time while
// the same address is refused again, moved by a refusal at a new address, cleared, and never
// reached from another tenant (gate SG-3).
func TestAnUndeliverableMarkIsKeptMovedAndCleared(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := seedAccount(ctx, t, tenantA)
	marks := postgres.NewMailUndeliverableRepository()
	first := created
	later := created.Add(time.Hour)

	mark := func(tenant shared.ID, address string, at time.Time) error {
		return write(ctx, t, tenant, func(ctx context.Context) error { return marks.Mark(ctx, account, address, at) })
	}
	find := func(tenant shared.ID) (string, time.Time, error) {
		var address string
		var since time.Time
		err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			address, since, err = marks.Find(ctx, account)
			return err
		})
		return address, since, err
	}

	if err := mark(tenantA, "gone@example.org", first); err != nil {
		t.Fatal(err)
	}
	if err := mark(tenantA, "gone@example.org", later); err != nil {
		t.Fatal(err)
	}
	if address, since, err := find(tenantA); err != nil || address != "gone@example.org" || !since.Equal(first) {
		t.Fatalf("found %q since %v (%v), want the first refusal kept", address, since, err)
	}
	if err := mark(tenantA, "new@example.org", later); err != nil {
		t.Fatal(err)
	}
	if address, since, err := find(tenantA); err != nil || address != "new@example.org" || !since.Equal(later) {
		t.Fatalf("found %q since %v (%v), want the new address marked", address, since, err)
	}

	// Tenant B's transaction, tenant A's account: nothing is found, cleared or written.
	if _, _, err := find(tenantB); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("Find across the tenant boundary: %v, want not found", err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error { return marks.Clear(ctx, account) }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := find(tenantA); err != nil {
		t.Errorf("a clear from tenant B removed tenant A's mark: %v", err)
	}
	if err := mark(tenantB, "stray@example.org", later); err == nil {
		t.Error("a mark for another tenant's account was written")
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return marks.Clear(ctx, account) }); err != nil {
		t.Fatal(err)
	}
	if _, _, err := find(tenantA); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("after the clear: %v, want not found", err)
	}
}
