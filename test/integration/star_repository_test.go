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

// UC-WRK-37 checks 2 and 3: a person's stars are written whole and read back in their order, a
// stale version writes nothing, and the workspace next door neither reads nor writes them
// (migration 0389, a cross-tenant negative for every method).
func TestStarsAreKeptInOrderGuardedByTheirVersionAndStayInTheirWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	stars := postgres.NewStarRepository()
	account := seedAccount(ctx, t, tenantA)
	at := time.Date(2026, 11, 7, 9, 0, 0, 0, time.UTC)
	page, hub := freshID(t), freshID(t)
	want := []identity.Star{{Kind: identity.StarPage, ID: page}, {Kind: identity.StarHub, ID: hub}}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		empty, err := stars.Find(ctx, account)
		if err != nil || len(empty.Stars) != 0 || empty.Version != 0 {
			t.Errorf("before the first star %+v, %v", empty, err)
		}
		if err := stars.Replace(ctx, account, want, 0, at); err != nil {
			return err
		}
		if err := stars.Replace(ctx, account, want[:1], 0, at); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("a second first write: %v, want a version conflict", err)
		}
		if err := stars.Replace(ctx, account, []identity.Star{want[1], want[0]}, 1, at); err != nil {
			return err
		}
		if err := stars.Replace(ctx, account, want, 1, at); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("a stale write: %v, want a version conflict", err)
		}
		got, err := stars.Find(ctx, account)
		if err != nil || got.Version != 2 || len(got.Stars) != 2 || got.Stars[0] != want[1] || got.Stars[1] != want[0] {
			t.Errorf("read back %+v, %v; want the hub then the page at version 2", got, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		got, err := stars.Find(ctx, account)
		if err != nil || len(got.Stars) != 0 || got.Version != 0 {
			t.Errorf("the workspace next door read %+v, %v", got, err)
		}
		if err := stars.Replace(ctx, account, want, 2, at); !errors.Is(err, shared.ErrVersionConflict) {
			t.Errorf("the workspace next door wrote: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
