// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The installation's presets against PostgreSQL as the application role (UC-INS-18): written in
// the scope without a tenant, read alike by every workspace, one default at most, and only the
// default locking.

func installationPresetIn(t *testing.T) domain.InstallationPreset {
	t.Helper()
	id := freshID(t)
	return domain.InstallationPreset{Preset: domain.Preset{
		Key: domain.SavedPresetKey(id), Origin: domain.PresetOriginInstallation, ID: id, Name: freshName(t),
		Values: domain.StructureValues{
			Levels:     []domain.Level{{Type: domain.ItemTask, Name: "Order"}},
			Completion: domain.CompletionManual,
		},
		Version: 1,
	}}
}

// withdrawnAtEnd removes the preset when the test ends: the table is the installation's, shared by
// every test in the run, and one default at most may stand.
func withdrawnAtEnd(ctx context.Context, t *testing.T, id shared.ID) {
	t.Cleanup(func() {
		_ = asInstallation(context.WithoutCancel(ctx), t, true, func(ctx context.Context) error {
			return postgres.NewInstallationPresetRepository().Delete(ctx, id, nil)
		})
	})
}

func TestAnInstallationPresetIsReadAlikeByEveryWorkspace(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	presets := postgres.NewInstallationPresetRepository()
	preset := installationPresetIn(t)
	if err := asInstallation(ctx, t, true, func(ctx context.Context) error {
		return presets.Insert(ctx, preset, authorA, created)
	}); err != nil {
		t.Fatalf("saving the installation's preset: %v", err)
	}
	withdrawnAtEnd(ctx, t, preset.ID)

	for _, tenant := range []shared.ID{tenantA, tenantB} {
		var found domain.InstallationPreset
		var listed []domain.InstallationPreset
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			if found, err = presets.Find(ctx, preset.ID); err != nil {
				return err
			}
			listed, err = presets.List(ctx)
			return err
		}); err != nil {
			t.Fatalf("workspace %s reading the installation's preset: %v", tenant, err)
		}
		if found.Name != preset.Name || found.Origin != domain.PresetOriginInstallation || found.Version != 1 ||
			len(found.Values.Levels) != 1 || found.Values.Levels[0].Name != "Order" || found.IsDefault {
			t.Errorf("workspace %s read %+v", tenant, found)
		}
		if !slices.ContainsFunc(listed, func(p domain.InstallationPreset) bool { return p.ID == preset.ID }) {
			t.Errorf("workspace %s is not offered the installation's preset", tenant)
		}
	}

	again := installationPresetIn(t)
	again.Name = preset.Name
	err := asInstallation(ctx, t, true, func(ctx context.Context) error { return presets.Insert(ctx, again, authorA, created) })
	if shared.AsError(err) == nil || shared.AsError(err).DetailCode != "presets.name_taken" {
		t.Errorf("a name taken was saved: %v", err)
	}
}

func TestOneInstallationPresetIsTheDefaultAndOnlyItLocks(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	presets := postgres.NewInstallationPresetRepository()
	first, second := installationPresetIn(t), installationPresetIn(t)
	first.IsDefault, first.Locks = true, []domain.LockKey{"views"}
	for _, preset := range []domain.InstallationPreset{first, second} {
		if err := asInstallation(ctx, t, true, func(ctx context.Context) error {
			if preset.IsDefault {
				if err := presets.ClearDefault(ctx, preset.ID, authorA, created); err != nil {
					return err
				}
			}
			return presets.Insert(ctx, preset, authorA, created)
		}); err != nil {
			t.Fatalf("saving: %v", err)
		}
		withdrawnAtEnd(ctx, t, preset.ID)
	}

	// A lock on a preset that is not the default is refused by the table itself.
	locked := second
	locked.Locks = []domain.LockKey{"views"}
	if err := asInstallation(ctx, t, true, func(ctx context.Context) error {
		return presets.Update(ctx, locked, nil, authorA, created)
	}); err == nil {
		t.Error("a preset that is not the default took a lock")
	}

	// Making the second the default lifts the first's default and its locks.
	second.IsDefault = true
	if err := asInstallation(ctx, t, true, func(ctx context.Context) error {
		if err := presets.ClearDefault(ctx, second.ID, authorA, created); err != nil {
			return err
		}
		return presets.Update(ctx, second, expecting(1), authorA, created)
	}); err != nil {
		t.Fatalf("making the second the default: %v", err)
	}
	var standing domain.InstallationPreset
	var formerly domain.InstallationPreset
	var held bool
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		if standing, held, err = presets.Default(ctx); err != nil {
			return err
		}
		formerly, err = presets.Find(ctx, first.ID)
		return err
	}); err != nil {
		t.Fatalf("reading the default: %v", err)
	}
	if !held || standing.ID != second.ID || standing.Version != 2 {
		t.Errorf("the default is %+v (%v)", standing.Preset, held)
	}
	if formerly.IsDefault || len(formerly.Locks) != 0 {
		t.Errorf("the former default kept %v, %v", formerly.IsDefault, formerly.Locks)
	}

	// A change on a version that moved on is a conflict, a withdrawn one is not found.
	if err := asInstallation(ctx, t, true, func(ctx context.Context) error {
		return presets.Update(ctx, second, expecting(1), authorA, created)
	}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale change: %v", err)
	}
	if err := asInstallation(ctx, t, true, func(ctx context.Context) error {
		return presets.Delete(ctx, first.ID, nil)
	}); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := presets.Find(ctx, first.ID)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a withdrawn preset: %v", err)
	}
}

func expecting(version int) *int { return &version }
