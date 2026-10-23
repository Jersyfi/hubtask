// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"fmt"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
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

// EntriesBelow walks the stored entries under each of ids, as the recursive statement does.
func (i *items) EntriesBelow(_ context.Context, ids []shared.ID) ([]repository.EntriesBelow, error) {
	groups := []repository.EntriesBelow{}
	for _, under := range ids {
		counts := map[string]int{}
		seenOf := map[string][]domain.ItemType{}
		var walk func(parent shared.ID, seen []domain.ItemType)
		walk = func(parent shared.ID, seen []domain.ItemType) {
			for _, item := range i.stored {
				if item.ParentID != parent || item.DeletedAt != nil || item.ArchivedAt != nil {
					continue
				}
				next := slices.Clone(seen)
				if !slices.Contains(next, item.Type) {
					next = append(next, item.Type)
				}
				slices.Sort(next)
				key := fmt.Sprint(next)
				counts[key]++
				seenOf[key] = next
				walk(item.ID, next)
			}
		}
		walk(under, nil)
		for key, entries := range counts {
			groups = append(groups, repository.EntriesBelow{Under: under, Seen: seenOf[key], Entries: entries})
		}
	}
	return groups, nil
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

// UC-WRK-24 check 4: a shown entry says how many entries under it its collection hides, so nobody
// believes them lost; an entry hidden itself and an entry in a collection offering every level say
// none.
func TestAShownEntrySaysHowManyEntriesBelowItAreHidden(t *testing.T) {
	store, containerStore := readFixture()
	task := store.stored[readItemID]
	packageID := shared.MustParseID("0192f000-0000-7000-8000-000000000313")
	stepID := shared.MustParseID("0192f000-0000-7000-8000-000000000314")
	callID := shared.MustParseID("0192f000-0000-7000-8000-000000000315")
	gone := now
	pack := domain.WorkItem{
		ID: packageID, TenantID: tenantID, CollectionID: readCollectionID, Type: domain.ItemWorkPackage,
		ParentID: readItemID, Path: task.ChildPath(packageID), Depth: 2, Title: "Pack", OrderKey: "a0",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	store.stored[packageID] = pack
	store.stored[stepID] = domain.WorkItem{
		ID: stepID, TenantID: tenantID, CollectionID: readCollectionID, Type: domain.ItemActivity,
		ParentID: packageID, Path: pack.ChildPath(stepID), Depth: 3, Title: "Step", OrderKey: "a0",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	store.stored[callID] = domain.WorkItem{
		ID: callID, TenantID: tenantID, CollectionID: readCollectionID, Type: domain.ItemActivity,
		ParentID: readItemID, Path: task.ChildPath(callID), Depth: 2, Title: "Call", OrderKey: "a1",
		CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, Version: 1, DeletedAt: &gone,
	}
	read := GetWorkItem{
		Items: store, Containers: containerStore, Authorizer: &authorizer{}, UnitOfWork: &unitOfWork{},
		Gate: gateOver(store, containerStore),
	}
	below := func(id shared.ID) any {
		t.Helper()
		out, err := read.Descriptor().Handler.Invoke(t.Context(), actorFixture(), usecase.Input{"item_id": id.String()})
		if err != nil {
			t.Fatalf("reading %s: %v", id, err)
		}
		return out["hidden"].(map[string]any)["below"]
	}

	if got := below(readItemID); got != 0 {
		t.Errorf("a collection offering every level hides %v under the task, want 0", got)
	}
	narrowTo(t, containerStore, readCollectionID, domain.PresetList)
	if got := below(readItemID); got != 2 {
		t.Errorf("List hides %v under the task, want 2 (the work package and its activity; the trashed one is not counted)", got)
	}
	if got := below(packageID); got != 0 {
		t.Errorf("a hidden work package says %v below it, want 0", got)
	}
	widen(containerStore, readCollectionID)
	if got := below(readItemID); got != 0 {
		t.Errorf("widened again, the task says %v hidden below it, want 0", got)
	}
}
