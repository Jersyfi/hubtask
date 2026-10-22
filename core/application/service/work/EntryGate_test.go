// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// TypesOf answers the types of the stored entries among ids, for the gate's ancestor chains.
func (i *items) TypesOf(_ context.Context, ids []shared.ID) (map[shared.ID]domain.ItemType, error) {
	types := map[shared.ID]domain.ItemType{}
	for _, id := range ids {
		if item, found := i.stored[id]; found {
			types[id] = item.Type
		}
	}
	return types, nil
}

// matrixProfiles is the installation's matrix as domain-model.md §2 ships it, every capability of
// every type: what the presets narrow.
func matrixProfiles() []domain.CapabilityProfile {
	return []domain.CapabilityProfile{
		{Type: domain.ItemTask, Capabilities: []domain.Capability{
			domain.CapabilityCompletion, domain.CapabilityDueDate, domain.CapabilityReminder,
			domain.CapabilityAssignment, domain.CapabilityMembers, domain.CapabilityBucket,
			domain.CapabilityNotes, domain.CapabilityLabels, domain.CapabilityComments,
			domain.CapabilityCover, domain.CapabilityAttachments, domain.CapabilityHistory,
			domain.CapabilityRecurrence, domain.CapabilityCustomFields,
		}, AllowedChildTypes: []domain.ItemType{domain.ItemWorkPackage, domain.ItemActivity}, MaxDepth: 3},
		{Type: domain.ItemWorkPackage, Capabilities: []domain.Capability{
			domain.CapabilityCompletion, domain.CapabilityDueDate, domain.CapabilityReminder,
			domain.CapabilityAssignment, domain.CapabilityMembers, domain.CapabilityNotes,
			domain.CapabilityLabels, domain.CapabilityComments, domain.CapabilityAttachments,
			domain.CapabilityHistory, domain.CapabilityCustomFields,
		}, AllowedChildTypes: []domain.ItemType{domain.ItemActivity}, MaxDepth: 2},
		{Type: domain.ItemActivity, Capabilities: []domain.Capability{
			domain.CapabilityCompletion, domain.CapabilityDueDate, domain.CapabilityReminder,
			domain.CapabilityAssignment, domain.CapabilityHistory,
		}, MaxDepth: 1},
	}
}

// narrowTo gives a stored collection the layer of one of Hubtask's presets, as choosing it would.
func narrowTo(t *testing.T, store *containers, collectionID shared.ID, key domain.PresetKey) {
	t.Helper()
	bound := domain.NewStructureBound(matrixProfiles(), matrixProfiles())
	preset, err := domain.FindPreset(key, bound)
	if err != nil {
		t.Fatalf("the preset %s: %v", key, err)
	}
	collection := store.stored[collectionID]
	collection.Structure = domain.StructureLayer{Preset: key, Values: preset.Values}
	store.stored[collectionID] = collection
}

// widen takes the collection's own layer away: it offers what its hub and the installation do.
func widen(store *containers, collectionID shared.ID) {
	collection := store.stored[collectionID]
	collection.Structure = domain.StructureLayer{}
	store.stored[collectionID] = collection
}

// gateOver is the gate over the fakes, with the full matrix as the installation's profile. The
// collections' hub is stored, since a collection's structure is resolved through it.
func gateOver(store *items, containerStore *containers) EntryGate {
	if _, held := containerStore.stored[hubID]; !held {
		containerStore.stored[hubID] = hubFixture(hubID, "Home", "a0")
	}
	return EntryGate{
		Structure: StructureResolver{
			Containers: containerStore, Structures: newStructureStore(containerStore),
			Profiles: &profiles{rows: matrixProfiles(), system: matrixProfiles()},
		},
		Types: store,
	}
}

func hiddenOf(t *testing.T, out usecase.Output) (bool, []string) {
	t.Helper()
	hidden, marked := out["hidden"].(map[string]any)
	if !marked {
		t.Fatalf("the answer carries no mark: %v", out)
	}
	level, _ := hidden["level"].(bool)
	capabilities, _ := hidden["capabilities"].([]string)
	return level, capabilities
}

// UC-WRK-24 check 3: a read returns everything stored and marks what the collection hides.
func TestAReadMarksWhatTheCollectionHides(t *testing.T) {
	store, containerStore := readFixture()
	packageID := shared.MustParseID("0192f000-0000-7000-8000-000000000303")
	task := store.stored[readItemID]
	store.stored[packageID] = domain.WorkItem{
		ID: packageID, TenantID: tenantID, CollectionID: readCollectionID, Type: domain.ItemWorkPackage,
		ParentID: readItemID, Path: task.ChildPath(packageID), Depth: 2, Title: "Pack", OrderKey: "a0",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	narrowTo(t, containerStore, readCollectionID, domain.PresetList)
	read := GetWorkItem{
		Items: store, Containers: containerStore, Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
		Gate: gateOver(store, containerStore),
	}

	out, err := read.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{
		"item_id": readItemID.String(),
	})
	if err != nil {
		t.Fatalf("reading the task: %v", err)
	}
	level, capabilities := hiddenOf(t, out)
	if level {
		t.Error("a task is hidden in List")
	}
	want := []string{"DUE_DATE", "REMINDER", "ASSIGNMENT", "MEMBERS", "BUCKET", "NOTES", "LABELS",
		"COMMENTS", "COVER", "ATTACHMENTS", "RECURRENCE"}
	if !slices.Equal(capabilities, want) {
		t.Errorf("hidden %v, want %v", capabilities, want)
	}
	if out["title"] != "Buy milk" {
		t.Errorf("the entry itself changed: %v", out["title"])
	}

	out, err = read.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{
		"item_id": packageID.String(),
	})
	if err != nil {
		t.Fatalf("reading the work package: %v", err)
	}
	if level, _ := hiddenOf(t, out); !level {
		t.Error("a work package is shown in List")
	}

	widen(containerStore, readCollectionID)
	out, err = read.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{
		"item_id": packageID.String(),
	})
	if err != nil {
		t.Fatalf("reading after widening: %v", err)
	}
	if level, capabilities := hiddenOf(t, out); level || len(capabilities) != 0 {
		t.Errorf("widened, the work package is still marked: %v %v", level, capabilities)
	}
}

func TestAListMarksEveryEntryOfThePage(t *testing.T) {
	store, containerStore := readFixture()
	store.page.Items = []domain.WorkItem{store.stored[readItemID]}
	narrowTo(t, containerStore, readCollectionID, domain.PresetTasks)
	list := ListWorkItems{
		Items: store, Containers: containerStore, Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
		Gate: gateOver(store, containerStore),
	}
	out, err := list.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{
		"collection_id": readCollectionID.String(),
	})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 1 {
		t.Fatalf("rows %v", out["data"])
	}
	if _, capabilities := hiddenOf(t, rows[0]); !slices.Equal(capabilities, []string{"MEMBERS", "BUCKET", "COVER"}) {
		t.Errorf("hidden %v, want Tasks' members, column and cover", capabilities)
	}
}

func TestAnUnwiredGateMarksNothing(t *testing.T) {
	store, containerStore := readFixture()
	out, err := GetWorkItem{
		Items: store, Containers: containerStore, Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
	}.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{"item_id": readItemID.String()})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if _, marked := out["hidden"]; marked {
		t.Errorf("a build without the gate marked %v", out["hidden"])
	}
}
