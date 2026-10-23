// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

type entryShapes struct{ stored []repository.EntryShape }

func (s entryShapes) ShapesIn(_ context.Context, ids []shared.ID, limit int) ([]repository.EntryShape, error) {
	found := []repository.EntryShape{}
	for _, shape := range s.stored {
		if slices.Contains(ids, shape.CollectionID) && len(found) < limit {
			found = append(found, shape)
		}
	}
	return found, nil
}

// The summer party: the hub is a project; each collection holds a task with a due date, a work
// package under it and an activity under that.
func newPreviewHarness(t *testing.T) (*structureHarness, *usecase.Registry) {
	t.Helper()
	h := newStructureHarness(t)
	project, _ := domain.FindPreset(domain.PresetProject, domain.NewStructureBound(seededProfiles(), seededProfiles()))
	hub := h.containers.stored[hubID]
	hub.Structure = domain.StructureLayer{Preset: domain.PresetProject, Values: project.Values}
	h.containers.stored[hubID] = hub

	shapes := entryShapes{}
	for n, collection := range []shared.ID{collectionID, otherCollectionID} {
		task := shared.MustParseID(fmt.Sprintf("0192f000-0000-7000-8000-0000000a1%d01", n))
		pack := shared.MustParseID(fmt.Sprintf("0192f000-0000-7000-8000-0000000a1%d02", n))
		step := shared.MustParseID(fmt.Sprintf("0192f000-0000-7000-8000-0000000a1%d03", n))
		taskPath := "/" + task.String() + "/"
		packPath := taskPath + pack.String() + "/"
		shapes.stored = append(shapes.stored,
			repository.EntryShape{ID: task, CollectionID: collection, Type: domain.ItemTask, Path: taskPath,
				Holds: []domain.Capability{domain.CapabilityDueDate}},
			repository.EntryShape{ID: pack, CollectionID: collection, Type: domain.ItemWorkPackage, Path: packPath},
			repository.EntryShape{ID: step, CollectionID: collection, Type: domain.ItemActivity,
				Path: packPath + step.String() + "/"},
		)
	}
	preview := PreviewContainerStructure{Set: h.set, Shapes: shapes}
	registry, err := usecase.NewRegistry(nil, preview.Descriptor())
	if err != nil {
		t.Fatalf("the catalogue refused the entry: %v", err)
	}
	return h, registry
}

// UC-WRK-24 checks 2 and 7: switching the hub to a list names the levels with their counts, the
// values on entries still shown, the views - over every collection that follows it - and the
// collection that keeps its own views; and writes nothing.
func TestAPreviewOfAHubNamesWhatWouldNoLongerBeShownAndWhoKeepsItsOwn(t *testing.T) {
	h, registry := newPreviewHarness(t)

	out, err := registry.Invoke(context.Background(), PreviewContainerStructureName, actor(),
		usecase.Input{"container_id": hubID.String(), "preset": "LIST"})
	if err != nil {
		t.Fatalf("the preview failed: %v", err)
	}
	if out["narrows"] != true {
		t.Errorf("narrows = %v", out["narrows"])
	}
	wantLevels := []any{
		map[string]any{"type": "WORK_PACKAGE", "entries": 2},
		map[string]any{"type": "ACTIVITY", "entries": 2},
	}
	if !reflect.DeepEqual(out["levels"], wantLevels) {
		t.Errorf("levels = %v", out["levels"])
	}
	wantFields := []any{map[string]any{"type": "TASK", "capability": "DUE_DATE", "entries": 2}}
	if !reflect.DeepEqual(out["fields"], wantFields) {
		t.Errorf("fields = %v", out["fields"])
	}
	views, _ := out["views"].([]any)
	if !slices.Contains(views, any("KANBAN")) {
		t.Errorf("views = %v", views)
	}
	wantKeeping := []any{map[string]any{
		"container_id": otherCollectionID.String(), "name": "Hardware", "settings": []any{"views"},
	}}
	if !reflect.DeepEqual(out["keeping"], wantKeeping) {
		t.Errorf("keeping = %v", out["keeping"])
	}

	if h.containers.stored[hubID].Structure.Preset != domain.PresetProject || len(h.store.written) != 0 {
		t.Error("the preview changed the hub")
	}
	if len(h.audit.entries) != 0 || len(h.events.appended) != 0 || len(h.changes.recorded) != 0 {
		t.Errorf("the preview wrote: audit %d, events %d, changes %d",
			len(h.audit.entries), len(h.events.appended), len(h.changes.recorded))
	}
}

// UC-WRK-24 check 1: a change that offers more narrows nothing, so a client applies it at once.
func TestAPreviewOfAWideningNarrowsNothing(t *testing.T) {
	h, registry := newPreviewHarness(t)
	list, _ := domain.FindPreset(domain.PresetList, domain.NewStructureBound(seededProfiles(), seededProfiles()))
	collection := h.containers.stored[collectionID]
	collection.Structure = domain.StructureLayer{Preset: domain.PresetList, Values: list.Values}
	h.containers.stored[collectionID] = collection

	out, err := registry.Invoke(context.Background(), PreviewContainerStructureName, actor(),
		usecase.Input{"container_id": collectionID.String(), "preset": "PROJECT"})
	if err != nil {
		t.Fatalf("the preview failed: %v", err)
	}
	if out["narrows"] != false || len(out["levels"].([]any)) != 0 || len(out["keeping"].([]any)) != 0 {
		t.Errorf("answered %v", out)
	}
}

// A preview is refused as the change would be.
func TestAPreviewIsRefusedAsTheChangeWouldBe(t *testing.T) {
	h, registry := newPreviewHarness(t)
	h.authorizer.err = shared.ErrForbidden.WithDetail("access.denied")

	_, err := registry.Invoke(context.Background(), PreviewContainerStructureName, actor(),
		usecase.Input{"container_id": collectionID.String(), "preset": "LIST"})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("err = %v", err)
	}
	if h.authorizer.requests[len(h.authorizer.requests)-1].Action != ContainerStructurePreviewedAction {
		t.Errorf("asked %+v", h.authorizer.requests)
	}
}
