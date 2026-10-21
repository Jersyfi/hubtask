// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// epochs counts the advances a fan-out too large to write row by row asks for.
type epochs struct{ advanced int }

func (e *epochs) Current(context.Context) (int64, error) { return int64(e.advanced), nil }
func (e *epochs) Advance(context.Context) (int64, error) {
	e.advanced++
	return int64(e.advanced), nil
}

var _ changelog.Epochs = (*epochs)(nil)

const otherCollectionID = shared.ID("0192f000-0000-7000-8000-0000000000c2")

type structureHarness struct {
	*harness
	store    *structureStore
	epochs   *epochs
	set      SetContainerStructure
	registry *usecase.Registry
}

func newStructureHarness(t *testing.T) *structureHarness {
	t.Helper()
	h := newHarness()
	store := h.withStructures()
	hub := h.withHub()
	list, _ := domain.FindPreset(domain.PresetList, domain.NewStructureBound(seededProfiles(), seededProfiles()))
	hub.Structure = domain.StructureLayer{Preset: domain.PresetList, Values: list.Values}
	h.containers.stored[hubID] = hub
	// One collection follows the hub; the other has decided its views itself.
	h.containers.stored[collectionID] = domain.Container{
		ID: collectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Groceries", OrderKey: "a0", CreatedBy: accountID, CreatedAt: now, Version: 1,
	}
	h.containers.stored[otherCollectionID] = domain.Container{
		ID: otherCollectionID, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID,
		Name: "Hardware", OrderKey: "a1", CreatedBy: accountID, CreatedAt: now, Version: 3,
		Structure: domain.StructureLayer{Values: domain.StructureValues{Views: &domain.Views{
			Offered: []domain.ViewLayout{domain.ViewKanban}, Default: domain.ViewKanban,
		}}},
	}

	counted := &epochs{}
	set := SetContainerStructure{
		Writer: ContainerWriter{
			Containers: h.containers, Authorizer: h.authorizer, Events: h.events, Changes: h.changes,
			Audit: h.audit, UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &ids{}, HLC: &hlcSource{},
		},
		Structure: h.handler.Structure,
		Epochs:    counted,
	}
	registry, err := usecase.NewRegistry(nil, set.Descriptor())
	if err != nil {
		t.Fatalf("the catalogue refused the entry: %v", err)
	}
	return &structureHarness{harness: h, store: store, epochs: counted, set: set, registry: registry}
}

func (h *structureHarness) invoke(t *testing.T, in usecase.Input) (usecase.Output, error) {
	t.Helper()
	return h.registry.Invoke(context.Background(), SetContainerStructureName, actor(), in)
}

func structureRecords(h *structureHarness) map[shared.ID]map[string]any {
	records := map[shared.ID]map[string]any{}
	for _, change := range h.changes.recorded {
		if change.Field == structureField {
			records[change.EntityID], _ = change.Payload[structureField].(map[string]any)
		}
	}
	return records
}

// UC-WRK-23 check 2: one setting changed on a hub; every other inherited, the collection that
// follows the hub follows, the one with its own value keeps it - and offline clients learn each.
func TestOneSettingOfAHubChangesOnItsOwnAndReachesItsCollections(t *testing.T) {
	h := newStructureHarness(t)

	out, err := h.invoke(t, usecase.Input{
		"container_id": hubID.String(),
		"settings": map[string]any{
			"views": map[string]any{"offered": []any{"LIST_COLLAPSED", "KANBAN"}, "default": "KANBAN"},
		},
	})
	if err != nil {
		t.Fatalf("the change failed: %v", err)
	}

	hub := h.containers.stored[hubID]
	if hub.Version != 2 || hub.Structure.Values.Views.Default != domain.ViewKanban ||
		hub.Structure.Values.Counts.Figure != domain.FigureOpen {
		t.Errorf("the hub stored %+v", hub.Structure)
	}
	structure, _ := out["structure"].(map[string]any)
	if sourceOf(structure, "views") != "HUB" || sourceOf(structure, "counts") != "HUB" {
		t.Errorf("answered %v", structure)
	}

	records := structureRecords(h)
	for _, id := range []shared.ID{hubID, collectionID, otherCollectionID} {
		if records[id] == nil {
			t.Errorf("offline clients were not told %s's structure", id)
		}
	}
	if sourceOf(records[collectionID], "views") != "HUB" || sourceOf(records[otherCollectionID], "views") != "COLLECTION" {
		t.Errorf("follower %v, own %v", records[collectionID]["sources"], records[otherCollectionID]["sources"])
	}

	if len(h.events.appended) != 1 {
		t.Fatalf("%d events", len(h.events.appended))
	}
	changeSet, _ := h.events.appended[0].Payload["change_set"].(map[string]any)
	moved, _ := changeSet["structure"].(map[string]any)
	if len(moved) != 1 || moved["views"] == nil {
		t.Errorf("the change set is %v", changeSet)
	}
	if len(h.audit.entries) != 1 || h.audit.entries[0].Action != ContainerStructureUpdatedAction {
		t.Errorf("audit %+v", h.audit.entries)
	}
}

