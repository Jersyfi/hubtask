// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The workspace's own row against the real boundary (F4-01). Gate SG-3: one workspace reads and
// changes itself and reaches nothing of the one next door - and the settings document keeps the
// keys this build does not model, which is the property no unit test can show.

var (
	wsTenantA = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd01")
	wsTenantB = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000fd02")
)

func seedWorkspaceTenants(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)
	statements := []string{
		// A carries a settings key this build does not model, put there the way a later release
		// would: the merge on the write has to leave it alone.
		`INSERT INTO tenant (id, slug, display_name, default_locale, default_time_zone, settings)
		 VALUES ('` + wsTenantA.String() + `', 'ws-a', 'Workspace A', 'en', 'UTC',
		         '{"require_admin_totp": false, "future_key": {"kept": true}}'::jsonb)
		 ON CONFLICT (id) DO NOTHING`,
		`INSERT INTO tenant (id, slug, display_name)
		 VALUES ('` + wsTenantB.String() + `', 'ws-b', 'Workspace B')
		 ON CONFLICT (id) DO NOTHING`,
	}
	for _, statement := range statements {
		if _, err := admin.Exec(ctx, statement); err != nil {
			t.Fatalf("seeding the workspace tenants: %v", err)
		}
	}
}

func TestAWorkspaceReadsAndChangesItselfAndNothingNextDoor(t *testing.T) {
	ctx := context.Background()
	seedWorkspaceTenants(ctx, t)

	workspaces := postgres.NewWorkspaceSettingsRepository()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	now := time.Now().UTC()

	var stored domain.Workspace
	inTenant(t, uow, wsTenantA, func(ctx context.Context) error {
		read, err := workspaces.Find(ctx)
		if err != nil {
			t.Fatalf("reading A: %v", err)
		}
		if read.ID != wsTenantA || read.Slug != "ws-a" || read.DisplayName != "Workspace A" {
			t.Errorf("A read itself as %+v", read.Tenant)
		}
		if read.Settings.RequireAdminTotp {
			t.Error("A demands a second factor without anybody having said so")
		}
		stored = read
		return nil
	})

	// A changes its name and switches enforcement on, guarded on the version it read.
	changed, moved, err := stored.With(domain.WorkspaceChange{
		DisplayName:      settingOf("Workspace A GmbH"),
		DefaultTimeZone:  settingOf("Europe/Berlin"),
		RequireAdminTotp: settingOf(true),
	})
	if err != nil {
		t.Fatalf("applying the change: %v", err)
	}
	if len(moved) != 3 {
		t.Fatalf("%d fields moved, want three", len(moved))
	}

	inTenant(t, uow, wsTenantA, func(ctx context.Context) error {
		written, err := workspaces.Update(ctx, changed, stored.Version, now)
		if err != nil {
			t.Fatalf("writing A: %v", err)
		}
		if !written {
			t.Fatal("the guarded write did not hold on the version just read")
		}
		return nil
	})

	// The version it was written against is spent: the same write again is refused rather than
	// applied a second time.
	inTenant(t, uow, wsTenantA, func(ctx context.Context) error {
		written, err := workspaces.Update(ctx, changed, stored.Version, now)
		if err != nil {
			t.Fatalf("writing A again: %v", err)
		}
		if written {
			t.Error("a stale version wrote anyway")
		}
		return nil
	})

	inTenant(t, uow, wsTenantA, func(ctx context.Context) error {
		read, err := workspaces.Find(ctx)
		if err != nil {
			t.Fatalf("reading A back: %v", err)
		}
		if read.DisplayName != "Workspace A GmbH" || read.DefaultTimeZone != "Europe/Berlin" {
			t.Errorf("A came back as %+v", read.Tenant)
		}
		if !read.Settings.RequireAdminTotp {
			t.Error("the enforcement switch did not survive the write")
		}
		if read.Version != stored.Version+1 {
			t.Errorf("version %d, want %d", read.Version, stored.Version+1)
		}
		if read.UpdatedAt.IsZero() {
			t.Error("the write left no updated_at")
		}
		return nil
	})

	// The settings key this build does not model is still there. The write merges rather than
	// replaces, which is what stops an older binary from discarding a newer one's key.
	var keptFutureKey bool
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT coalesce((settings #> '{future_key,kept}')::boolean, false)
		 FROM tenant WHERE id = $1`, wsTenantA).Scan(&keptFutureKey); err != nil {
		t.Fatalf("reading the settings document: %v", err)
	}
	if !keptFutureKey {
		t.Error("the write discarded a settings key this build does not model")
	}

	// Gate SG-3: B is untouched, sees itself, and its own write reaches nothing of A's.
	inTenant(t, uow, wsTenantB, func(ctx context.Context) error {
		read, err := workspaces.Find(ctx)
		if err != nil {
			t.Fatalf("reading B: %v", err)
		}
		if read.ID != wsTenantB || read.DisplayName != "Workspace B" {
			t.Errorf("B read %+v, want its own row", read.Tenant)
		}
		if read.Settings.RequireAdminTotp {
			t.Error("A's enforcement switch reached B")
		}

		// The identifier is not a parameter anywhere, so B cannot even ask for A - what it can
		// do is write, and what it writes has to land on its own row.
		next, _, err := read.With(domain.WorkspaceChange{DisplayName: settingOf("B, renamed")})
		if err != nil {
			t.Fatalf("applying B's change: %v", err)
		}
		if _, err := workspaces.Update(ctx, next, read.Version, now); err != nil {
			t.Fatalf("writing B: %v", err)
		}
		return nil
	})

	inTenant(t, uow, wsTenantA, func(ctx context.Context) error {
		read, err := workspaces.Find(ctx)
		if err != nil {
			t.Fatalf("reading A after B's write: %v", err)
		}
		if read.DisplayName != "Workspace A GmbH" {
			t.Errorf("B's write reached A: %q", read.DisplayName)
		}
		return nil
	})
}

// settingOf is this file's pointer helper; `pointerTo` in update_test.go takes strings only.
func settingOf[T any](value T) *T { return &value }
