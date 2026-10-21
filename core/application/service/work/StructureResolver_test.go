// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// structureStore is the structure layers as the use cases see them, over the containers fake: what
// was written, what was held, and the workspace's layer with its version.
type structureStore struct {
	containers *containers
	workspace  domain.WorkspaceStructure
	version    int
	holds      []repository.LayerHold
	held       []shared.ID
	written    []domain.Container
	workspaces []domain.WorkspaceStructure
}

func newStructureStore(store *containers) *structureStore {
	return &structureStore{containers: store, version: 1}
}

func (s *structureStore) SetContainerStructure(_ context.Context, container domain.Container, expected int) error {
	stored, found := s.containers.stored[container.ID]
	if !found || stored.Version != expected {
		return shared.ErrVersionConflict.WithDetail("containers.version_conflict")
	}
	container.Version = expected + 1
	s.containers.stored[container.ID] = container
	s.written = append(s.written, container)
	return nil
}

func (s *structureStore) HoldContainer(_ context.Context, id shared.ID) error {
	if _, found := s.containers.stored[id]; !found {
		return shared.ErrNotFound.WithDetail("containers.not_found")
	}
	s.held = append(s.held, id)
	return nil
}

func (s *structureStore) Heirs(_ context.Context, hubID shared.ID, ceiling int) ([]domain.Container, error) {
	var heirs []domain.Container
	for _, container := range s.containers.stored {
		if container.DeletedAt != nil {
			continue
		}
		if hubID.IsZero() || container.ParentID == hubID {
			heirs = append(heirs, container)
		}
	}
	slices.SortFunc(heirs, func(a, b domain.Container) int {
		if a.ID < b.ID {
			return -1
		}
		return 1
	})
	if len(heirs) > ceiling {
		heirs = heirs[:ceiling]
	}
	return heirs, nil
}

func (s *structureStore) Workspace(_ context.Context, hold repository.LayerHold) (domain.WorkspaceStructure, int, error) {
	s.holds = append(s.holds, hold)
	return s.workspace, s.version, nil
}

func (s *structureStore) SetWorkspace(
	_ context.Context, workspace domain.WorkspaceStructure, expected int, _ time.Time,
) error {
	if expected != s.version {
		return shared.ErrVersionConflict.WithDetail("workspace.version_conflict")
	}
	s.workspace, s.version = workspace, s.version+1
	s.workspaces = append(s.workspaces, workspace)
	return nil
}

// seededProfiles is the system matrix as db/migrations/0002 seeds it, with its dates: the
// presets are written against it.
func seededProfiles() []domain.CapabilityProfile {
	return []domain.CapabilityProfile{
		{Type: domain.ItemTask, Capabilities: []domain.Capability{
			domain.CapabilityCompletion, domain.CapabilityDueDate, domain.CapabilityReminder,
			domain.CapabilityAssignment, domain.CapabilityMembers, domain.CapabilityBucket,
			domain.CapabilityNotes, domain.CapabilityLabels, domain.CapabilityComments,
			domain.CapabilityCover, domain.CapabilityAttachments, domain.CapabilityHistory,
			domain.CapabilityRecurrence, domain.CapabilityCustomFields,
		}, AllowedChildTypes: []domain.ItemType{domain.ItemWorkPackage}, MaxDepth: 3},
		{Type: domain.ItemWorkPackage, Capabilities: []domain.Capability{
			domain.CapabilityCompletion, domain.CapabilityDueDate, domain.CapabilityHistory,
		}, AllowedChildTypes: []domain.ItemType{domain.ItemActivity}, MaxDepth: 2},
		{Type: domain.ItemActivity, Capabilities: []domain.Capability{
			domain.CapabilityCompletion, domain.CapabilityDueDate, domain.CapabilityHistory,
		}, MaxDepth: 1},
	}
}

// withStructures wires a resolver into the create harness.
func (h *harness) withStructures() *structureStore {
	store := newStructureStore(h.containers)
	h.handler.Structure = StructureResolver{
		Containers: h.containers, Structures: store, Profiles: &profiles{rows: seededProfiles()},
	}
	return store
}