// UC-WRK-23 check 5: a widening is refused as capability_not_supported, and nothing is written.
func TestAWideningIsRefusedAndWritesNothing(t *testing.T) {
	cases := map[string]map[string]any{
		"a capability the type never had": {"fields": map[string]any{
			"ACTIVITY": map[string]any{"capabilities": []any{"COMPLETION", "NOTES"}},
		}},
		"something to run by itself": {"automation": []any{"DONE_COLUMN_COMPLETES"}},
		"a level the installation lacks": {"levels": []any{
			map[string]any{"type": "TASK"}, map[string]any{"type": "MILESTONE"},
		}},
	}
	for name, settings := range cases {
		t.Run(name, func(t *testing.T) {
			h := newStructureHarness(t)
			_, err := h.invoke(t, usecase.Input{"container_id": collectionID.String(), "settings": settings})
			if !errors.Is(err, shared.ErrCapabilityNotSupported) || shared.AsError(err).DetailCode != "structure.widens" {
				t.Fatalf("refused as %v", err)
			}
			if len(h.store.written) != 0 || len(h.audit.entries) != 0 || len(h.events.appended) != 0 {
				t.Error("a refused change left something behind")
			}
		})
	}
}

// UC-WRK-23 check 6: a level's name is stored as the person wrote it, and the trail hashes it.
func TestALevelsNameIsStoredAndTheTrailHashesIt(t *testing.T) {
	h := newStructureHarness(t)
	name := "Einkaufsposten"

	_, err := h.invoke(t, usecase.Input{
		"container_id": collectionID.String(),
		"settings":     map[string]any{"levels": []any{map[string]any{"type": "TASK", "name": "  " + name + " "}}},
	})
	if err != nil {
		t.Fatalf("the change failed: %v", err)
	}
	if got := h.containers.stored[collectionID].Structure.Values.Levels; len(got) != 1 || got[0].Name != name {
		t.Errorf("stored %+v", got)
	}
	if len(h.audit.entries) != 1 {
		t.Fatalf("audit %+v", h.audit.entries)
	}
	recorded := fmt.Sprint(h.audit.entries[0].Changes)
	if strings.Contains(recorded, name) {
		t.Errorf("the name reached the trail in clear: %s", recorded)
	}
	if !strings.Contains(recorded, "structure.levels") {
		t.Errorf("the trail does not say the levels moved: %s", recorded)
	}
}

// A lock binds: setting a locked value is refused; choosing a preset under it keeps the locked
// value and says so (S4).
func TestALockRefusesTheSettingAndKeepsItUnderAPreset(t *testing.T) {
	h := newStructureHarness(t)
	h.store.workspace = domain.WorkspaceStructure{
		Layer: domain.StructureLayer{Values: domain.StructureValues{Celebration: domain.CelebrationAll}},
		Locks: map[domain.LockKey]shared.ID{"celebration": accountID},
	}

	_, err := h.invoke(t, usecase.Input{
		"container_id": collectionID.String(), "settings": map[string]any{"celebration": "SUBTLE"},
	})
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "structure.setting_locked" ||
		refusal.Params["origin"] != "WORKSPACE" || refusal.Params["setting"] != "celebration" {
		t.Fatalf("refused as %v", err)
	}

	out, err := h.invoke(t, usecase.Input{"container_id": collectionID.String(), "preset": "LIST"})
	if err != nil {
		t.Fatalf("the preset was refused: %v", err)
	}
	if kept, _ := out["kept_by_lock"].([]any); len(kept) != 1 || kept[0] != "celebration" {
		t.Errorf("kept_by_lock = %v", out["kept_by_lock"])
	}
	if h.containers.stored[collectionID].Structure.Values.Has(domain.SettingCelebration) {
		t.Error("the locked setting was copied into the collection")
	}
}

