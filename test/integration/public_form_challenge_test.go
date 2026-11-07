// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// UC-SVC-13 check 8 against the real statements: a challenge is spent once for every server, a
// rolled-back request leaves it unspent, another workspace's same nonce is its own, an expired one
// is forgotten, and a requester's contacts are counted per address within the window.
func TestASpentChallengeHoldsForEveryServer(t *testing.T) {
	ctx := context.Background()
	if _, err := adminPool(ctx, t).Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone)
		VALUES ($1, 'tenant-a', 'A', 'de', 'Europe/Berlin'), ($2, 'tenant-b', 'B', 'en', 'UTC')
		ON CONFLICT (id) DO NOTHING`, tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding the workspaces: %v", err)
	}
	ledger := postgres.PublicFormChallengeRepository{}
	now := time.Now().UTC()
	nonce := "n-" + freshID(t).String()
	take := func(tenant any, at time.Time) bool {
		t.Helper()
		var taken bool
		id := tenantA
		if tenant == "b" {
			id = tenantB
		}
		if err := write(ctx, t, id, func(ctx context.Context) error {
			var err error
			taken, err = ledger.Take(ctx, nonce, at.Add(10*time.Minute), at)
			return err
		}); err != nil {
			t.Fatalf("taking: %v", err)
		}
		return taken
	}

	refused := errors.New("the request was refused")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if taken, err := ledger.Take(ctx, nonce, now.Add(10*time.Minute), now); err != nil || !taken {
			t.Fatalf("a fresh challenge: %v %v", taken, err)
		}
		return refused
	}); !errors.Is(err, refused) {
		t.Fatal(err)
	}
	if !take("a", now) {
		t.Fatal("a rolled-back request left its challenge spent")
	}
	if take("a", now) {
		t.Error("a challenge was taken twice")
	}
	if !take("b", now) {
		t.Error("another workspace's same nonce counted as spent here")
	}
	if !take("a", now.Add(11*time.Minute)) {
		t.Error("an expired challenge was not forgotten")
	}

	item := seedWorkItemForRules(ctx, t, tenantA)
	other := seedWorkItemForRules(ctx, t, tenantA)
	store := postgres.RequesterRepository{IDs: freshIDs{t}}
	address := "form-" + freshID(t).String()[28:] + "@shop.test"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := store.RecordContact(ctx, address, "", item, now.Add(-2*time.Hour)); err != nil {
			return err
		}
		return store.RecordContact(ctx, address, "", other, now)
	}); err != nil {
		t.Fatal(err)
	}
	count := func(tenant any) int {
		t.Helper()
		id := tenantA
		if tenant == "b" {
			id = tenantB
		}
		var n int
		if err := read(ctx, t, id, func(ctx context.Context) error {
			var err error
			n, err = store.ContactsSince(ctx, "  "+address, now.Add(-time.Hour))
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count("a"); n != 1 {
		t.Errorf("%d contacts in the last hour, want 1", n)
	}
	if n := count("b"); n != 0 {
		t.Errorf("another workspace counted %d of this one's contacts", n)
	}
}