func structureChange(h *harness) map[string]any {
	for _, change := range h.changes.recorded {
		if change.Field == structureField {
			structure, _ := change.Payload[structureField].(map[string]any)
			return structure
		}
	}
	return nil
}

func sourceOf(structure map[string]any, setting string) string {
	sources, _ := structure["sources"].(map[string]any)
	source, _ := sources[setting].(string)
	return source
}

// A hub made from a preset carries a copy of its values; the answer and the change for offline
// clients both name the structure in force.
func TestAHubMadeFromAPresetCopiesIt(t *testing.T) {
	h := newHarness()
	store := h.withStructures()

	cmd := hubCommand()
	cmd.Preset = domain.PresetList
	created, structure, err := h.handler.create(context.Background(), actor(), cmd)
	if err != nil {
		t.Fatalf("creating failed: %v", err)
	}
	if created.Structure.Preset != domain.PresetList || created.Structure.Values.Views == nil ||
		created.Structure.Values.Views.Default != domain.ViewListCollapsed {
		t.Errorf("the hub's own layer is %+v", created.Structure)
	}
	if structure == nil || structure.SourceOf(domain.SettingLevels) != "HUB" {
		t.Fatalf("the structure in force is %+v", structure)
	}
	recorded := structureChange(h)
	if recorded == nil || sourceOf(recorded, "views") != "HUB" || recorded["preset"] != "LIST" {
		t.Errorf("offline clients were told %v", recorded)
	}
	// The workspace's row is held while the new layer is resolved from it.
	if !slices.Contains(store.holds, repository.LayerShared) {
		t.Errorf("holds %v", store.holds)
	}
}

// B2: a hub made without a preset starts from the workspace's default.
func TestAHubWithoutAPresetStartsFromTheWorkspacesDefault(t *testing.T) {
	h := newHarness()
	store := h.withStructures()
	store.workspace.DefaultPreset = domain.PresetTasks

	created, err := h.handler.Execute(context.Background(), actor(), hubCommand())
	if err != nil {
		t.Fatalf("creating failed: %v", err)
	}
	if created.Structure.Preset != domain.PresetTasks {
		t.Errorf("the hub started from %q", created.Structure.Preset)
	}
}

// B2: a collection made with its hub's preset, or with none, stores nothing and follows its hub.
func TestACollectionWithItsHubsPresetFollowsTheHub(t *testing.T) {
	for _, preset := range []domain.PresetKey{"", domain.PresetProject} {
		h := newHarness()
		h.withStructures()
		hub := h.withHub()
		project, _ := domain.FindPreset(domain.PresetProject, domain.NewStructureBound(seededProfiles(), seededProfiles()))
		hub.Structure = domain.StructureLayer{Preset: domain.PresetProject, Values: project.Values}
		h.containers.stored[hubID] = hub

		created, structure, err := h.handler.create(context.Background(), actor(), CreateContainerCommand{
			Type: domain.ContainerCollection, ParentID: hubID, Name: "Garden", Preset: preset,
		})
		if err != nil {
			t.Fatalf("creating with %q failed: %v", preset, err)
		}
		if !created.Structure.IsEmpty() {
			t.Errorf("with %q the collection stored %+v", preset, created.Structure)
		}
		if structure.SourceOf(domain.SettingCompletion) != "HUB" || structure.Values.Completion != domain.CompletionRollup {
			t.Errorf("with %q the collection resolves to %+v", preset, structure)
		}
	}
}

func TestACollectionWithAnotherPresetKeepsItsOwnCopy(t *testing.T) {
	h := newHarness()
	h.withStructures()
	h.withHub()

	created, structure, err := h.handler.create(context.Background(), actor(), CreateContainerCommand{
		Type: domain.ContainerCollection, ParentID: hubID, Name: "Groceries", Preset: domain.PresetList,
	})
	if err != nil {
		t.Fatalf("creating failed: %v", err)
	}
	if created.Structure.Preset != domain.PresetList || structure.SourceOf(domain.SettingLevels) != "COLLECTION" {
		t.Errorf("stored %+v, resolved %+v", created.Structure, structure)
	}
	if got := structure.Values.Levels; len(got) != 1 || got[0].Type != domain.ItemTask {
		t.Errorf("levels %+v", got)
	}
}