// S1: a reset is asked for by name, and a collection's reset completion is its stored key removed.
func TestAResetGoesBackToTheLayerAbove(t *testing.T) {
	h := newStructureHarness(t)
	other := h.containers.stored[otherCollectionID]
	other.Structure.Values.Completion = domain.CompletionRollup
	h.containers.stored[otherCollectionID] = other

	out, err := h.invoke(t, usecase.Input{
		"container_id": otherCollectionID.String(), "reset": []any{"views", "completion"},
	})
	if err != nil {
		t.Fatalf("the reset failed: %v", err)
	}
	stored := h.containers.stored[otherCollectionID]
	if !stored.Structure.IsEmpty() || stored.CompletionPolicy != domain.CompletionManual {
		t.Errorf("stored %+v, completion %s", stored.Structure, stored.CompletionPolicy)
	}
	structure, _ := out["structure"].(map[string]any)
	if sourceOf(structure, "views") != "HUB" || sourceOf(structure, "completion") != "HUB" {
		t.Errorf("answered %v", structure["sources"])
	}

	_, err = h.invoke(t, usecase.Input{"container_id": otherCollectionID.String(), "reset": []any{"colour"}})
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "structure.setting_invalid" {
		t.Errorf("an unknown setting was reset: %v", err)
	}
}

func TestWritingWhatIsStoredWritesNothing(t *testing.T) {
	h := newStructureHarness(t)
	_, err := h.invoke(t, usecase.Input{
		"container_id": otherCollectionID.String(),
		"settings":     map[string]any{"views": map[string]any{"offered": []any{"KANBAN"}, "default": "KANBAN"}},
	})
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(h.store.written) != 0 || len(h.events.appended) != 0 || len(h.audit.entries) != 0 {
		t.Error("an unchanged structure was written")
	}
}

func TestAStaleVersionAndARefusedRoleWriteNothing(t *testing.T) {
	h := newStructureHarness(t)
	_, err := h.invoke(t, usecase.Input{
		"container_id": otherCollectionID.String(), "reset": []any{"views"}, "expected_version": 2,
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Errorf("a stale version was answered %v", err)
	}

	h.authorizer.err = shared.ErrForbidden.WithDetail("access.denied")
	_, err = h.invoke(t, usecase.Input{"container_id": otherCollectionID.String(), "reset": []any{"views"}})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a refused role was answered %v", err)
	}
	if h.authorizer.requests[len(h.authorizer.requests)-1].Action != ContainerStructureUpdatedAction {
		t.Errorf("asked %+v", h.authorizer.requests)
	}
	if len(h.store.written) != 0 {
		t.Error("something was written")
	}
}

// N6: a fan-out past the ceiling advances the workspace's epoch rather than writing a row per heir.
func TestAFanOutPastTheCeilingAdvancesTheEpoch(t *testing.T) {
	h := newStructureHarness(t)
	for i := range structureFanOutCeiling {
		id := shared.ID(fmt.Sprintf("0192f000-0000-7000-8000-%012x", 0x100000+i))
		h.containers.stored[id] = domain.Container{
			ID: id, TenantID: tenantID, Type: domain.ContainerCollection, ParentID: hubID, Version: 1,
		}
	}

	if _, err := h.invoke(t, usecase.Input{"container_id": hubID.String(), "reset": []any{"views"}}); err != nil {
		t.Fatalf("failed: %v", err)
	}
	if h.epochs.advanced != 1 {
		t.Errorf("the epoch advanced %d times", h.epochs.advanced)
	}
	if records := structureRecords(h); len(records) != 1 || records[hubID] == nil {
		t.Errorf("%d structure records, want the hub's alone", len(records))
	}
}

func TestTheDescriptorDeclaresAWriteThatIsNotDestructive(t *testing.T) {
	descriptor := SetContainerStructure{}.Descriptor()
	if descriptor.ReadOnly || descriptor.Destructive || !descriptor.Audit.Required ||
		descriptor.MCPTool() != "set_container_structure" || descriptor.TokenScope != containersWrite {
		t.Errorf("descriptor %+v", descriptor)
	}
	if !slices.ContainsFunc(descriptor.Input, func(f usecase.Field) bool { return f.Name == "reset" }) {
		t.Error("no reset")
	}
}
