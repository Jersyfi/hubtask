// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

func newWorkspaceStructureHarness(t *testing.T) (*structureHarness, *usecase.Registry) {
	t.Helper()
	h := newStructureHarness(t)
	set := SetWorkspaceStructure{
		Structure: h.set.Structure, Authorizer: h.authorizer, Changes: h.changes, Epochs: h.epochs,
		Audit: h.audit, UnitOfWork: h.uow, Clock: clock.Fixed(now), HLC: &hlcSource{},
	}
	registry, err := usecase.NewRegistry(nil, set.Descriptor(), h.set.Descriptor())
	if err != nil {
		t.Fatalf("the catalogue refused the entries: %v", err)
	}
	return h, registry
}

// UC-WRK-23 check 7's writer, and check 8's refusal: an administrator locks dates on for every
// collection; the lock is answered with who set it, binds every collection below, and a collection
// asking to switch dates off is refused.
func TestAWorkspaceLockBindsEveryCollection(t *testing.T) {
	h, registry := newWorkspaceStructureHarness(t)
	ctx := context.Background()

	out, err := registry.Invoke(ctx, SetWorkspaceStructureName, actor(), usecase.Input{
		"settings": map[string]any{"celebration": "SUBTLE"},
		"locks":    []any{"fields.TASK.DUE_DATE", "celebration"},
	})
	if err != nil {
		t.Fatalf("locking failed: %v", err)
	}
	locks, _ := out["locks"].(map[string]any)
	dates, _ := locks["fields.TASK.DUE_DATE"].(map[string]any)
	if dates["origin"] != "WORKSPACE" || dates["locked_by"] != accountID.String() || out["version"] != 2 {
		t.Errorf("answered %v", out)
	}
	if sourceOf(out, "celebration") != "WORKSPACE" {
		t.Errorf("sources %v", out["sources"])
	}
	// Every container below was told its structure in force.
	if records := structureRecords(h); len(records) != 3 {
		t.Errorf("%d structure records, want the hub and its two collections", len(records))
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != WorkspaceStructureUpdatedAction {
		t.Errorf("audit %+v", h.audit.entries)
	}

	_, err = registry.Invoke(ctx, SetContainerStructureName, actor(), usecase.Input{
		"container_id": collectionID.String(),
		"settings": map[string]any{"fields": map[string]any{
			"TASK": map[string]any{"capabilities": []any{"COMPLETION"}},
		}},
	})
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "structure.setting_locked" || refusal.Params["setting"] != "fields.TASK.DUE_DATE" {
		t.Errorf("switching dates off was answered %v", err)
	}
}

// A lock kept keeps who set it; a new one is the actor's; an empty list removes them all.
func TestALockKeepsWhoSetIt(t *testing.T) {
	h, registry := newWorkspaceStructureHarness(t)
	h.store.workspace.Locks = map[domain.LockKey]shared.ID{"views": colleagueAccountID}

	if _, err := registry.Invoke(context.Background(), SetWorkspaceStructureName, actor(), usecase.Input{
		"locks": []any{"views", "counts"},
	}); err != nil {
		t.Fatalf("failed: %v", err)
	}
	if locks := h.store.workspace.Locks; locks["views"] != colleagueAccountID || locks["counts"] != accountID {
		t.Errorf("locks %v", locks)
	}

	if _, err := registry.Invoke(context.Background(), SetWorkspaceStructureName, actor(), usecase.Input{
		"locks": []any{},
	}); err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(h.store.workspace.Locks) != 0 {
		t.Errorf("locks %v", h.store.workspace.Locks)
	}
}

func TestTheDefaultPresetIsSetAndCleared(t *testing.T) {
	h, registry := newWorkspaceStructureHarness(t)
	ctx := context.Background()

	if _, err := registry.Invoke(ctx, SetWorkspaceStructureName, actor(), usecase.Input{"default_preset": "TASKS"}); err != nil {
		t.Fatalf("failed: %v", err)
	}
	if h.store.workspace.DefaultPreset != domain.PresetTasks {
		t.Errorf("default %q", h.store.workspace.DefaultPreset)
	}
	// The default is not a value anything inherits: no container is told anything new.
	if records := structureRecords(h); len(records) != 0 {
		t.Errorf("%d structure records for a default preset", len(records))
	}
	if _, err := registry.Invoke(ctx, SetWorkspaceStructureName, actor(), usecase.Input{"clear_default_preset": true}); err != nil {
		t.Fatalf("failed: %v", err)
	}
	if h.store.workspace.DefaultPreset != "" {
		t.Errorf("default %q", h.store.workspace.DefaultPreset)
	}
}

func TestTheWorkspaceIsHeldToTheInstallationAndItsVersion(t *testing.T) {
	h, registry := newWorkspaceStructureHarness(t)
	ctx := context.Background()

	_, err := registry.Invoke(ctx, SetWorkspaceStructureName, actor(), usecase.Input{
		"settings": map[string]any{"automation": []any{"DONE_COLUMN_COMPLETES"}},
	})
	if !errors.Is(err, shared.ErrCapabilityNotSupported) {
		t.Errorf("a widening was answered %v", err)
	}
	_, err = registry.Invoke(ctx, SetWorkspaceStructureName, actor(), usecase.Input{"locks": []any{"fields.ACTIVITY.NOTES"}})
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "structure.lock_unknown" {
		t.Errorf("a lock on nothing was answered %v", err)
	}
	_, err = registry.Invoke(ctx, SetWorkspaceStructureName, actor(), usecase.Input{
		"default_preset": "TASKS", "expected_version": 7,
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version was answered %v", err)
	}
	if len(h.store.workspaces) != 0 || len(h.audit.entries) != 0 {
		t.Error("a refused change was written")
	}

	h.authorizer.err = shared.ErrForbidden.WithDetail("access.denied")
	if _, err := registry.Invoke(ctx, SetWorkspaceStructureName, actor(), usecase.Input{"default_preset": "TASKS"}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a member was answered %v", err)
	}
	request := h.authorizer.requests[len(h.authorizer.requests)-1]
	if request.TokenScope != "workspace:manage" || request.Action != WorkspaceStructureUpdatedAction {
		t.Errorf("asked %+v", request)
	}
}

func TestTheWorkspaceStructureDescriptorIsAWriteWithoutStepUp(t *testing.T) {
	descriptor := SetWorkspaceStructure{}.Descriptor()
	if descriptor.ReadOnly || descriptor.Destructive || descriptor.StepUp != "" || !descriptor.Audit.Required {
		t.Errorf("descriptor %+v", descriptor)
	}
}
