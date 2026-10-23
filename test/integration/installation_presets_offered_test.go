// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The installation's presets as every workspace meets them, through the catalogue against
// PostgreSQL (UC-INS-18 checks 2 and 3, UC-WRK-30 check 2): offered in every workspace with origin
// INSTALLATION, chosen by id, withdrawn while what was made keeps its settings; the default a hub
// starts from where its workspace chose none, and the default's locks binding every layer.

func kanbanPreset(t *testing.T) domain.InstallationPreset {
	t.Helper()
	preset := installationPresetIn(t)
	preset.Values = domain.StructureValues{
		Views: &domain.Views{Offered: []domain.ViewLayout{domain.ViewKanban, domain.ViewListCollapsed}, Default: domain.ViewKanban},
	}
	return preset
}

func storeInstallationPreset(ctx context.Context, t *testing.T, preset domain.InstallationPreset) {
	t.Helper()
	presets := postgres.NewInstallationPresetRepository()
	if err := asInstallation(ctx, t, true, func(ctx context.Context) error {
		if preset.IsDefault {
			if err := presets.ClearDefault(ctx, preset.ID, "", created); err != nil {
				return err
			}
		}
		return presets.Insert(ctx, preset, "", created)
	}); err != nil {
		t.Fatalf("storing the installation's preset: %v", err)
	}
	withdrawnAtEnd(ctx, t, preset.ID)
}

func ownerOfB() appshared.ActorContext {
	actor := structureActor(tenantB, authorB)
	actor.Scopes = append(actor.Scopes, "workspace:manage")
	return actor
}

func defaultViewOf(structure map[string]any) string {
	settings, _ := structure["settings"].(map[string]any)
	views, _ := settings["views"].(map[string]any)
	view, _ := views["default"].(string)
	return view
}

func TestTheInstallationsPresetsAreOfferedInEveryWorkspaceAndChosenByID(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := structureCatalogue(t)
	preset := kanbanPreset(t)
	storeInstallationPreset(ctx, t, preset)

	for _, actor := range []appshared.ActorContext{presetAdmin(), ownerOfB()} {
		if origin := offeredTo(ctx, t, registry, actor)[preset.ID.String()]; origin != "INSTALLATION" {
			t.Errorf("workspace %s is offered the installation's preset as %q", actor.TenantID, origin)
		}
	}

	hub := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t), "preset_id": preset.ID.String()})
	made := structureOf(ctx, t, registry, hub)
	if defaultViewOf(made) != "KANBAN" || made["preset_id"] != preset.ID.String() {
		t.Fatalf("a hub from the installation's preset: %v", made)
	}

	// Withdrawn, it is offered no more and what was made from it keeps its settings.
	if err := asInstallation(ctx, t, true, func(ctx context.Context) error {
		return postgres.NewInstallationPresetRepository().Delete(ctx, preset.ID, nil)
	}); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	if _, offered := offeredTo(ctx, t, registry, presetAdmin())[preset.ID.String()]; offered {
		t.Error("a withdrawn preset is still offered")
	}
	if defaultViewOf(structureOf(ctx, t, registry, hub)) != "KANBAN" {
		t.Error("the hub lost the withdrawn preset's settings")
	}
	if _, err := registry.Invoke(ctx, "CreateContainer", presetAdmin(), usecase.Input{
		"type": "HUB", "name": freshName(t), "preset_id": preset.ID.String(),
	}); detailOf(err) != "presets.unknown" {
		t.Errorf("choosing a withdrawn preset answered %v", err)
	}
}

func TestTheInstallationsDefaultStartsAHubAndItsLocksBindEveryLayer(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)
	registry := structureCatalogue(t)
	owner := ownerOfB()
	if _, err := registry.Invoke(ctx, "SetWorkspaceStructure", owner, usecase.Input{"default_preset": nil, "locks": []any{}}); err != nil {
		t.Fatalf("clearing tenant B's own default: %v", err)
	}
	preset := kanbanPreset(t)
	preset.IsDefault, preset.Locks = true, []domain.LockKey{"views"}
	storeInstallationPreset(ctx, t, preset)

	// A hub created without a preset, in a workspace without a default of its own, starts from the
	// installation's default.
	out, err := registry.Invoke(ctx, "CreateContainer", owner, usecase.Input{"type": "HUB", "name": freshName(t)})
	if err != nil {
		t.Fatalf("creating a hub: %v", err)
	}
	structure, _ := out["structure"].(map[string]any)
	if structure["preset_id"] != preset.ID.String() || defaultViewOf(structure) != "KANBAN" {
		t.Errorf("a hub without a preset: %v", structure)
	}
	locks, _ := structure["locks"].(map[string]any)
	lock, _ := locks["views"].(map[string]any)
	if lock["origin"] != "INSTANCE" || lock["locked_by"] != nil {
		t.Errorf("the installation's lock reads %v", lock)
	}

	listOnly := map[string]any{"views": map[string]any{"offered": []any{"LIST_COLLAPSED"}, "default": "LIST_COLLAPSED"}}
	if _, err := registry.Invoke(ctx, "SetContainerStructure", owner, usecase.Input{
		"container_id": out.String("id"), "settings": listOnly,
	}); detailOf(err) != "structure.setting_locked" {
		t.Errorf("a hub's own views under the installation's lock answered %v", err)
	}
	if _, err := registry.Invoke(ctx, "SetWorkspaceStructure", owner, usecase.Input{"settings": listOnly}); detailOf(err) != "structure.setting_locked" {
		t.Errorf("the workspace's own views under the installation's lock answered %v", err)
	}
	if _, err := registry.Invoke(ctx, "SetWorkspaceStructure", owner, usecase.Input{"locks": []any{"views"}}); detailOf(err) != "structure.setting_locked" {
		t.Errorf("the workspace locking what the installation locks answered %v", err)
	}
	if _, err := registry.Invoke(ctx, "SetWorkspaceStructure", owner, usecase.Input{"default_preset_id": preset.ID.String()}); detailOf(err) != "presets.unknown" {
		t.Errorf("the installation's preset as the workspace's own default answered %v", err)
	}

	// Another workspace is bound alike: tenant A's hubs answer the same lock.
	hubA := createIn(ctx, t, registry, usecase.Input{"type": "HUB", "name": freshName(t), "preset": "LIST"})
	locksA, _ := structureOf(ctx, t, registry, hubA)["locks"].(map[string]any)
	if lockA, _ := locksA["views"].(map[string]any); lockA["origin"] != "INSTANCE" {
		t.Errorf("tenant A's hub reads the lock as %v", lockA)
	}
}
