// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Saved presets against PostgreSQL as the application role (UC-WRK-30 check 1): stored with their
// settings, offered to whom they belong, and invisible to another tenant on every method.

func savedPresetIn(t *testing.T, origin domain.PresetOrigin, owner shared.ID) domain.Preset {
	t.Helper()
	id := freshID(t)
	values := domain.StructureValues{
		Levels:      []domain.Level{{Type: domain.ItemTask, Name: "Client"}},
		Completion:  domain.CompletionManual,
		Celebration: domain.CelebrationSubtle,
		Automation:  []domain.AutomationBehaviour{},
	}
	if origin != domain.PresetOriginOwn {
		owner = ""
	}
	return domain.Preset{
		Key: domain.SavedPresetKey(id), Origin: origin, ID: id, Name: freshName(t), OwnerID: owner,
		Values: values, Version: 1,
	}
}

func insertPreset(ctx context.Context, t *testing.T, tenant shared.ID, preset domain.Preset) {
	t.Helper()
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return postgres.NewPresetRepository().Insert(ctx, preset, created)
	}); err != nil {
		t.Fatalf("saving the preset: %v", err)
	}
}

func TestASavedPresetIsReadBackWithItsSettings(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	preset := savedPresetIn(t, domain.PresetOriginOwn, authorA)
	insertPreset(ctx, t, tenantA, preset)

	var found domain.Preset
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		found, err = postgres.NewPresetRepository().Find(ctx, preset.ID)
		return err
	}); err != nil {
		t.Fatalf("reading the preset: %v", err)
	}
	if found.Name != preset.Name || found.Origin != domain.PresetOriginOwn || found.OwnerID != authorA ||
		found.Version != 1 || found.Key != domain.SavedPresetKey(preset.ID) {
		t.Fatalf("read back %+v, want %+v", found, preset)
	}
	if len(found.Values.Levels) != 1 || found.Values.Levels[0].Name != "Client" ||
		found.Values.Completion != domain.CompletionManual || found.Values.Automation == nil {
		t.Fatalf("settings read back as %+v", found.Values)
	}
}

func TestOnlyTheWorkspacesAndOnesOwnPresetsAreOffered(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	other := seedAccount(ctx, t, tenantA)
	workspace := savedPresetIn(t, domain.PresetOriginWorkspace, "")
	mine := savedPresetIn(t, domain.PresetOriginOwn, authorA)
	theirs := savedPresetIn(t, domain.PresetOriginOwn, other)
	for _, preset := range []domain.Preset{workspace, mine, theirs} {
		insertPreset(ctx, t, tenantA, preset)
	}

	var offered []domain.Preset
	var own, workspaceCount int
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		repo := postgres.NewPresetRepository()
		var err error
		if offered, err = repo.ListOffered(ctx, authorA); err != nil {
			return err
		}
		if own, err = repo.Count(ctx, domain.PresetOriginOwn, other); err != nil {
			return err
		}
		workspaceCount, err = repo.Count(ctx, domain.PresetOriginWorkspace, "")
		return err
	}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	seen := map[shared.ID]bool{}
	for _, preset := range offered {
		seen[preset.ID] = true
	}
	if !seen[workspace.ID] || !seen[mine.ID] || seen[theirs.ID] {
		t.Fatalf("offered %v: want the workspace's and one's own, never another person's", seen)
	}
	if own != 1 || workspaceCount < 1 {
		t.Fatalf("counted %d own and %d of the workspace's", own, workspaceCount)
	}
}

func TestAPresetIsChangedAndWithdrawnUnderItsVersion(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	preset := savedPresetIn(t, domain.PresetOriginWorkspace, "")
	insertPreset(ctx, t, tenantA, preset)
	repo := postgres.NewPresetRepository()

	renamed := preset
	renamed.Name = freshName(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Update(ctx, renamed, 1, created)
	}); err != nil {
		t.Fatalf("changing: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Update(ctx, renamed, 1, created)
	}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale change answered %v, want a version conflict", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Delete(ctx, preset.ID, 2)
	}); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := repo.Find(ctx, preset.ID)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a withdrawn preset answered %v, want not found", err)
	}
}

// Every method, from tenant B, on tenant A's preset: nothing is read, counted, changed or removed.
func TestAnotherTenantsPresetIsNeitherReadNorWritten(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	preset := savedPresetIn(t, domain.PresetOriginWorkspace, "")
	insertPreset(ctx, t, tenantA, preset)
	repo := postgres.NewPresetRepository()

	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		_, err := repo.Find(ctx, preset.ID)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("Find from tenant B answered %v, want not found", err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		offered, err := repo.ListOffered(ctx, authorB)
		for _, seen := range offered {
			if seen.ID == preset.ID {
				t.Errorf("ListOffered from tenant B answered tenant A's preset")
			}
		}
		return err
	}); err != nil {
		t.Fatalf("ListOffered: %v", err)
	}
	var before, after int
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		before, err = repo.Count(ctx, domain.PresetOriginWorkspace, "")
		return err
	}); err != nil {
		t.Fatalf("Count: %v", err)
	}
	insertPreset(ctx, t, tenantA, savedPresetIn(t, domain.PresetOriginWorkspace, ""))
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		after, err = repo.Count(ctx, domain.PresetOriginWorkspace, "")
		return err
	}); err != nil {
		t.Fatalf("Count: %v", err)
	}
	if before != after {
		t.Fatalf("tenant B counted tenant A's preset: %d then %d", before, after)
	}

	changed := preset
	changed.Name = freshName(t)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return repo.Update(ctx, changed, 1, created)
	}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("Update from tenant B answered %v, want nothing written", err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return repo.Delete(ctx, preset.ID, 1)
	}); !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("Delete from tenant B answered %v, want nothing removed", err)
	}
	// Insert into tenant B with tenant A's owner: the composite key refuses an account of another
	// tenant.
	foreign := savedPresetIn(t, domain.PresetOriginOwn, authorA)
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return repo.Insert(ctx, foreign, created)
	}); err == nil {
		t.Fatalf("Insert in tenant B with tenant A's account as owner was accepted")
	}

	var still domain.Preset
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		still, err = repo.Find(ctx, preset.ID)
		return err
	}); err != nil || still.Name != preset.Name || still.Version != 1 {
		t.Fatalf("tenant A's preset after tenant B's attempts: %+v, %v", still, err)
	}
}
