// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// UC-WRK-25 checks 3 and 5 at the structure writer: a done section's name, and categories that are
// labels of the collection naming them - refused on a hub or the workspace, refused for another
// collection's label or one that does not exist, kept when this layer already named a label
// deleted since.

var (
	aisleDairy   = shared.ID("0192f000-0000-7000-8000-0000000000f1")
	aisleProduce = shared.ID("0192f000-0000-7000-8000-0000000000f2")
	aisleOther   = shared.ID("0192f000-0000-7000-8000-0000000000f3")
	aisleGone    = shared.ID("0192f000-0000-7000-8000-0000000000f4")
)

func categoriesHarness(t *testing.T) *structureHarness {
	t.Helper()
	h := newStructureHarness(t)
	deleted := now.Add(-time.Hour)
	store := &labels{stored: map[shared.ID]domain.Label{
		aisleDairy:   {ID: aisleDairy, TenantID: tenantID, CollectionID: collectionID, Name: "Dairy", ColorToken: "accent.blue"},
		aisleProduce: {ID: aisleProduce, TenantID: tenantID, CollectionID: collectionID, Name: "Produce", ColorToken: "accent.green"},
		aisleOther:   {ID: aisleOther, TenantID: tenantID, CollectionID: otherCollectionID, Name: "Tools", ColorToken: "accent.red"},
		aisleGone:    {ID: aisleGone, TenantID: tenantID, CollectionID: collectionID, Name: "Bakery", ColorToken: "accent.amber", DeletedAt: &deleted},
	}}
	h.set.Labels = store
	registry, err := usecase.NewRegistry(nil, h.set.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	h.registry = registry
	return h
}

func categoriesInput(container shared.ID, categories ...shared.ID) usecase.Input {
	ids := make([]any, 0, len(categories))
	for _, id := range categories {
		ids = append(ids, id.String())
	}
	return usecase.Input{
		"container_id": container.String(),
		"settings": map[string]any{"fields": map[string]any{
			"TASK": map[string]any{"capabilities": []any{"COMPLETION", "LABELS"}, "categories": ids},
		}},
	}
}

func TestACollectionNamesItsLabelsAsCategoriesInOrder(t *testing.T) {
	h := categoriesHarness(t)
	out, err := h.invoke(t, categoriesInput(collectionID, aisleProduce, aisleDairy))
	if err != nil {
		t.Fatalf("naming categories: %v", err)
	}
	stored := h.containers.stored[collectionID].Structure.Values.Fields[domain.ItemTask].Categories
	if !slices.Equal(stored, []shared.ID{aisleProduce, aisleDairy}) {
		t.Errorf("stored %v", stored)
	}
	structure, _ := out["structure"].(map[string]any)
	settings, _ := structure["settings"].(map[string]any)
	fields, _ := settings["fields"].(map[string]any)
	task, _ := fields["TASK"].(map[string]any)
	if got, _ := task["categories"].([]any); len(got) != 2 || got[0] != aisleProduce.String() {
		t.Errorf("answered %v", task)
	}
}

func TestCategoriesAreRefusedWhereTheyNameNothing(t *testing.T) {
	cases := map[string]struct {
		container shared.ID
		category  shared.ID
		code      string
	}{
		"on a hub":                      {hubID, aisleDairy, "structure.categories_collection_only"},
		"another collection's label":    {collectionID, aisleOther, "structure.category_unknown"},
		"no label at all":               {collectionID, shared.ID("0192f000-0000-7000-8000-0000000000f9"), "structure.category_unknown"},
		"a label deleted before naming": {collectionID, aisleGone, "structure.category_unknown"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h := categoriesHarness(t)
			before := h.containers.stored[c.container]
			_, err := h.invoke(t, categoriesInput(c.container, c.category))
			if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != c.code {
				t.Fatalf("answered %v, want %s", err, c.code)
			}
			if refusal := shared.AsError(err); refusal.Params["label_id"] != "" {
				t.Errorf("the refusal names the label: %+v", refusal.Params)
			}
			if after := h.containers.stored[c.container]; after.Version != before.Version {
				t.Error("the layer was written all the same")
			}
		})
	}
}

// The undo of a label's deletion (UX-16) brings its category back: an id this layer already names
// is kept, deleted or not.
func TestAnAlreadyNamedCategoryIsKeptAfterItsLabelWasDeleted(t *testing.T) {
	h := categoriesHarness(t)
	collection := h.containers.stored[collectionID]
	collection.Structure = domain.StructureLayer{Values: domain.StructureValues{Fields: map[domain.ItemType]domain.LevelFields{
		domain.ItemTask: {Capabilities: []domain.Capability{domain.CapabilityCompletion, domain.CapabilityLabels},
			Categories: []shared.ID{aisleGone, aisleDairy}},
	}}}
	h.containers.stored[collectionID] = collection

	if _, err := h.invoke(t, categoriesInput(collectionID, aisleDairy, aisleGone, aisleProduce)); err != nil {
		t.Fatalf("reordering with a deleted label still named: %v", err)
	}
	stored := h.containers.stored[collectionID].Structure.Values.Fields[domain.ItemTask].Categories
	if !slices.Equal(stored, []shared.ID{aisleDairy, aisleGone, aisleProduce}) {
		t.Errorf("stored %v", stored)
	}
}

func TestTheWorkspaceNamesNoCategories(t *testing.T) {
	h, registry := newWorkspaceStructureHarness(t)
	_ = h
	_, err := registry.Invoke(context.Background(), SetWorkspaceStructureName, actor(), usecase.Input{
		"settings": map[string]any{"fields": map[string]any{
			"TASK": map[string]any{"capabilities": []any{"COMPLETION", "LABELS"}, "categories": []any{aisleDairy.String()}},
		}},
	})
	if refusal := shared.AsError(err); refusal == nil || refusal.DetailCode != "structure.categories_collection_only" {
		t.Fatalf("answered %v", err)
	}
}

func TestADoneSectionsNameIsStoredAndAnswered(t *testing.T) {
	h := categoriesHarness(t)
	out, err := h.invoke(t, usecase.Input{
		"container_id": collectionID.String(),
		"settings":     map[string]any{"levels": []any{map[string]any{"type": "TASK", "done_name": " In the trolley "}}},
	})
	if err != nil {
		t.Fatalf("naming: %v", err)
	}
	if got := h.containers.stored[collectionID].Structure.Values.Levels[0].DoneName; got != "In the trolley" {
		t.Errorf("stored %q", got)
	}
	structure, _ := out["structure"].(map[string]any)
	settings, _ := structure["settings"].(map[string]any)
	levels, _ := settings["levels"].([]any)
	if level, _ := levels[0].(map[string]any); level["done_name"] != "In the trolley" {
		t.Errorf("answered %v", levels)
	}
	for _, entry := range h.audit.entries {
		recorded, _ := entry.Changes["structure.levels"].(map[string]any)
		if recorded == nil || recorded["changed"] != true {
			t.Fatalf("the levels are not in the trail: %+v", entry.Changes)
		}
		if _, readable := recorded["to"]; readable {
			t.Errorf("the done section's name is in the trail in clear text: %+v", recorded)
		}
	}

	if _, err := h.invoke(t, usecase.Input{
		"container_id": collectionID.String(),
		"settings":     map[string]any{"levels": []any{map[string]any{"type": "TASK", "colour": "red"}}},
	}); shared.AsError(err) == nil || shared.AsError(err).DetailCode != "structure.setting_invalid" {
		t.Errorf("an unknown member of a level answered %v", err)
	}
}
