// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// seedCapabilityOverride gives tenant A a narrowed TASK profile. Narrowing is what a tenant may
// do; the system row stays untouched (domain-model.md §2).
func seedCapabilityOverride(ctx context.Context, t *testing.T) {
	t.Helper()
	admin := adminPool(ctx, t)

	if _, err := admin.Exec(ctx, `
		INSERT INTO tenant (id, slug, display_name) VALUES ($1, 'tenant-a', 'A'), ($2, 'tenant-b', 'B')
		ON CONFLICT (id) DO NOTHING`, tenantA.String(), tenantB.String()); err != nil {
		t.Fatalf("seeding tenants: %v", err)
	}
	if _, err := admin.Exec(ctx, `
		INSERT INTO item_capability_profile (tenant_id, type, capabilities, allowed_child_types, max_depth)
		VALUES ($1, 'TASK', ARRAY['COMPLETION'], ARRAY[]::item_type[], 1)
		ON CONFLICT DO NOTHING`, tenantA.String()); err != nil {
		t.Fatalf("seeding the override: %v", err)
	}
}

func profilesIn(ctx context.Context, t *testing.T, scope persistence.Scope) map[work.ItemType]work.CapabilityProfile {
	t.Helper()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	repo := postgres.NewCapabilityProfileRepository()

	var profiles []work.CapabilityProfile
	if err := uow.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		profiles, err = repo.List(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the profiles: %v", err)
	}

	byType := make(map[work.ItemType]work.CapabilityProfile, len(profiles))
	for _, profile := range profiles {
		byType[profile.Type] = profile
	}
	return byType
}

// The migration ships the capability matrix as data, and the installation scope is what reads it
// without a tenant.
func TestTheSystemProfilesAreReadableWithoutATenant(t *testing.T) {
	ctx := context.Background()
	seedCapabilityOverride(ctx, t)

	profiles := profilesIn(ctx, t, persistence.InstallationScope())

	for _, itemType := range []work.ItemType{work.ItemTask, work.ItemWorkPackage, work.ItemActivity} {
		if _, found := profiles[itemType]; !found {
			t.Errorf("no system profile for %s", itemType)
		}
	}
	task := profiles[work.ItemTask]
	if !task.Allows(work.CapabilityCover) || task.MaxDepth != 3 {
		t.Errorf("the system TASK profile is not the one the migration seeds: %+v", task)
	}
	if !task.AllowsChild(work.ItemWorkPackage) {
		t.Error("a work package may not sit under a task")
	}
	activity := profiles[work.ItemActivity]
	if activity.Allows(work.CapabilityNotes) || len(activity.AllowedChildTypes) != 0 {
		t.Errorf("the system ACTIVITY profile is not the reduced level: %+v", activity)
	}
}

// The cross-tenant negative test for this repository method: an override belongs to the tenant
// that wrote it and to nobody else.
func TestATenantsOverrideIsInvisibleElsewhere(t *testing.T) {
	ctx := context.Background()
	seedCapabilityOverride(ctx, t)

	t.Run("its own tenant sees it", func(t *testing.T) {
		task := profilesIn(ctx, t, persistence.Scope{TenantID: tenantA})[work.ItemTask]
		if task.Allows(work.CapabilityCover) || task.MaxDepth != 1 {
			t.Errorf("tenant A did not get its override: %+v", task)
		}
	})

	t.Run("another tenant sees the system default", func(t *testing.T) {
		task := profilesIn(ctx, t, persistence.Scope{TenantID: tenantB})[work.ItemTask]
		if !task.Allows(work.CapabilityCover) || task.MaxDepth != 3 {
			t.Errorf("tenant B saw tenant A's override: %+v", task)
		}
	})

	t.Run("the installation scope sees the system default", func(t *testing.T) {
		task := profilesIn(ctx, t, persistence.InstallationScope())[work.ItemTask]
		if !task.Allows(work.CapabilityCover) || task.MaxDepth != 3 {
			t.Errorf("an anonymous read saw a tenant's override: %+v", task)
		}
	})
}

// An override never adds a type: one row per item type reaches the caller, whichever applies.
func TestOneProfilePerItemType(t *testing.T) {
	ctx := context.Background()
	seedCapabilityOverride(ctx, t)

	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	var profiles []work.CapabilityProfile
	if err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantA}, func(ctx context.Context) error {
		var err error
		profiles, err = postgres.NewCapabilityProfileRepository().List(ctx)
		return err
	}); err != nil {
		t.Fatalf("reading the profiles: %v", err)
	}

	seen := map[work.ItemType]int{}
	for _, profile := range profiles {
		seen[profile.Type]++
	}
	for itemType, count := range seen {
		if count != 1 {
			t.Errorf("%s appears %d times - the caller would have to choose", itemType, count)
		}
	}
}

// The installation scope is read-only by construction: every WITH CHECK compares against a tenant
// it deliberately does not have, so a write under it is a programming error with a name rather
// than a policy violation at run time.
func TestTheInstallationScopeRefusesAWriteTransaction(t *testing.T) {
	ctx := context.Background()
	uow := postgres.NewUnitOfWork(appPool(ctx, t))

	err := uow.Within(ctx, persistence.InstallationScope(), func(context.Context) error { return nil })
	if err == nil {
		t.Fatal("a read-write transaction was opened without a tenant")
	}
}

// And with no tenant context set, no tenant's rows are reachable at all - which is what makes the
// installation scope the strictest position inside the boundary rather than a way around it.
func TestTheInstallationScopeSeesNoTenantRows(t *testing.T) {
	ctx := context.Background()
	seedCapabilityOverride(ctx, t)

	uow := postgres.NewUnitOfWork(appPool(ctx, t))
	var visible int
	if err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		tx, err := postgres.FromContext(ctx)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM tenant`).Scan(&visible)
	}); err != nil {
		t.Fatalf("counting tenants: %v", err)
	}
	if visible != 0 {
		t.Errorf("%d tenant rows visible without a tenant context", visible)
	}
}