// S4: a preset chosen under a workspace lock is accepted, the locked setting kept as locked.
func TestAPresetChosenUnderALockKeepsTheLockedSetting(t *testing.T) {
	h := newHarness()
	store := h.withStructures()
	store.workspace = domain.WorkspaceStructure{
		Locks: map[domain.LockKey]shared.ID{"fields.TASK.DUE_DATE": accountID},
	}

	cmd := hubCommand()
	cmd.Preset = domain.PresetList
	created, structure, err := h.handler.create(context.Background(), actor(), cmd)
	if err != nil {
		t.Fatalf("creating failed: %v", err)
	}
	if !slices.Contains(created.Structure.Values.Fields[domain.ItemTask].Capabilities, domain.CapabilityDueDate) {
		t.Errorf("the list dropped the locked date: %+v", created.Structure.Values.Fields)
	}
	if structure.Locks["fields.TASK.DUE_DATE"].LockedBy != accountID {
		t.Errorf("locks %+v", structure.Locks)
	}
}

func TestAnUnknownPresetIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	h := newHarness()
	h.withStructures()
	cmd := hubCommand()
	cmd.Preset = "SHOPPING"

	_, err := h.handler.Execute(context.Background(), actor(), cmd)
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "presets.unknown" {
		t.Fatalf("refused as %v", err)
	}
	if len(h.containers.inserted) != 0 {
		t.Error("a container was written")
	}

	registry, regErr := usecase.NewRegistry(nil, h.handler.Descriptor())
	if regErr != nil {
		t.Fatal(regErr)
	}
	_, err = registry.Invoke(context.Background(), CreateContainerName, actor(),
		usecase.Input{"type": "HUB", "name": "Private", "preset": "SHOPPING"})
	if err == nil {
		t.Error("the catalogue accepted a preset outside its enum")
	}
}

func TestAPresetWithoutAResolverIsNotSilentlyDropped(t *testing.T) {
	h := newHarness()
	cmd := hubCommand()
	cmd.Preset = domain.PresetList

	_, err := h.handler.Execute(context.Background(), actor(), cmd)
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "containers.structure_unwired" {
		t.Errorf("refused as %v", err)
	}
}

// UC-WRK-23 check 8, the read: every container read answers its structure, an untouched one
// Hubtask's default with every source DEFAULT.
func TestAContainerReadAnswersItsStructure(t *testing.T) {
	h := newHarness()
	h.withHub()
	store := newStructureStore(h.containers)
	reader := actor()
	reader.Scopes = []string{containersRead}
	get := GetContainer{
		Containers: h.containers, Authorizer: h.authorizer, UnitOfWork: h.uow,
		Structure: StructureResolver{Containers: h.containers, Structures: store, Profiles: &profiles{rows: seededProfiles()}},
	}
	registry, err := usecase.NewRegistry(nil, get.Descriptor())
	if err != nil {
		t.Fatal(err)
	}

	out, err := registry.Invoke(context.Background(), GetContainerName, reader, usecase.Input{"container_id": hubID.String()})
	if err != nil {
		t.Fatalf("reading failed: %v", err)
	}
	structure, _ := out["structure"].(map[string]any)
	settings, _ := structure["settings"].(map[string]any)
	if len(settings) != 8 || structure["preset"] != nil {
		t.Fatalf("structure %v", structure)
	}
	for _, setting := range domain.StructureSettingNames() {
		if sourceOf(structure, string(setting)) != "DEFAULT" {
			t.Errorf("%s from %s", setting, sourceOf(structure, string(setting)))
		}
	}
	if locks, _ := structure["locks"].(map[string]any); len(locks) != 0 {
		t.Errorf("locks %v", locks)
	}
}
