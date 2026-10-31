// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A desk's logo against a real database (UC-SVC-05 check 9): written and read back on its
// collection, unreachable from another workspace (gate SG-3 for Find, Set and ServedDesks), kept
// alive by the recount, and cleared rather than refused when the picture goes.
func TestADesksLogoIsSetReadAndCountedInItsOwnWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	collection := collectionFor(ctx, t, tenantA, authorA)
	logo := sealMedia(ctx, t, tenantA, stagedMedia(ctx, t, tenantA, authorA, media.UsageDeskLogo))
	logos := postgres.DeskLogoRepository{}

	err := write(ctx, t, tenantB, func(ctx context.Context) error { return logos.Set(ctx, collection, logo.ID) })
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("setting from tenant B: %v, want not found", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return logos.Set(ctx, collection, logo.ID) }); err != nil {
		t.Fatalf("setting: %v", err)
	}

	var found shared.ID
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		found, err = logos.Find(ctx, collection)
		return err
	}); err != nil || found != logo.ID {
		t.Fatalf("read back %q, %v; want %q", found, err, logo.ID)
	}
	err = read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := logos.Find(ctx, collection)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("reading from tenant B: %v, want not found", err)
	}

	for tenant, want := range map[shared.ID]int{tenantA: 1, tenantB: 0} {
		var desks []shared.ID
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			desks, err = mediaRepo().ServedDesks(ctx, logo.ID)
			return err
		}); err != nil || len(desks) != want {
			t.Errorf("tenant %s sees desks %v, %v; want %d", tenant, desks, err, want)
		}
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error { return mediaRepo().Recount(ctx, created) }); err != nil {
		t.Fatalf("recounting: %v", err)
	}
	if stored := findMedia(ctx, t, tenantA, logo.ID); stored.RefCount != 1 {
		t.Errorf("ref_count %d, want 1", stored.RefCount)
	}

	// The object going clears the logo: a replace-restore and an erasure both remove media first.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := mediaRepo().RemoveRows(ctx, []shared.ID{logo.ID})
		return err
	}); err != nil {
		t.Fatalf("removing the logo: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		found, err = logos.Find(ctx, collection)
		return err
	}); err != nil || !found.IsZero() {
		t.Errorf("after the picture went the desk shows %q, %v", found, err)
	}
}
